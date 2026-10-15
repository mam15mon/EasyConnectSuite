package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"EasyConnectSuite/engine"
)

type pendingShutdownEngine struct {
	fakeVPNEngine
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (e *pendingShutdownEngine) Stop() {
	e.calls.Add(1)
	e.entered <- struct{}{}
	<-e.release
	e.fakeVPNEngine.Stop()
}

func TestShutdownRetainsTrayUntilCleanupAndRunsOnce(t *testing.T) {
	a := multiTestApp(t)
	e := &pendingShutdownEngine{entered: make(chan struct{}, 2), release: make(chan struct{})}
	if got := a.startConnection(func(opts engine.Options) (vpnEngine, error) {
		e.opts = opts
		opts.OnStatus(engine.Status{Phase: engine.PhaseReconnecting})
		return e, nil
	}); got != "OK" {
		t.Fatal(got)
	}
	// Shutdown must not send events to the frontend after its message loop exits.
	a.ctx = context.Background()

	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)
	trayIconMu.Lock()
	previousClosed := trayClosed
	trayClosed = false
	trayIconMu.Unlock()
	defer func() {
		trayIconMu.Lock()
		trayClosed = previousClosed
		trayIconMu.Unlock()
	}()

	done := make(chan struct{})
	go func() { a.shutdown(context.Background()); close(done) }()
	select {
	case <-e.entered:
	case <-time.After(time.Second):
		close(e.release)
		t.Fatal("shutdown did not reach the reconnecting engine")
	}
	trayIconMu.Lock()
	removedEarly := trayClosed
	trayIconMu.Unlock()
	if removedEarly {
		t.Error("tray disappeared while VPN cleanup was still pending")
	}
	if e.opts.Context.Err() == nil {
		t.Error("shutdown did not cancel reconnect work before waiting")
	}
	if got := a.startConnectionID("b", func(engine.Options) (vpnEngine, error) {
		t.Error("shutdown allowed another VPN engine to start")
		return nil, nil
	}); got == "OK" {
		t.Error("accepted a new connection during shutdown")
	}
	secondDone := make(chan struct{})
	go func() { a.shutdown(context.Background()); close(secondDone) }()
	select {
	case <-secondDone:
		t.Error("concurrent shutdown returned before cleanup finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(e.release)
	for _, finished := range []<-chan struct{}{done, secondDone} {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("shutdown did not finish after engine cleanup")
		}
	}
	if e.calls.Load() != 1 || a.GetStatus().Running {
		t.Fatal("shutdown repeated cleanup or retained a running connection")
	}
	trayIconMu.Lock()
	removed := trayClosed
	trayIconMu.Unlock()
	if !removed {
		t.Error("tray remained after cleanup completed")
	}
	for _, marker := range []string{"name=app_shutdown", "stage=disconnect_all", "stage=close_tun_worker", "stage=remove_tray", "outcome=returned", "elapsed=", "error_type="} {
		if !strings.Contains(logs.String(), marker) {
			t.Errorf("missing shutdown evidence %q", marker)
		}
	}
}
