package main

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDarwinWorkerAuthorizationScriptCompiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/osacompile", "-o", filepath.Join(t.TempDir(), "worker.scpt"), "-e", darwinWorkerScript).CombinedOutput()
	if err != nil {
		t.Fatalf("compile worker authorization AppleScript: %s: %v", out, err)
	}
}

func TestDarwinCanceledAuthorizationWaitsForProcessExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := newTUNManager()
	var stdin io.WriteCloser
	launches := 0
	manager.launch = func(string) (*exec.Cmd, error) {
		launches++
		cmd := exec.Command("/bin/cat")
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		if err = cmd.Start(); err != nil {
			stdin.Close()
			return nil, err
		}
		cancel()
		return cmd, nil
	}
	if err := manager.ensure(ctx); err == nil {
		t.Fatal("canceled authorization succeeded")
	}
	t.Cleanup(func() { _ = stdin.Close() })
	if err := manager.ensure(context.Background()); err == nil || launches != 1 {
		t.Fatalf("new authorization started before prior process exit: %v, launches=%d", err, launches)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		manager.mu.Lock()
		recoveryErr := manager.recoveryErr
		manager.mu.Unlock()
		if recoveryErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("authorization process exit did not clear wait: %v", recoveryErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
