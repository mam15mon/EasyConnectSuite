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

func TestDarwinTraySnapshotIncludesIndependentActions(t *testing.T) {
	statuses := []VPNStatus{
		{ProfileID: "first", ActiveProfile: "甲", State: "connected", Running: true},
		{ProfileID: "second", ActiveProfile: "乙", State: "error"},
		{ProfileID: "third", ActiveProfile: "丙", State: "stopping", Running: true},
	}
	menu := darwinTraySnapshot(statuses)
	if !menu.Running || len(menu.Profiles) != 3 {
		t.Fatalf("unexpected menu: %+v", menu)
	}
	if menu.Profiles[0].ID != "first" || menu.Profiles[0].Action != "disconnect" || menu.Profiles[1].ID != "second" || menu.Profiles[1].Action != "connect" || !menu.Profiles[2].Disabled {
		t.Fatalf("incorrect profile actions: %+v", menu.Profiles)
	}
}
