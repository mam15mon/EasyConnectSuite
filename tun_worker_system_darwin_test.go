package main

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
)

type macWorkerEcho struct{ calls chan string }

func (b *macWorkerEcho) Dial(_ context.Context, network, addr string) (net.Conn, error) {
	b.calls <- network + " " + addr
	a, c := net.Pipe()
	go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
	return a, nil
}
func (*macWorkerEcho) DNS(context.Context, []byte) ([]byte, []string, error) { return nil, nil, nil }

func TestDarwinWorkerHelper(t *testing.T) {
	if os.Getenv("ECS_DARWIN_WORKER_HELPER") != "1" {
		return
	}
	err := runTUNWorker([]string{"--tun-worker", os.Getenv("ECS_DARWIN_SOCKET"), os.Getenv("ECS_DARWIN_PARENT")})
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestDarwinWorkerProcessIdentity(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	if os.Geteuid() != 0 {
		t.Fatal("system test requires root")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := newTUNManager()
	m.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(exe, "-test.run=^TestDarwinWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_DARWIN_WORKER_HELPER=1", "ECS_DARWIN_SOCKET="+name, "ECS_DARWIN_PARENT="+strconv.Itoa(os.Getpid()))
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = m.Prepare(ctx, "system-test"); err != nil {
		t.Fatalf("prepare worker: %v", err)
	}
	m.mu.Lock()
	done := m.workerDone
	m.mu.Unlock()
	if done == nil {
		t.Fatal("authenticated worker never reached handshake")
	}
	backend := &macWorkerEcho{calls: make(chan string, 2)}
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "system-test", Session: "1", Attachment: tunipc.NewName(), Policy: 1}, Slot: 62, Prefixes: []string{"198.18.251.45/32"}}
	attachment, err := m.Attach(ctx, spec, backend)
	if err != nil {
		t.Fatalf("attach worker: %v", err)
	}
	for _, network := range []string{"tcp", "udp"} {
		conn, err := net.DialTimeout(network+"4", "198.18.251.45:43210", 5*time.Second)
		if err != nil {
			t.Fatalf("%s dial: %v", network, err)
		}
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err = conn.Write([]byte("worker-flow")); err != nil {
			conn.Close()
			t.Fatalf("%s write: %v", network, err)
		}
		var reply [11]byte
		if _, err = io.ReadFull(conn, reply[:]); err != nil || string(reply[:]) != "worker-flow" {
			conn.Close()
			t.Fatalf("%s reply %q: %v", network, reply, err)
		}
		conn.Close()
		if got := <-backend.calls; got != network+" 198.18.251.45:43210" {
			t.Fatalf("unexpected worker flow %q", got)
		}
	}
	if err = attachment.Close(ctx); err != nil {
		t.Fatalf("detach worker: %v", err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("worker failed to exit after rejected handshake")
	}
}
