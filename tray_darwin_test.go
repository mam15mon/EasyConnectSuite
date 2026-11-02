package main

import (
	"context"
	"reflect"
	"testing"
)

func TestDarwinSecondLaunchRestoresWindow(t *testing.T) {
	originalShow, originalRestore := showDarwinWindow, restoreDarwinWindow
	defer func() { showDarwinWindow, restoreDarwinWindow = originalShow, originalRestore }()
	ctx := context.Background()
	app := &App{ctx: ctx}
	var calls []string
	showDarwinWindow = func(got context.Context) {
		if got != ctx || !app.mu.TryLock() {
			t.Fatal("window show held app lock or used wrong context")
		}
		app.mu.Unlock()
		calls = append(calls, "show")
	}
	restoreDarwinWindow = func(got context.Context) {
		if got != ctx || !app.mu.TryLock() {
			t.Fatal("window restore held app lock or used wrong context")
		}
		app.mu.Unlock()
		calls = append(calls, "restore")
	}
	showTrayWorkspace(app)
	if !reflect.DeepEqual(calls, []string{"show", "restore"}) {
		t.Fatalf("window calls: %v", calls)
	}
	app.shuttingDown = true
	showTrayWorkspace(app)
	if len(calls) != 2 {
		t.Fatalf("window restored while shutting down: %v", calls)
	}
}
