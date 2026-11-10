package main

import (
	"context"
	"fmt"
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
	peer := tunipc.NewPeer(conn, func(ctx context.Context, msg tunipc.Message) (tunipc.Message, error) {
		if msg.Op != "dial" || (msg.Network != "tcp" && msg.Network != "udp") || msg.Ref.Attachment != "test" {
			return tunipc.Message{}, fmt.Errorf("unexpected worker request")
		}
		dataName := tunipc.NewName()
		dataListener, err := tunipc.Listen(dataName)
		if err != nil {
			return tunipc.Message{}, err
		}
		go func() {
			defer dataListener.Close()
			acceptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			dataConn, err := dataListener.Accept(acceptCtx, pid)
			if err != nil {
				return
			}
			defer dataConn.Close()
			stream := tunipc.NewDataConn(dataConn, msg.Network == "udp")
			buf := make([]byte, 2048)
			for {
				n, err := stream.Read(buf)
				if err != nil {
					return
				}
				if _, err = stream.Write(buf[:n]); err != nil {
					return
				}
			}
		}()
		return tunipc.Message{Pipe: dataName}, nil
	}, nil)
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
	for _, network := range []string{"tcp4", "udp4"} {
		flow, err := net.DialTimeout(network, "198.18.71.2:12345", 5*time.Second)
		if err != nil {
			t.Fatalf("%s dial: %v", network, err)
		}
		_ = flow.SetDeadline(time.Now().Add(5 * time.Second))
		payload := []byte("worker-" + network)
		if _, err = flow.Write(payload); err != nil {
			flow.Close()
			t.Fatal(err)
		}
		reply := make([]byte, len(payload))
		if _, err = io.ReadFull(flow, reply); err != nil {
			flow.Close()
			t.Fatalf("%s read: %v", network, err)
		}
		flow.Close()
		if string(reply) != string(payload) {
			t.Fatalf("%s payload mismatch", network)
		}
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

type linuxManagerEcho struct{}

func (linuxManagerEcho) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	a, b := net.Pipe()
	go func() { defer b.Close(); _, _ = io.Copy(b, b) }()
	return a, nil
}
func (linuxManagerEcho) DNS(context.Context, []byte) ([]byte, []string, error) { return nil, nil, nil }

func TestLinuxManagerWorkerLifecycle(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	manager := newLinuxTUNManager()
	manager.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "manager", Session: "session", Attachment: "attachment", Policy: 1}, Slot: 7, Prefixes: []string{"198.18.72.0/24"}}
	if err := manager.Prepare(ctx, spec.Profile); err != nil {
		t.Fatal(err)
	}
	attachment, err := manager.Attach(ctx, spec, linuxManagerEcho{})
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"tcp4", "udp4"} {
		conn, err := net.DialTimeout(network, "198.18.72.2:23456", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		payload := []byte("manager-" + network)
		if _, err = conn.Write(payload); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		result := make([]byte, len(payload))
		if _, err = io.ReadFull(conn, result); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		conn.Close()
		if string(result) != string(payload) {
			t.Fatalf("unexpected %s data", network)
		}
	}
	if err := attachment.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-attachment.Done():
	case <-ctx.Done():
		t.Fatal("attachment did not stop")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			t.Fatal("manager left an interface behind")
		}
	}
}
