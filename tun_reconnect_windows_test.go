package main

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
)

type reconnectRequestBackend struct {
	dial func() (net.Conn, error)
	dns  func() ([]byte, []string, error)
}

func (b *reconnectRequestBackend) Dial(context.Context, string, string) (net.Conn, error) {
	return b.dial()
}
func (b *reconnectRequestBackend) DNS(context.Context, []byte) ([]byte, []string, error) {
	return b.dns()
}

func reconnectAttachment(t *testing.T, backend engine.TUNBackend) *remoteTUN {
	t.Helper()
	left, right := net.Pipe()
	m := newTUNManager()
	m.peer = tunipc.NewPeer(left, m.handle, nil)
	peer := m.peer
	worker := tunipc.NewPeer(right, func(context.Context, tunipc.Message) (tunipc.Message, error) {
		return tunipc.Message{}, nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	a := &remoteTUN{manager: m, backend: backend, ctx: ctx, cancel: cancel, done: make(chan struct{}),
		spec: engine.TUNSpec{Ref: tunipc.Ref{Profile: "reconnect%profile", Session: "session", Attachment: "attachment", Policy: 1}}}
	m.attachments[a.spec.Attachment] = a
	t.Cleanup(func() { a.finish(); peer.Close(); worker.Close() })
	return a
}

type reconnectBlockingCloser struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *reconnectBlockingCloser) Close() error {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	return nil
}

// A relogin callback can run inside a TCP send that already owns the endpoint
// mutex. Closing that same stream must wait outside the relogin call stack.
func TestRemoteTUNUpdateDoesNotWaitForStreamClose(t *testing.T) {
	a := reconnectAttachment(t, nil)
	closer := &reconnectBlockingCloser{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(closer.release)
	a.streams.Store(closer, true)
	next := a.spec
	next.Policy++
	updated := make(chan error, 1)
	go func() { updated <- a.Update(context.Background(), next) }()
	select {
	case <-closer.entered:
	case <-time.After(time.Second):
		t.Fatal("old stream close did not start")
	}
	select {
	case err := <-updated:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("relogin policy update waited for the sending TCP endpoint to close")
	}
	if _, retained := a.streams.Load(closer); retained {
		t.Fatal("retired stream still belongs to the current policy")
	}
}

func TestRemoteTUNControlDoesNotWaitForPendingRequest(t *testing.T) {
	for _, op := range []string{"dial", "dns"} {
		for _, control := range []string{"update", "close"} {
			t.Run(op+"/"+control, func(t *testing.T) {
				entered, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				defer unblock()
				left, right := net.Pipe()
				defer left.Close()
				defer right.Close()
				backend := &reconnectRequestBackend{
					dial: func() (net.Conn, error) { close(entered); <-release; return left, nil },
					dns: func() ([]byte, []string, error) {
						close(entered)
						<-release
						return []byte("private DNS result"), []string{"192.0.2.1/32"}, nil
					},
				}
				a := reconnectAttachment(t, backend)
				request := tunipc.Message{Op: op, Ref: a.spec.Ref, Network: "tcp", Address: "192.0.2.1:443"}
				finished := make(chan error, 1)
				go func() {
					reply, err := a.manager.handle(context.Background(), request)
					if reply.Pipe != "" || len(reply.Data) != 0 || len(reply.Routes) != 0 {
						err = errors.New("obsolete request published a flow or DNS result")
					} else if err == nil {
						err = errors.New("obsolete request succeeded")
					} else {
						err = nil
					}
					finished <- err
				}()
				<-entered
				controlled := make(chan error, 1)
				go func() {
					if control == "close" {
						controlled <- a.Close(context.Background())
					} else {
						next := request.Ref
						next.Policy++
						controlled <- a.Update(context.Background(), engine.TUNSpec{Ref: next})
					}
				}()
				blocked := false
				select {
				case err := <-controlled:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					blocked = true
				}
				unblock()
				if blocked {
					<-controlled
				}
				if err := <-finished; err != nil {
					t.Error(err)
				}
				if blocked {
					t.Error("TUN control waited for a network request that needs relogin")
				}
				if op == "dial" {
					right.SetReadDeadline(time.Now().Add(time.Second))
					if _, err := right.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
						t.Errorf("obsolete connection was not closed: %v", err)
					}
				}
			})
		}
	}
}
