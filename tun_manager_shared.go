//go:build windows || darwin

package main

import (
	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	vpnlog "EasyConnectSuite/vpncore/log"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type remoteTUN struct {
	manager    *tunManager
	mu         sync.RWMutex
	spec       engine.TUNSpec
	backend    engine.TUNBackend
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	once       sync.Once
	streams    sync.Map
	workerDone <-chan struct{}
	lossErr    error
}

func (m *tunManager) lost(peer *tunipc.Peer, cleanupErr error) {
	m.mu.Lock()
	if m.peer != peer {
		m.mu.Unlock()
		return
	}
	m.peer = nil
	m.recoveryErr = cleanupErr
	m.reservations = map[string]bool{}
	list := make([]*remoteTUN, 0, len(m.attachments))
	for _, a := range m.attachments {
		list = append(list, a)
	}
	m.attachments = map[string]*remoteTUN{}
	m.mu.Unlock()
	for _, a := range list {
		a.mu.Lock()
		a.lossErr = cleanupErr
		a.mu.Unlock()
		a.finish()
	}
}
func (m *tunManager) request(ctx context.Context, msg tunipc.Message) (tunipc.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	m.mu.Lock()
	p := m.peer
	m.mu.Unlock()
	if p == nil {
		return tunipc.Message{}, errors.New("TUN Worker 未运行")
	}
	return p.Call(ctx, msg)
}
func (m *tunManager) Prepare(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if err := m.ensure(ctx); err != nil {
		return err
	}
	_, err := m.request(ctx, tunipc.Message{Op: "prepare", Ref: tunipc.Ref{Profile: id}})
	if err != nil {
		m.idle(ctx)
		return err
	}
	m.mu.Lock()
	m.reservations[id] = true
	m.mu.Unlock()
	return nil
}
func (m *tunManager) Release(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	reserved := m.reservations[id]
	delete(m.reservations, id)
	m.mu.Unlock()
	var err error
	if reserved {
		_, err = m.request(ctx, tunipc.Message{Op: "release", Ref: tunipc.Ref{Profile: id}})
	}
	return errors.Join(err, m.idle(ctx))
}
func (m *tunManager) idle(ctx context.Context) error {
	m.mu.Lock()
	p := m.peer
	idle := len(m.attachments) == 0 && len(m.reservations) == 0
	if p != nil && idle {
		m.peer = nil
	}
	m.mu.Unlock()
	if p != nil && idle {
		_, err := p.Call(ctx, tunipc.Message{Op: "shutdown"})
		p.Close()
		return err
	}
	return nil
}
func (m *tunManager) Attach(ctx context.Context, spec engine.TUNSpec, b engine.TUNBackend) (engine.TUNAttachment, error) {
	m.op.Lock()
	defer m.op.Unlock()
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	ac, cancel := context.WithCancel(ctx)
	a := &remoteTUN{manager: m, spec: spec, backend: b, ctx: ac, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	a.workerDone = m.workerDone
	m.attachments[spec.Attachment] = a
	m.mu.Unlock()
	_, err := m.request(ctx, tunipc.Message{Op: "attach", Ref: spec.Ref, Spec: &spec})
	if err != nil {
		_, cleanupErr := m.request(context.Background(), tunipc.Message{Op: "detach", Ref: spec.Ref})
		err = errors.Join(err, cleanupErr)
		m.mu.Lock()
		delete(m.reservations, spec.Profile)
		delete(m.attachments, spec.Attachment)
		m.mu.Unlock()
		a.finish()
		m.idle(context.Background())
		return nil, err
	}
	m.mu.Lock()
	delete(m.reservations, spec.Profile)
	m.mu.Unlock()
	return a, nil
}
func (a *remoteTUN) Done() <-chan struct{} { return a.done }
func (a *remoteTUN) finish() {
	a.once.Do(func() {
		a.mu.Lock()
		a.cancel()
		var streams []io.Closer
		a.streams.Range(func(key, value any) bool {
			streams = append(streams, key.(io.Closer))
			a.streams.Delete(key)
			return true
		})
		a.mu.Unlock()
		for _, stream := range streams {
			stream.Close()
		}
		close(a.done)
	})
}
func (a *remoteTUN) Update(ctx context.Context, spec engine.TUNSpec) (err error) {
	prefix := fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment, spec.Policy)
	trace := vpnlog.BeginOperation(prefix, "attachment_update", ctx.Done())
	defer func() { trace.Finish("returned", err) }()
	trace.Step("wait_manager_operation_lock")
	a.manager.op.Lock()
	defer a.manager.op.Unlock()
	trace.Step("wait_attachment_lock")
	a.mu.Lock()
	a.spec = spec
	var streams []io.Closer
	a.streams.Range(func(key, value any) bool {
		streams = append(streams, key.(io.Closer))
		a.streams.Delete(key)
		return true
	})
	a.mu.Unlock()
	// Relogin may run inside WritePackets with a TCP endpoint lock held.
	// Let that call return before Close tries to acquire the same lock.
	if len(streams) > 0 {
		go func() {
			cleanup := vpnlog.BeginOperation(prefix, "retired_stream_close", a.ctx.Done())
			var closeErr error
			defer func() { cleanup.Finish("closed", closeErr) }()
			vpnlog.Printf("%sretired_streams=%d", prefix, len(streams))
			for _, stream := range streams {
				cleanup.Step("stream_close")
				closeErr = errors.Join(closeErr, stream.Close())
			}
		}()
	}
	trace.Step("worker_update_reply")
	_, err = a.manager.request(ctx, tunipc.Message{Op: "update", Ref: spec.Ref, Spec: &spec})
	if err != nil {
		trace.Step("worker_cleanup_wait")
		err = errors.Join(err, a.waitWorkerCleanup(ctx))
	}
	return err
}
func (a *remoteTUN) Close(ctx context.Context) error {
	cleanupErr := a.waitWorkerCleanup(ctx)
	m := a.manager
	m.op.Lock()
	defer m.op.Unlock()
	a.finish()
	a.mu.RLock()
	ref := a.spec.Ref
	a.mu.RUnlock()
	m.mu.Lock()
	_, exists := m.attachments[ref.Attachment]
	m.mu.Unlock()
	var err error
	if exists {
		_, err = m.request(ctx, tunipc.Message{Op: "detach", Ref: ref})
		m.mu.Lock()
		delete(m.attachments, ref.Attachment)
		m.mu.Unlock()
	}
	a.mu.RLock()
	lossErr := a.lossErr
	a.mu.RUnlock()
	return errors.Join(cleanupErr, lossErr, err, m.idle(ctx))
}
func (m *tunManager) Close() {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	p := m.peer
	m.mu.Unlock()
	if p != nil {
		ctx, c := context.WithTimeout(context.Background(), 45*time.Second)
		defer c()
		_, _ = p.Call(ctx, tunipc.Message{Op: "shutdown"})
		p.Close()
	}
}
func (m *tunManager) handle(ctx context.Context, msg tunipc.Message) (tunipc.Message, error) {
	if msg.Op != "dial" && msg.Op != "dns" {
		return tunipc.Message{}, errors.New("unsupported Worker request")
	}
	m.mu.Lock()
	a := m.attachments[msg.Ref.Attachment]
	pid := m.pid
	m.mu.Unlock()
	if a == nil {
		return tunipc.Message{}, errors.New("unknown TUN attachment")
	}
	a.mu.RLock()
	if a.spec.Ref != msg.Ref || a.ctx.Err() != nil {
		a.mu.RUnlock()
		return tunipc.Message{}, errors.New("obsolete TUN attachment")
	}
	a.mu.RUnlock()
	// Dial/DNS can synchronously trigger a relogin and policy update.
	// Snapshot identity here and validate it again before publishing results.
	if msg.Op == "dns" {
		data, routes, err := a.backend.DNS(ctx, msg.Data)
		a.mu.RLock()
		obsolete := a.spec.Ref != msg.Ref || a.ctx.Err() != nil
		a.mu.RUnlock()
		if obsolete {
			return tunipc.Message{}, errors.New("obsolete TUN attachment")
		}
		if ctx.Err() != nil {
			return tunipc.Message{}, ctx.Err()
		}
		return tunipc.Message{Data: data, Routes: routes}, err
	}
	select {
	case m.flows <- struct{}{}:
	default:
		return tunipc.Message{}, errors.New("TUN flow limit reached")
	}
	upstream, err := a.backend.Dial(ctx, msg.Network, msg.Address)
	if err != nil {
		<-m.flows
		return tunipc.Message{}, err
	}
	name := tunipc.NewName()
	listener, err := tunipc.Listen(name)
	if err != nil {
		upstream.Close()
		<-m.flows
		return tunipc.Message{}, err
	}
	a.mu.RLock()
	obsolete := a.spec.Ref != msg.Ref || a.ctx.Err() != nil
	if obsolete || ctx.Err() != nil {
		a.mu.RUnlock()
		upstream.Close()
		listener.Close()
		<-m.flows
		if ctx.Err() != nil {
			return tunipc.Message{}, ctx.Err()
		}
		return tunipc.Message{}, net.ErrClosed
	}
	a.streams.Store(upstream, true)
	a.streams.Store(listener, true)
	a.mu.RUnlock()
	go func() {
		defer func() {
			upstream.Close()
			listener.Close()
			a.streams.Delete(upstream)
			a.streams.Delete(listener)
			<-m.flows
		}()
		ac, c := context.WithTimeout(a.ctx, 15*time.Second)
		defer c()
		conn, e := listener.Accept(ac, pid)
		if e != nil {
			return
		}
		defer conn.Close()
		a.mu.RLock()
		if a.spec.Ref != msg.Ref || a.ctx.Err() != nil {
			a.mu.RUnlock()
			return
		}
		a.streams.Store(conn, true)
		a.mu.RUnlock()
		defer a.streams.Delete(conn)
		tunipc.Relay(tunipc.NewDataConn(conn, msg.Network == "udp"), upstream, msg.Network == "udp")
	}()
	return tunipc.Message{Pipe: name}, nil
}

func (a *remoteTUN) waitWorkerCleanup(ctx context.Context) error {
	m := a.manager
	m.mu.Lock()
	peer := m.peer
	m.mu.Unlock()
	if peer != nil {
		select {
		case <-peer.Done():
		default:
			return nil
		}
	}
	select {
	case <-a.workerDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
