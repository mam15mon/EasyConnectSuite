package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	"golang.org/x/sys/windows"
)

func TestWorkerExitFixture(t *testing.T) {
	value := os.Getenv("ECS_TEST_WORKER_EXIT")
	if value == "" {
		return
	}
	var release [1]byte
	io.ReadFull(os.Stdin, release[:])
	code, _ := strconv.Atoi(value)
	os.Exit(code)
}

func TestWorkerRecoveryWaitsForProcessCleanup(t *testing.T) {
	for _, code := range []int{0, 1} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestWorkerExitFixture$")
			cmd.Env = append(os.Environ(), "ECS_TEST_WORKER_EXIT="+strconv.Itoa(code))
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			release, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Wait()
			defer release.Close()
			handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(handle)
			done := make(chan error, 1)
			go func() { done <- waitTUNWorkerExit(handle) }()
			select {
			case err := <-done:
				t.Fatalf("cleanup did not wait: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			release.Write([]byte{1})
			select {
			case err := <-done:
				if (err != nil) != (code != 0) {
					t.Fatalf("exit=%d cleanup=%v", code, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cleanup wait did not finish")
			}
		})
	}
}

func TestWorkerLossPreservesCleanupErrorAndBlocksRelaunch(t *testing.T) {
	m := newTUNManager()
	left, right := net.Pipe()
	defer right.Close()
	peer := tunipc.NewPeer(left, nil, nil)
	defer peer.Close()
	m.peer = peer
	m.workerDone = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &remoteTUN{manager: m, ctx: ctx, cancel: cancel, done: make(chan struct{}), workerDone: m.workerDone, spec: engine.TUNSpec{Ref: tunipc.Ref{Attachment: "old"}}}
	m.attachments["old"] = a
	failure := errors.New("worker cleanup failed")
	peer.Close()
	m.lost(peer, failure)
	close(m.workerDone)
	if err := a.Close(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("cleanup cause lost: %v", err)
	}
	m.launch = func(string) (windows.Handle, error) {
		t.Error("relaunched before confirmed cleanup")
		return 0, errors.New("unexpected launch")
	}
	if err := m.ensure(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("relaunch guard=%v", err)
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("lost flows retained")
	}
}

type failedCleanupIngress struct{ failure error }

func (i *failedCleanupIngress) Close() error { return i.failure }
func (i *failedCleanupIngress) UpdateSpec(context.Context, engine.TUNSpec) error {
	return errors.New("policy update failed")
}
func TestWorkerRetainsCleanupFailureUntilExit(t *testing.T) {
	for _, op := range []string{"update", "detach", "shutdown"} {
		t.Run(op, func(t *testing.T) {
			failure := errors.New("route cleanup failed")
			ref := tunipc.Ref{Profile: "p", Session: "s", Attachment: "a", Policy: 1}
			spec := engine.TUNSpec{Ref: ref}
			w := &workerServer{authenticated: true, closing: make(chan struct{}), entries: map[string]*workerEntry{"a": {spec: spec, ingress: &failedCleanupIngress{failure: failure}, backend: &workerBackend{ref: ref}}}}
			if op == "update" {
				spec.Policy++
			}
			_, err := w.handle(context.Background(), tunipc.Message{Op: op, Ref: spec.Ref, Spec: &spec})
			if !errors.Is(err, failure) {
				t.Fatalf("operation lost cleanup cause: %v", err)
			}
			if err := w.cleanup(); !errors.Is(err, failure) {
				t.Fatalf("exit concealed previous cleanup failure: %v", err)
			}
		})
	}
}
