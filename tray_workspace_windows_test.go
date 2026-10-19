package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"
)

func TestTrayWorkspaceAllowsSynchronousWindowReentry(t *testing.T) {
	for _, reenterAt := range []string{"show", "restore"} {
		t.Run(reenterAt, func(t *testing.T) {
			a := &App{ctx: context.Background()}
			previousShow, previousRestore := showTrayWindow, restoreTrayWindow
			defer func() { showTrayWindow, restoreTrayWindow = previousShow, previousRestore }()
			var logs bytes.Buffer
			previousWriter := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(previousWriter)

			var calls []string
			reentered := false
			windowAction := func(stage string, ctx context.Context) {
				if ctx != a.ctx {
					t.Error("window action lost the startup context")
				}
				if !a.mu.TryLock() {
					t.Errorf("application lock held during window %s", stage)
					return
				}
				a.mu.Unlock()
				calls = append(calls, stage)
				if stage == reenterAt && !reentered {
					reentered = true
					showTrayWorkspace(a)
				}
				// Both the right-click menu and connection status reads need this lock.
				_ = a.trayStatuses()
				_ = a.GetConnections()
			}
			showTrayWindow = func(ctx context.Context) { windowAction("show", ctx) }
			restoreTrayWindow = func(ctx context.Context) { windowAction("restore", ctx) }
			done := make(chan struct{})
			go func() { showTrayWorkspace(a); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("workspace opening deadlocked during a synchronous window callback")
			}
			want := "show,show,restore,restore"
			if reenterAt == "restore" {
				want = "show,restore,show,restore"
			}
			if !reentered || strings.Join(calls, ",") != want {
				t.Fatalf("window callback did not finish both workspace openings: %v", calls)
			}
			for _, marker := range []string{"[VPN pid=", "name=tray_workspace", "goroutine=", "stage=wait_app_lock", "stage=window_show", "stage=window_restore", "elapsed=", "canceled=false", "error_type=<nil>"} {
				if !strings.Contains(logs.String(), marker) {
					t.Errorf("missing workspace diagnostic %q", marker)
				}
			}
			if got := strings.Count(logs.String(), "phase=end outcome=shown"); got != 2 {
				t.Fatalf("workspace operations did not both finish: %d", got)
			}
		})
	}
}

func TestTrayWorkspaceSkipsUnavailableOrShuttingDownFrontend(t *testing.T) {
	previousShow, previousRestore := showTrayWindow, restoreTrayWindow
	defer func() { showTrayWindow, restoreTrayWindow = previousShow, previousRestore }()
	showTrayWindow = func(context.Context) { t.Error("showed an unavailable or shutting-down frontend") }
	restoreTrayWindow = func(context.Context) { t.Error("restored an unavailable or shutting-down frontend") }
	for _, a := range []*App{{}, {ctx: context.Background(), shuttingDown: true}} {
		showTrayWorkspace(a)
		if !a.mu.TryLock() {
			t.Fatal("skipped workspace opening retained the application lock")
		}
		a.mu.Unlock()
	}
}
