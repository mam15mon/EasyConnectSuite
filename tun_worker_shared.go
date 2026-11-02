//go:build windows || darwin

package main

import (
	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type workerIngress interface {
	Close() error
	UpdateSpec(engine.TUNSpec) error
}
type workerEntry struct {
	spec    engine.TUNSpec
	ingress workerIngress
	backend *workerBackend
}
type workerServer struct {
	mu            sync.Mutex
	peer          *tunipc.Peer
	pid           uint32
	entries       map[string]*workerEntry
	reservations  map[string]bool
	closing       chan struct{}
	once          sync.Once
	authenticated bool
	cleanupErr    error
}
type workerBackend struct {
	mu     sync.RWMutex
	ref    tunipc.Ref
	server *workerServer
}

func (b *workerBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	b.mu.RLock()
	ref := b.ref
	b.mu.RUnlock()
	ctx, c := context.WithTimeout(ctx, 15*time.Second)
	defer c()
	r, e := b.server.peer.Call(ctx, tunipc.Message{Op: "dial", Ref: ref, Network: network, Address: address})
	if e != nil {
		return nil, e
	}
	conn, e := tunipc.Dial(ctx, r.Pipe, b.server.pid)
	if e != nil {
		return nil, e
	}
	return tunipc.NewDataConn(conn, network == "udp"), nil
}
func (b *workerBackend) DNS(ctx context.Context, data []byte) ([]byte, []string, error) {
	b.mu.RLock()
	ref := b.ref
	b.mu.RUnlock()
	ctx, c := context.WithTimeout(ctx, 10*time.Second)
	defer c()
	r, e := b.server.peer.Call(ctx, tunipc.Message{Op: "dns", Ref: ref, Data: data})
	return r.Data, r.Routes, e
}
func (w *workerServer) handle(ctx context.Context, m tunipc.Message) (tunipc.Message, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if m.Op == "hello" {
		if w.authenticated {
			return tunipc.Message{}, errors.New("duplicate hello")
		}
		err := cleanupBeforeWorker()
		if err == nil {
			w.authenticated = true
		}
		return tunipc.Message{}, err
	}
	if !w.authenticated {
		return tunipc.Message{}, errors.New("TUN Worker handshake required")
	}
	switch m.Op {
	case "prepare":
		if m.Ref.Profile == "" {
			return tunipc.Message{}, errors.New("missing profile")
		}
		w.reservations[m.Ref.Profile] = true
	case "release":
		delete(w.reservations, m.Ref.Profile)
	case "attach":
		if m.Spec == nil || m.Spec.Ref != m.Ref {
			return tunipc.Message{}, errors.New("invalid TUN snapshot")
		}
		if err := m.Spec.Validate(); err != nil {
			return tunipc.Message{}, err
		}
		if w.entries[m.Ref.Attachment] != nil {
			return tunipc.Message{}, errors.New("duplicate attachment")
		}
		for _, entry := range w.entries {
			if entry.spec.Profile == m.Ref.Profile || entry.spec.Slot == m.Spec.Slot {
				return tunipc.Message{}, errors.New("TUN profile or slot already attached")
			}
		}
		backend := &workerBackend{ref: m.Ref, server: w}
		ingress, err := startWorkerIngress(*m.Spec, backend, func(stage, text string) {
			_ = w.peer.Emit(tunipc.Message{Op: "progress", Ref: m.Ref, Text: stage + ": " + text})
		})
		if err != nil {
			return tunipc.Message{}, err
		}
		if ctx.Err() != nil {
			cleanupErr := ingress.Close()
			w.cleanupErr = errors.Join(w.cleanupErr, cleanupErr)
			return tunipc.Message{}, errors.Join(ctx.Err(), cleanupErr)
		}
		w.entries[m.Ref.Attachment] = &workerEntry{spec: *m.Spec, ingress: ingress, backend: backend}
		delete(w.reservations, m.Ref.Profile)
	case "update":
		entry := w.entries[m.Ref.Attachment]
		if entry == nil || m.Spec == nil || m.Spec.Ref != m.Ref || entry.spec.Profile != m.Ref.Profile || entry.spec.Session != m.Ref.Session || m.Ref.Policy <= entry.spec.Policy || entry.spec.Slot != m.Spec.Slot {
			return tunipc.Message{}, errors.New("obsolete TUN update")
		}
		if err := entry.ingress.UpdateSpec(*m.Spec); err != nil {
			cleanupErr := entry.ingress.Close()
			w.cleanupErr = errors.Join(w.cleanupErr, cleanupErr)
			err = errors.Join(err, cleanupErr)
			delete(w.entries, m.Ref.Attachment)
			return tunipc.Message{}, err
		}
		entry.spec = *m.Spec
		entry.backend.mu.Lock()
		entry.backend.ref = m.Ref
		entry.backend.mu.Unlock()
	case "detach":
		entry := w.entries[m.Ref.Attachment]
		if entry == nil {
			return tunipc.Message{}, nil
		}
		if entry.spec.Ref != m.Ref {
			return tunipc.Message{}, errors.New("obsolete TUN detach")
		}
		err := entry.ingress.Close()
		w.cleanupErr = errors.Join(w.cleanupErr, err)
		delete(w.entries, m.Ref.Attachment)
		return tunipc.Message{}, err
	case "shutdown":
		err := w.cleanup()
		w.once.Do(func() { close(w.closing) })
		return tunipc.Message{}, err
	default:
		return tunipc.Message{}, errors.New("unsupported GUI request")
	}
	return tunipc.Message{}, nil
}
func (w *workerServer) cleanup() error {
	err := w.cleanupErr
	for id, e := range w.entries {
		err = errors.Join(err, e.ingress.Close())
		delete(w.entries, id)
	}
	w.cleanupErr = err
	return err
}

type workerLog struct{ peer *tunipc.Peer }

func (w workerLog) Write(data []byte) (int, error) {
	// Diagnostic evidence must survive a broken control pipe. The worker's
	// stderr already belongs to its private crash log; write it there as well.
	if bytes.Contains(data, []byte("operation=")) {
		_, _ = os.Stderr.Write(data)
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 4096 {
		text = text[:4096]
	}
	err := w.peer.Emit(tunipc.Message{Op: "log", Text: text})
	return len(data), err
}
