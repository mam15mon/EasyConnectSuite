package engine

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type controllerTestIngress func() error

func (f controllerTestIngress) Close() error { return f() }

type controllerTestBackend struct {
	dial func(context.Context, string, string) (net.Conn, error)
	dns  func(context.Context, []byte) ([]byte, []string, error)
}

func (b controllerTestBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return b.dial(ctx, network, address)
}
func (b controllerTestBackend) DNS(ctx context.Context, data []byte) ([]byte, []string, error) {
	return b.dns(ctx, data)
}
func controllerWait(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("controller operation blocked")
		return nil
	}
}

type controllerLockedConn struct {
	net.Conn
	mu           sync.Mutex
	write        func() error
	closeEntered chan struct{}
	closeDone    chan struct{}
}

func (c *controllerLockedConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.write(); err != nil {
		return 0, err
	}
	return len(p), nil
}
func (c *controllerLockedConn) Close() error {
	close(c.closeEntered)
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.closeDone)
	return nil
}

func TestLinuxControllerUpdateDoesNotReenterUpstreamEndpointLock(t *testing.T) {
	// A gVisor write can synchronously publish a recovered policy while holding
	// its endpoint lock. Retiring that same connection must not wait for Close.
	logPath := filepath.Join(t.TempDir(), "engine.log")
	capture := &Engine{opts: Options{LogFile: logPath}}
	capture.startLogCapture()
	defer capture.stopLogCapture()
	controller := NewLinuxTUNController()
	defer controller.Close()
	var active net.Conn
	raw := &controllerLockedConn{closeEntered: make(chan struct{}), closeDone: make(chan struct{})}
	controller.start = func(context.Context, TUNSpec, TUNBackend) (linuxControllerIngress, error) {
		return controllerTestIngress(func() error {
			if active != nil {
				return active.Close()
			}
			return nil
		}), nil
	}
	attachment, err := controller.Attach(context.Background(), linuxTestSpec(), controllerTestBackend{
		dial: func(context.Context, string, string) (net.Conn, error) { return raw, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	a := attachment.(*linuxTUNAttachment)
	active, err = a.generation.Dial(context.Background(), "tcp", "192.0.2.1:443")
	if err != nil {
		t.Fatal(err)
	}
	raw.write = func() error { next := linuxTestSpec(); next.Policy++; return a.Update(context.Background(), next) }
	done := make(chan error, 1)
	go func() { _, err := active.Write([]byte("policy-trigger")); done <- err }()
	if err := controllerWait(t, done); err != nil {
		t.Fatal(err)
	}
	select {
	case <-raw.closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("retired close did not finish after write released its lock")
	}
	if _, err := active.Write([]byte("stale")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("retired write: %v", err)
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	// Operation completion is emitted immediately after the underlying Close.
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if strings.Contains(text, "name=headless_retired_stream_close") && strings.Contains(text, "outcome=closed") {
			for _, marker := range []string{`profile="test"`, `session="test"`, `attachment="test"`, "policy=1", "stage=stream_close", "stage_elapsed="} {
				if !strings.Contains(text, marker) {
					t.Errorf("missing retirement evidence %q", marker)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("missing completed retirement operation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestLinuxControllerDialCanSynchronouslyReplacePolicy(t *testing.T) {
	controller := NewLinuxTUNController()
	defer controller.Close()
	controller.start = func(context.Context, TUNSpec, TUNBackend) (linuxControllerIngress, error) {
		return controllerTestIngress(func() error { return nil }), nil
	}
	var attachment TUNAttachment
	left, right := net.Pipe()
	defer right.Close()
	attachment, err := controller.Attach(context.Background(), linuxTestSpec(), controllerTestBackend{
		dial: func(context.Context, string, string) (net.Conn, error) {
			next := linuxTestSpec()
			next.Policy++
			if err := attachment.Update(context.Background(), next); err != nil {
				return nil, err
			}
			return left, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	old := attachment.(*linuxTUNAttachment).generation
	done := make(chan error, 1)
	go func() {
		conn, err := old.Dial(context.Background(), "tcp", "192.0.2.1:443")
		if conn != nil {
			conn.Close()
			done <- errors.New("stale dial published a connection")
			return
		}
		done <- err
	}()
	if err := controllerWait(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("reentrant dial: %v", err)
	}
	_ = right.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := right.Read(make([]byte, 1)); err == nil {
		t.Fatal("late connection remained open")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("late connection was not retired")
	}
}

func TestLinuxControllerDiscardsLateDNSAfterUpdateAndClose(t *testing.T) {
	for _, action := range []string{"update", "close"} {
		t.Run(action, func(t *testing.T) {
			controller := NewLinuxTUNController()
			defer controller.Close()
			controller.start = func(context.Context, TUNSpec, TUNBackend) (linuxControllerIngress, error) {
				return controllerTestIngress(func() error { return nil }), nil
			}
			entered, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			attachment, err := controller.Attach(context.Background(), linuxTestSpec(), controllerTestBackend{
				dns: func(context.Context, []byte) ([]byte, []string, error) {
					close(entered)
					<-release
					return []byte("stale-answer"), []string{"192.0.2.9/32"}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			old := attachment.(*linuxTUNAttachment).generation
			done := make(chan error, 1)
			go func() {
				answer, routes, err := old.DNS(context.Background(), nil)
				if answer != nil || routes != nil {
					done <- errors.New("stale DNS answer or routes published")
					return
				}
				done <- err
			}()
			<-entered
			mutation := make(chan error, 1)
			go func() {
				if action == "update" {
					next := linuxTestSpec()
					next.Policy++
					mutation <- attachment.Update(context.Background(), next)
				} else {
					mutation <- attachment.Close(context.Background())
				}
			}()
			if err := controllerWait(t, mutation); err != nil {
				t.Fatal(err)
			}
			release <- struct{}{}
			if err := controllerWait(t, done); !errors.Is(err, context.Canceled) {
				t.Fatalf("late DNS: %v", err)
			}
		})
	}
}

func TestLinuxControllerCancelledUpdateCleansLateIngress(t *testing.T) {
	controller := NewLinuxTUNController()
	defer controller.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	closed := 0
	controller.start = func(_ context.Context, spec TUNSpec, _ TUNBackend) (linuxControllerIngress, error) {
		if spec.Policy == 2 {
			close(entered)
			<-release
		}
		return controllerTestIngress(func() error { closed++; return nil }), nil
	}
	attachment, err := controller.Attach(context.Background(), linuxTestSpec(), nil)
	if err != nil {
		t.Fatal(err)
	}
	next := linuxTestSpec()
	next.Policy++
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- attachment.Update(ctx, next) }()
	<-entered
	cancel()
	release <- struct{}{}
	if err := controllerWait(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update: %v", err)
	}
	if closed != 2 || controller.attachment != nil {
		t.Fatalf("late ingress leaked: closes=%d attached=%t", closed, controller.attachment != nil)
	}
	select {
	case <-attachment.Done():
	default:
		t.Fatal("cancelled attachment not finalized")
	}
}

func TestLinuxControllerCleanupFailureBlocksReplacement(t *testing.T) {
	failure := errors.New("synthetic system cleanup failure")
	controller := NewLinuxTUNController()
	starts := 0
	controller.start = func(context.Context, TUNSpec, TUNBackend) (linuxControllerIngress, error) {
		starts++
		return controllerTestIngress(func() error { return failure }), nil
	}
	attachment, err := controller.Attach(context.Background(), linuxTestSpec(), nil)
	if err != nil {
		t.Fatal(err)
	}
	next := linuxTestSpec()
	next.Policy++
	if err := attachment.Update(context.Background(), next); !errors.Is(err, failure) {
		t.Fatalf("update lost cleanup failure: %v", err)
	}
	if err := attachment.Close(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("detach lost cleanup failure: %v", err)
	}
	if _, err := controller.Attach(context.Background(), next, nil); !errors.Is(err, failure) {
		t.Fatalf("replacement accepted after unconfirmed cleanup: %v", err)
	}
	if err := controller.Close(); !errors.Is(err, failure) || starts != 1 {
		t.Fatalf("shutdown: err=%v starts=%d", err, starts)
	}
}
