package main

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	if err := setupDarwinWorkerCrashLog(os.Getenv("ECS_DARWIN_LOG_DIR")); err != nil {
		os.Exit(3)
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
	unsafeDir := filepath.Join(t.TempDir(), "unsafe-logs")
	if err := os.Mkdir(unsafeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "other-file"), filepath.Join(unsafeDir, "tun-worker-crash.log")); err != nil {
		t.Fatal(err)
	}
	if err := setupDarwinWorkerCrashLog(unsafeDir); err == nil {
		t.Fatal("worker crash log accepted a symbolic link")
	}
	logDir := filepath.Join(t.TempDir(), "worker-logs")
	m := newTUNManager()
	m.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(exe, "-test.run=^TestDarwinWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_DARWIN_WORKER_HELPER=1", "ECS_DARWIN_SOCKET="+name, "ECS_DARWIN_PARENT="+strconv.Itoa(os.Getpid()), "ECS_DARWIN_LOG_DIR="+logDir)
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
	spec.Ref.Policy++
	spec.Prefixes = []string{"198.18.251.46/32"}
	if err = attachment.Update(ctx, spec); err != nil {
		t.Fatalf("replace worker policy: %v", err)
	}
	conn, err := net.DialTimeout("tcp4", "198.18.251.46:43210", 5*time.Second)
	if err != nil {
		t.Fatalf("updated route dial: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = conn.Write([]byte("updated")); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	var updated [7]byte
	if _, err = io.ReadFull(conn, updated[:]); err != nil || string(updated[:]) != "updated" {
		conn.Close()
		t.Fatalf("updated route reply %q: %v", updated, err)
	}
	conn.Close()
	if got := <-backend.calls; got != "tcp 198.18.251.46:43210" {
		t.Fatalf("unexpected updated flow %q", got)
	}
	secondBackend := &macWorkerEcho{calls: make(chan string, 2)}
	secondSpec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "system-test-2", Session: "2", Attachment: tunipc.NewName(), Policy: 1}, Slot: 61, Prefixes: []string{"198.18.251.47/32"}}
	second, err := m.Attach(ctx, secondSpec, secondBackend)
	if err != nil {
		t.Fatalf("attach second worker ingress: %v", err)
	}
	if err = attachment.Close(ctx); err != nil {
		t.Fatalf("detach first worker ingress: %v", err)
	}
	for _, network := range []string{"tcp", "udp"} {
		conn, err := net.DialTimeout(network+"4", "198.18.251.47:43210", 5*time.Second)
		if err != nil {
			t.Fatalf("remaining %s dial: %v", network, err)
		}
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err = conn.Write([]byte("second")); err != nil {
			conn.Close()
			t.Fatalf("remaining %s write: %v", network, err)
		}
		var reply [6]byte
		if _, err = io.ReadFull(conn, reply[:]); err != nil || string(reply[:]) != "second" {
			conn.Close()
			t.Fatalf("remaining %s reply %q: %v", network, reply, err)
		}
		conn.Close()
		if got := <-secondBackend.calls; got != network+" 198.18.251.47:43210" {
			t.Fatalf("unexpected remaining worker flow %q", got)
		}
	}
	if err = second.Close(ctx); err != nil {
		t.Fatalf("detach second worker ingress: %v", err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("worker failed to exit after shutdown")
	}
	data, err := os.ReadFile(filepath.Join(logDir, "tun-worker-crash.log"))
	if err != nil || !strings.Contains(string(data), "operation=") {
		t.Fatalf("worker operation evidence missing from crash log: %v", err)
	}
	previousSize := len(data)
	if err = m.Prepare(ctx, "disconnected-test"); err != nil {
		t.Fatalf("prepare worker for disconnected IPC: %v", err)
	}
	lostSpec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "disconnected-test", Session: "3", Attachment: tunipc.NewName(), Policy: 1}, Slot: 62, Prefixes: []string{"198.18.251.48/32"}}
	if _, err = m.Attach(ctx, lostSpec, &macWorkerEcho{calls: make(chan string, 1)}); err != nil {
		t.Fatalf("attach before IPC disconnect: %v", err)
	}
	m.mu.Lock()
	peer, lostDone := m.peer, m.workerDone
	m.mu.Unlock()
	peer.Close()
	select {
	case <-lostDone:
	case <-ctx.Done():
		t.Fatal("worker failed to clean up after IPC disconnect")
	}
	data, err = os.ReadFile(filepath.Join(logDir, "tun-worker-crash.log"))
	if err != nil || len(data) <= previousSize || !strings.Contains(string(data[previousSize:]), "name=darwin_tun_release") {
		t.Fatalf("cleanup evidence missing after IPC disconnect: %v", err)
	}
}
