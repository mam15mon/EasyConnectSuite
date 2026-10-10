package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestEngineCleanupIsLocalAndIdempotent(t *testing.T) {
	var a, b Engine
	closedA, closedB := 0, 0
	a.registerTerminal("A", func(context.Context) error { closedA++; return nil })
	b.registerTerminal("B", func(context.Context) error { closedB++; return nil })
	a.execTerminal()
	a.execTerminal()
	if closedA != 1 || closedB != 0 {
		t.Fatal("cleanup crossed engines")
	}
	a.registerTerminal("late A", func(context.Context) error { closedA++; return nil })
	if closedA != 2 {
		t.Fatal("resource acquired after startup failure leaked")
	}
	b.execTerminal()
	if closedB != 1 {
		t.Fatal("second cleanup missing")
	}
}
func TestSharedLogSurvivesAnotherEngineStopping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.log")
	a := &Engine{opts: Options{LogFile: path}}
	b := &Engine{opts: Options{LogFile: path}}
	a.startLogCapture()
	b.startLogCapture()
	a.stopLogCapture()
	if _, err := engineLogs.Write([]byte("second VPN remains active\n")); err != nil {
		t.Fatal(err)
	}
	b.stopLogCapture()
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "second VPN remains active") {
		t.Fatalf("log lost: %s %v", data, err)
	}
	// Concurrent registrations and writes must not invert the standard logger lock.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := &Engine{}
			e.startLogCapture()
			engineLogs.Write([]byte("parallel log\n"))
			e.stopLogCapture()
		}()
	}
	wg.Wait()
}

func TestLifecycleAndCleanupCausePersistWithConnectionIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.log")
	e := &Engine{id: 19, opts: Options{ConnectionID: "synthetic-profile", LogFile: path}}
	e.startLogCapture()
	e.emitStatusFull(Status{Phase: PhaseReconnecting, Message: "network timeout"})
	e.registerTerminal("CloseSynthetic", func(context.Context) error { return fmt.Errorf("synthetic cleanup refusal") })
	e.Stop()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`engine=19 profile="synthetic-profile"`, "phase=reconnecting", `message="network timeout"`, "shutdown reason=stop_requested", "cleanup complete step=CloseSynthetic", "cause=synthetic cleanup refusal", "phase=disconnected"} {
		if !strings.Contains(string(data), marker) {
			t.Fatal("missing durable lifecycle diagnostic", marker)
		}
	}
}
