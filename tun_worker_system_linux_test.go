package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"EasyConnectSuite/internal/tunipc"
)

func TestLinuxWorkerHelper(t *testing.T) {
	if os.Getenv("ECS_LINUX_WORKER_HELPER") != "1" {
		return
	}
	if err := runTUNWorker([]string{"--tun-worker", os.Getenv("ECS_LINUX_WORKER_SOCKET"), os.Getenv("ECS_LINUX_PARENT_PID")}); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxWorkerProcessLifecycle(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	name := tunipc.NewName()
	listener, err := tunipc.Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
	cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	conn, pid, err := listener.AcceptWorker(ctx, executable)
	if err != nil {
		t.Fatal(err)
	}
	if pid != uint32(cmd.Process.Pid) {
		conn.Close()
		t.Fatalf("unexpected worker PID %d", pid)
	}
	peer := tunipc.NewPeer(conn, nil, nil)
	defer peer.Close()
	if _, err = peer.Call(ctx, tunipc.Message{Op: "hello"}); err != nil {
		t.Fatal(err)
	}
	ref := tunipc.Ref{Profile: "test", Session: "test", Attachment: "test", Policy: 1}
	spec := tunipc.Spec{Ref: ref, Slot: 5, Prefixes: []string{"198.18.71.0/24"}}
	if _, err = peer.Call(ctx, tunipc.Message{Op: "prepare", Ref: ref}); err != nil {
		t.Fatal(err)
	}
	if _, err = peer.Call(ctx, tunipc.Message{Op: "attach", Ref: ref, Spec: &spec}); err != nil {
		t.Fatal(err)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			found = true
		}
	}
	if !found {
		t.Fatal("worker did not create an interface")
	}
	// Simulate the parent disappearing while the attachment is active.
	peer.Close()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err = <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not exit after control channel closed")
	}
	ifaces, err = net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			t.Fatal("worker left an interface behind")
		}
	}
}
