package main

import (
	"EasyConnectSuite/engine"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type fakeVPNEngine struct {
	stopped atomic.Bool
	opts    engine.Options
}

func (e *fakeVPNEngine) Running() bool { return !e.stopped.Load() }
func (e *fakeVPNEngine) Stop() {
	e.stopped.Store(true)
	e.opts.OnStatus(engine.Status{Phase: engine.PhaseDisconnected})
}
func (e *fakeVPNEngine) Resources() []engine.ResourceEntry {
	return []engine.ResourceEntry{{Target: e.opts.Config.ServerAddress}}
}
func multiTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{configPath: filepath.Join(t.TempDir(), "profiles.json"), appConfig: AppConfig{ActiveProfileID: "a", Profiles: []Profile{
		{ID: "a", Name: "A", Server: "a.invalid", Port: 443, Username: "test", Protocol: "easyconnect"},
		{ID: "b", Name: "B", Server: "b.invalid", Port: 443, Username: "test", Protocol: "univpn"}}}}
	a.normalizeProfiles()
	return a
}
func TestConcurrentProfilesRemainIndependent(t *testing.T) {
	a := multiTestApp(t)
	a.appConfig.Profiles[1].InsecureSkipVerify = true
	engines := map[string]*fakeVPNEngine{}
	start := func(o engine.Options) (vpnEngine, error) {
		e := &fakeVPNEngine{opts: o}
		engines[o.Config.ServerAddress] = e
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected, ClientIP: "10.0.0.2"})
		return e, nil
	}
	if got := a.startConnection(start); got != "OK" {
		t.Fatal(got)
	}
	a.SetActiveProfile("b")
	if a.GetStatus().Running {
		t.Fatal("selected unconnected profile shows the other connection")
	}
	if got := a.startConnection(start); got != "OK" {
		t.Fatal(got)
	}
	if engines["a.invalid"].opts.TUNSlot == engines["b.invalid"].opts.TUNSlot {
		t.Fatal("shared TUN slot")
	}
	if engines["a.invalid"].opts.Config.SocksBind == engines["b.invalid"].opts.Config.SocksBind {
		t.Fatal("shared proxy port")
	}
	if engines["a.invalid"].opts.InsecureSkipVerify || !engines["b.invalid"].opts.InsecureSkipVerify {
		t.Fatal("certificate option crossed profiles or was not propagated")
	}
	if !a.GetStatus().Running || len(a.GetConnections()) != 2 {
		t.Fatal("second connection missing")
	}
	a.StopVPN()
	if engines["a.invalid"].stopped.Load() {
		t.Fatal("disconnecting B stopped A")
	}
	if engines["a.invalid"].opts.Context.Err() != nil {
		t.Fatal("disconnecting B cancelled A")
	}
	a.SetActiveProfile("a")
	if !a.GetStatus().Running || a.GetVPNResources()[0].Target != "a.invalid" {
		t.Fatal("lost A status/resources")
	}
	// A delayed event from an old B connection must not overwrite selected A.
	engines["b.invalid"].opts.OnStatus(engine.Status{Phase: engine.PhaseConnected, ClientIP: "10.0.0.9"})
	if a.GetStatus().ProfileID != "a" || a.GetStatus().ClientIP != "10.0.0.2" {
		t.Fatal("crossed profile events")
	}
	a.StopAllVPN()
	if !engines["a.invalid"].stopped.Load() {
		t.Fatal("exit retained a connection")
	}
}
func TestLoginFailureDoesNotDropOtherProfile(t *testing.T) {
	a := multiTestApp(t)
	if got := a.startConnection(func(o engine.Options) (vpnEngine, error) {
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return &fakeVPNEngine{opts: o}, nil
	}); got != "OK" {
		t.Fatal(got)
	}
	a.SetActiveProfile("b")
	if got := a.startConnection(func(engine.Options) (vpnEngine, error) { return nil, errors.New("authentication rejected") }); got != "authentication rejected" {
		t.Fatal(got)
	}
	a.SetActiveProfile("a")
	if !a.GetStatus().Running {
		t.Fatal("failed B changed A")
	}
	a.StopAllVPN()
}
func TestCancelStartingConnectionAndWaitForCleanup(t *testing.T) {
	a := multiTestApp(t)
	entered := make(chan struct{})
	finished := make(chan string, 1)
	go func() {
		finished <- a.startConnection(func(o engine.Options) (vpnEngine, error) {
			close(entered)
			<-o.Context.Done()
			return nil, o.Context.Err()
		})
	}()
	<-entered
	if got := a.StopVPN(); got != "OK" {
		t.Fatal(got)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("start not cancelled")
	}
	if a.GetStatus().Running || a.GetStatus().State != "disconnected" {
		t.Fatal("cancel became an error or stayed connected")
	}
}
func TestProtocolAndPortsPersistAndOldProfilesMigrate(t *testing.T) {
	a := multiTestApp(t)
	if a.appConfig.Profiles[0].SocksPort != 1080 || a.appConfig.Profiles[1].SocksPort != 1082 || a.appConfig.Profiles[1].InsecureSkipVerify {
		t.Fatal("incorrect migration")
	}
	p, err := a.SaveProfile(Profile{Name: "Uni", Server: "test.invalid", Port: 443, Username: "test", Protocol: "univpn", Domain: "test-domain", InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.SocksPort != 1084 || p.HTTPPort != 1085 {
		t.Fatal("new profile reused ports")
	}
	loaded := multiTestApp(t)
	loaded.configPath = a.configPath
	loaded.loadConfigFile()
	got := loaded.GetActiveProfile()
	if got.Protocol != "univpn" || got.Domain != "test-domain" || got.HTTPPort != 1085 || !got.InsecureSkipVerify {
		t.Fatal("lost protocol fields")
	}
	conf := a.buildEngineConfig(got)
	if conf.Protocol != "univpn" || !conf.DisableRemoteDNS || !conf.DisableKeepAlive || !conf.DisableZJUConfig {
		t.Fatal("UniVPN claims unsupported DNS/keepalive")
	}
	_, err = a.SaveProfile(Profile{Port: 443, Protocol: "unknown"})
	if err == nil {
		t.Fatal("unknown protocol accepted")
	}
	_, err = a.SaveProfile(Profile{Port: 443, Protocol: "univpn", Enable2FA: true})
	if err == nil {
		t.Fatal("unsupported auth accepted")
	}
}

func TestTopSAPProfileKeepsResourceRoutingAndExistingProtocols(t *testing.T) {
	a := multiTestApp(t)
	p, err := a.SaveProfile(Profile{Name: "TopSAP", Protocol: "topsec", Server: "test.invalid", Port: 443, Username: "test", Password: " space ", TUNMode: true})
	if err != nil {
		t.Fatal(err)
	}
	conf := a.buildEngineConfig(p)
	if conf.Protocol != "topsec" || !conf.DisableZJUConfig || !conf.DisableKeepAlive || conf.Password != " space " || conf.SocksBind == "" || conf.HTTPBind == "" {
		t.Fatal("incorrect TopSAP profile mapping")
	}
	if a.appConfig.Profiles[0].Protocol != "easyconnect" || a.appConfig.Profiles[1].Protocol != "univpn" {
		t.Fatal("existing profiles changed")
	}
	if _, err := a.SaveProfile(Profile{Protocol: "topsec", Port: 443, Enable2FA: true}); err == nil {
		t.Fatal("unsupported authentication accepted")
	}
}

func TestStoppingTopSAPRetainsEasyConnectAndUniVPN(t *testing.T) {
	a := multiTestApp(t)
	p, err := a.SaveProfile(Profile{Name: "TopSAP", Protocol: "topsec", Server: "top.invalid", Port: 443, Username: "test", InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	engines := map[string]*fakeVPNEngine{}
	start := func(o engine.Options) (vpnEngine, error) {
		e := &fakeVPNEngine{opts: o}
		engines[o.Config.Protocol] = e
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return e, nil
	}
	for _, id := range []string{"a", "b", p.ID} {
		if ok := a.SetActiveProfile(id); !ok {
			t.Fatal("could not select profile", id)
		}
		if got := a.startConnection(start); got != "OK" {
			t.Fatal(got)
		}
	}
	defer a.StopAllVPN()
	if engines["topsec"].opts.TUNSlot == engines["univpn"].opts.TUNSlot || engines["topsec"].opts.Config.SocksBind == engines["easyconnect"].opts.Config.SocksBind || !engines["topsec"].opts.InsecureSkipVerify || engines["univpn"].opts.InsecureSkipVerify {
		t.Fatal("TopSAP session shares slot/port/certificate option")
	}
	if got := a.StopVPN(); got != "OK" {
		t.Fatal(got)
	}
	for _, protocol := range []string{"easyconnect", "univpn"} {
		e := engines[protocol]
		if e.stopped.Load() || e.opts.Context.Err() != nil {
			t.Fatal("TopSAP stop interrupted", protocol)
		}
	}
	if !engines["topsec"].stopped.Load() {
		t.Fatal("TopSAP did not stop")
	}
}
func TestElevationDoesNotInterruptOtherConnections(t *testing.T) {
	a := multiTestApp(t)
	a.startConnection(func(o engine.Options) (vpnEngine, error) {
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return &fakeVPNEngine{opts: o}, nil
	})
	a.SetActiveProfile("b")
	launched := false
	if _, err := a.setTUNMode(true, false, func(string) error { launched = true; return nil }); err == nil || launched {
		t.Fatal("elevation interrupted active connections")
	}
	a.StopAllVPN()
}

func TestEditingStoppedProfileUpdatesStatusMetadata(t *testing.T) {
	a := multiTestApp(t)
	a.startConnection(func(o engine.Options) (vpnEngine, error) {
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return &fakeVPNEngine{opts: o}, nil
	})
	a.StopVPN()
	p := a.GetActiveProfile()
	p.Name = "Renamed"
	p.SocksPort = 2080
	p.HTTPPort = 2081
	if _, err := a.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	status := a.GetStatus()
	if status.ActiveProfile != "Renamed" || status.SocksPort != "2080" || status.Running {
		t.Fatal("stopped status retained stale settings")
	}
}
