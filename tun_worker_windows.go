package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	vpnlog "EasyConnectSuite/vpncore/log"
	"golang.org/x/sys/windows"
)

type tunManager struct {
	op           sync.Mutex
	mu           sync.Mutex
	peer         *tunipc.Peer
	pid          uint32
	workerDone   chan struct{}
	recoveryErr  error
	attachments  map[string]*remoteTUN
	reservations map[string]bool
	flows        chan struct{}
	launch       func(string) (windows.Handle, error)
}
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

func newTUNManager() *tunManager {
	return &tunManager{attachments: map[string]*remoteTUN{}, reservations: map[string]bool{}, flows: make(chan struct{}, tunipc.MaxFlows), launch: launchTUNWorker}
}
func (m *tunManager) ensure(ctx context.Context) error {
	m.mu.Lock()
	p := m.peer
	recoveryErr := m.recoveryErr
	m.mu.Unlock()
	if recoveryErr != nil {
		return recoveryErr
	}
	if p != nil {
		select {
		case <-p.Done():
			return errors.New("TUN Worker 已退出，请重新开启 TUN")
		default:
			return nil
		}
	}
	name := tunipc.NewName()
	listener, err := tunipc.Listen(name)
	if err != nil {
		return err
	}
	defer listener.Close()
	handle, err := m.launch(name)
	if err != nil {
		return err
	}
	pid, err := windows.GetProcessId(handle)
	if err != nil {
		windows.CloseHandle(handle)
		return err
	}
	handshake, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	conn, err := listener.Accept(handshake, pid)
	if err != nil {
		windows.CloseHandle(handle)
		return err
	}
	if state, e := windows.WaitForSingleObject(handle, 0); e != nil || state == windows.WAIT_OBJECT_0 {
		conn.Close()
		windows.CloseHandle(handle)
		return errors.New("TUN Worker exited during handshake")
	}
	peer := tunipc.NewPeer(conn, m.handle, func(msg tunipc.Message) {
		if msg.Op == "log" || msg.Op == "progress" {
			log.Printf("[TUN worker=%d profile=%q] %s", pid, msg.Ref.Profile, msg.Text)
		}
	})
	m.mu.Lock()
	m.peer = peer
	m.pid = pid
	m.workerDone = make(chan struct{})
	workerDone := m.workerDone
	m.mu.Unlock()
	peer.Heartbeat()
	if _, err = peer.Call(handshake, tunipc.Message{Op: "hello"}); err != nil {
		peer.Close()
		windows.CloseHandle(handle)
		m.mu.Lock()
		m.peer = nil
		m.mu.Unlock()
		return err
	}
	go func() {
		defer windows.CloseHandle(handle)
		defer close(workerDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-peer.Done():
				m.lost(peer, waitTUNWorkerExit(handle))
				return
			case <-ticker.C:
				r, e := windows.WaitForSingleObject(handle, 0)
				if e != nil || r == windows.WAIT_OBJECT_0 {
					peer.Close()
				}
			}
		}
	}()
	return nil
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

func launchTUNWorker(name string) (windows.Handle, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	parameters := windows.ComposeCommandLine([]string{"--tun-worker", name, strconv.Itoa(os.Getpid())})
	info := shellExecuteInfo{Mask: 0x100 | 0x400 | 0x40, Verb: windows.StringToUTF16Ptr("runas"), File: windows.StringToUTF16Ptr(exe), Parameters: windows.StringToUTF16Ptr(parameters), Directory: windows.StringToUTF16Ptr(filepath.Dir(exe)), Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2|4)
	if int32(hr) < 0 {
		return 0, fmt.Errorf("初始化 TUN Worker 启动失败：0x%x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	result, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, errors.New("已取消管理员授权")
		}
		return 0, fmt.Errorf("启动 TUN Worker 失败：%w", err)
	}
	return info.Process, nil
}

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
		_, err := engine.CleanupStaleTUN(false)
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
		ingress, err := engine.StartTUNIngress(*m.Spec, backend, func(stage, text string) {
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
func runTUNWorker(args []string) (result error) {
	if len(args) != 3 || args[0] != "--tun-worker" {
		return errors.New("invalid TUN Worker arguments")
	}
	pid, e := strconv.ParseUint(args[2], 10, 32)
	if e != nil || pid == 0 || pid == uint64(os.Getpid()) {
		return errors.New("invalid GUI PID")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("TUN Worker requires administrator permission")
	}
	parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return e
	}
	defer windows.CloseHandle(parent)
	var path [32768]uint16
	size := uint32(len(path))
	if e = windows.QueryFullProcessImageName(parent, 0, &path[0], &size); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if !strings.EqualFold(filepath.Clean(exe), filepath.Clean(windows.UTF16ToString(path[:size]))) {
		return errors.New("GUI executable identity mismatch")
	}
	ctx, c := context.WithTimeout(context.Background(), 20*time.Second)
	defer c()
	conn, e := tunipc.Dial(ctx, args[1], uint32(pid))
	if e != nil {
		return e
	}
	w := &workerServer{pid: uint32(pid), entries: map[string]*workerEntry{}, reservations: map[string]bool{}, closing: make(chan struct{})}
	ready := make(chan struct{})
	w.peer = tunipc.NewPeer(conn, func(ctx context.Context, m tunipc.Message) (tunipc.Message, error) { <-ready; return w.handle(ctx, m) }, nil)
	close(ready)
	log.SetOutput(workerLog{w.peer})
	w.peer.Heartbeat()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer w.peer.Close()
	defer func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if err := w.cleanup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			result = errors.Join(result, err)
		}
	}()
	for {
		select {
		case <-w.peer.Done():
			return nil
		case <-w.closing:
			select {
			case <-w.peer.Done():
			case <-time.After(time.Second):
			}
			return nil
		case <-ticker.C:
			r, e := windows.WaitForSingleObject(parent, 0)
			if e != nil || r == windows.WAIT_OBJECT_0 {
				return nil
			}
		}
	}
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

// A dead control pipe does not imply that the old process finished removing
// routes. Keep its handle until cleanup completes before permitting replacement.
func waitTUNWorkerExit(handle windows.Handle) error {
	state, err := windows.WaitForSingleObject(handle, 45000)
	if err != nil {
		return fmt.Errorf("wait TUN Worker cleanup: %w", err)
	}
	if state != windows.WAIT_OBJECT_0 {
		return errors.New("TUN Worker cleanup timed out")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("TUN Worker exited with code %d; cleanup is unconfirmed", code)
	}
	return nil
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
