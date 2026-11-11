package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
)

type pipeTestBackend struct {
	target  string
	network string
}

type rejectedUpdateIngress struct{ closed bool }

func (i *rejectedUpdateIngress) Close() error { i.closed = true; return nil }
func (i *rejectedUpdateIngress) UpdateSpec(context.Context, engine.TUNSpec) error {
	return errors.New("conflicting DNS namespace")
}
func TestWorkerRejectsDirectionAndClosesFailedPolicyUpdate(t *testing.T) {
	ref := tunipc.Ref{Profile: "profile", Session: "session", Attachment: "attachment", Policy: 1}
	spec := engine.TUNSpec{Ref: ref, Slot: 1}
	ingress := &rejectedUpdateIngress{}
	w := &workerServer{authenticated: true, entries: map[string]*workerEntry{ref.Attachment: {spec: spec, ingress: ingress, backend: &workerBackend{ref: ref}}}}
	for _, op := range []string{"dial", "dns", "unknown"} {
		if _, e := w.handle(context.Background(), tunipc.Message{Op: op, Ref: ref}); e == nil {
			t.Fatal("wrong direction accepted", op)
		}
	}
	stale := spec
	if _, e := w.handle(context.Background(), tunipc.Message{Op: "update", Ref: stale.Ref, Spec: &stale}); e == nil {
		t.Fatal("old version accepted")
	}
	if ingress.closed {
		t.Fatal("stale request closed active ingress")
	}
	next := spec
	next.Policy++
	if _, e := w.handle(context.Background(), tunipc.Message{Op: "update", Ref: next.Ref, Spec: &next}); e == nil {
		t.Fatal("failed update hidden")
	}
	if !ingress.closed || len(w.entries) != 0 {
		t.Fatal("failed update retained ingress")
	}
}

func (b *pipeTestBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	if network != b.network || address != "192.0.2.1:443" {
		return nil, errors.New("denied")
	}
	return (&net.Dialer{}).DialContext(ctx, network, b.target)
}
func (b *pipeTestBackend) DNS(context.Context, []byte) ([]byte, []string, error) {
	return []byte{1, 2, 3}, []string{"192.0.2.1/32"}, nil
}
func testWorkerBridge(t *testing.T, backend engine.TUNBackend) (*workerBackend, *remoteTUN) {
	t.Helper()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(c)
	name := tunipc.NewName()
	l, err := tunipc.Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, e := l.Accept(ctx, uint32(os.Getpid()))
		if e != nil {
			acceptErr <- e
		} else {
			accepted <- conn
		}
	}()
	client, err := tunipc.Dial(ctx, name, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	var server net.Conn
	select {
	case server = <-accepted:
	case err = <-acceptErr:
		t.Fatal(err)
	}
	m := newTUNManager()
	m.pid = uint32(os.Getpid())
	m.peer = tunipc.NewPeer(server, m.handle, nil)
	w := &workerServer{pid: uint32(os.Getpid()), peer: tunipc.NewPeer(client, nil, nil)}
	ac, cc := context.WithCancel(context.Background())
	ref := tunipc.Ref{Profile: "profile", Session: "session", Attachment: "attachment", Policy: 1}
	a := &remoteTUN{manager: m, spec: engine.TUNSpec{Ref: ref}, backend: backend, ctx: ac, cancel: cc, done: make(chan struct{})}
	m.attachments[ref.Attachment] = a
	t.Cleanup(func() { a.finish(); m.peer.Close(); w.peer.Close() })
	return &workerBackend{ref: ref, server: w}, a
}
func TestWorkerTCPBridgeKeepsReverseTrafficAfterEOF(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		c, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		b, e := io.ReadAll(c)
		if e == nil && string(b) != "request before EOF" {
			e = errors.New("wrong TCP payload")
		}
		if e == nil {
			_, e = c.Write([]byte("response after EOF"))
		}
		done <- e
	}()
	b, _ := testWorkerBridge(t, &pipeTestBackend{target: listener.Addr().String(), network: "tcp"})
	conn, err := b.Dial(context.Background(), "tcp", "192.0.2.1:443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = conn.Write([]byte("request before EOF")); err != nil {
		t.Fatal(err)
	}
	if err = conn.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(conn)
	if err != nil || string(got) != "response after EOF" {
		t.Fatalf("%q %v", got, err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
func TestWorkerUDPBridgePreservesPacketsAndRejectsOldPolicy(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, e := listener.ReadFrom(buf)
			if e != nil {
				return
			}
			listener.WriteTo(buf[:n], addr)
		}
	}()
	b, a := testWorkerBridge(t, &pipeTestBackend{target: listener.LocalAddr().String(), network: "udp"})
	conn, err := b.Dial(context.Background(), "udp", "192.0.2.1:443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	for _, packet := range [][]byte{{}, []byte("one"), make([]byte, 1400), []byte("three")} {
		if _, err = conn.Write(packet); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 65535)
		n, e := conn.Read(buf)
		if e != nil || n != len(packet) {
			t.Fatalf("datagram length %d want %d: %v", n, len(packet), e)
		}
	}
	a.mu.Lock()
	a.spec.Policy++
	a.mu.Unlock()
	if _, _, err = b.DNS(context.Background(), []byte{0}); err == nil {
		t.Fatal("old policy accepted")
	}
	a.finish()
	if _, err = b.Dial(context.Background(), "udp", "192.0.2.1:443"); err == nil {
		t.Fatal("detached backend accepted")
	}
}
