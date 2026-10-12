package main

import (
	"EasyConnectSuite/engine"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeTUNControl struct {
	prepared int
	failure  error
	prepare  func()
}

type hotTUNTestEngine struct {
	fakeVPNEngine
	state engine.TUNStatus
}

func (e *hotTUNTestEngine) SetTUN(ctx context.Context, enabled bool) error {
	e.state = engine.TUNStatus{State: "off"}
	if enabled {
		e.state.State = "on"
	}
	e.opts.OnTUNStatus(e.state)
	return nil
}
func (e *hotTUNTestEngine) TUNState() engine.TUNStatus { return e.state }
func TestAppHotTUNSwitchPreservesConnectionStatus(t *testing.T) {
	a := multiTestApp(t)
	var e *hotTUNTestEngine
	if got := a.startConnection(func(o engine.Options) (vpnEngine, error) {
		e = &hotTUNTestEngine{fakeVPNEngine: fakeVPNEngine{opts: o}}
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected, ClientIP: "192.0.2.19"})
		return e, nil
	}); got != "OK" {
		t.Fatal(got)
	}
	defer a.StopAllVPN()
	before := a.GetStatus()
	for _, enabled := range []bool{true, false} {
		if err := a.setProfileTUNMode("a", enabled, &fakeTUNControl{}); err != nil {
			t.Fatal(err)
		}
		s := a.GetStatus()
		if s.State != "connected" || !s.Running || s.ClientIP != before.ClientIP || s.SocksPort != before.SocksPort || s.HTTPPort != before.HTTPPort || s.TUNMode != enabled || e.stopped.Load() {
			t.Fatalf("hot switch altered VPN %+v", s)
		}
	}
}

func (f *fakeTUNControl) Prepare(context.Context, string) error {
	f.prepared++
	if f.prepare != nil {
		f.prepare()
	}
	return f.failure
}
func (f *fakeTUNControl) Release(context.Context, string) error { return nil }
func (f *fakeTUNControl) Close()                                {}
func (f *fakeTUNControl) Attach(context.Context, engine.TUNSpec, engine.TUNBackend) (engine.TUNAttachment, error) {
	return nil, errors.New("not used")
}
func tunModeTestApp(t *testing.T) *App {
	return &App{configPath: filepath.Join(t.TempDir(), "配置 with spaces.json"), appConfig: AppConfig{ActiveProfileID: "second", Profiles: []Profile{{ID: "first"}, {ID: "second"}}}}
}
func readTUNTestConfig(t *testing.T, a *App) AppConfig {
	t.Helper()
	data, e := os.ReadFile(a.configPath)
	if e != nil {
		t.Fatal(e)
	}
	var c AppConfig
	if e = json.Unmarshal(data, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func TestTUNPreparationSavesBeforeUACAndReleasesAppLock(t *testing.T) {
	a := tunModeTestApp(t)
	f := &fakeTUNControl{prepare: func() {
		if !readTUNTestConfig(t, a).Profiles[1].TUNMode {
			t.Fatal("not saved before launch")
		}
		if !a.mu.TryLock() {
			t.Fatal("App lock held during UAC")
		}
		a.mu.Unlock()
	}}
	if e := a.setProfileTUNMode("second", true, f); e != nil {
		t.Fatal(e)
	}
	if readTUNTestConfig(t, a).Profiles[0].TUNMode {
		t.Fatal("other profile modified")
	}
}
func TestTUNPreparationCancellationRollsBackOnlyTarget(t *testing.T) {
	a := tunModeTestApp(t)
	denied := errors.New("canceled UAC")
	f := &fakeTUNControl{failure: denied}
	if e := a.setProfileTUNMode("first", true, f); !errors.Is(e, denied) {
		t.Fatal(e)
	}
	if readTUNTestConfig(t, a).Profiles[0].TUNMode {
		t.Fatal("failed intent retained")
	}
}
func TestTUNSaveFailureDoesNotLaunchWorker(t *testing.T) {
	a := tunModeTestApp(t)
	a.configPath = filepath.Join(t.TempDir(), "missing", "profiles.json")
	f := &fakeTUNControl{}
	if e := a.setProfileTUNMode("first", true, f); e == nil || f.prepared != 0 {
		t.Fatal("launched without saving")
	}
}
