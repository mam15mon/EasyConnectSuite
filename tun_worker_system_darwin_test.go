package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

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
	err = m.Prepare(ctx, "system-test")
	if err == nil || !strings.Contains(err.Error(), "残留清理尚未实现") {
		t.Fatalf("worker must reject incomplete cleanup: %v", err)
	}
	m.mu.Lock()
	done := m.workerDone
	m.mu.Unlock()
	if done == nil {
		t.Fatal("authenticated worker never reached handshake")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("worker failed to exit after rejected handshake")
	}
}
