package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"EasyConnectSuite/engine"
)

func TestTUNProfileUsesServerResourcesAndRetainsLocalProxies(t *testing.T) {
	app := &App{}
	conf := app.buildEngineConfig(Profile{Server: "vpn.example", Username: "user", TUNMode: true, DisableZjuConfig: false})
	if conf.TUNMode || conf.AddRoute || conf.DisableZJUConfig {
		t.Fatal("TUN preference must not replace the proxy policy")
	}
	if conf.SocksBind != "127.0.0.1:1080" || conf.HTTPBind != "127.0.0.1:1081" {
		t.Fatal("local proxies missing")
	}
	if !conf.AutoDetectInterface {
		t.Fatal("empty interface must auto-detect")
	}
}

func TestExistingProfilesStayInProxyMode(t *testing.T) {
	var profile Profile
	if err := json.Unmarshal([]byte(`{"server":"vpn.example","username":"user","bindInterface":"WLAN","disableZjuConfig":true}`), &profile); err != nil {
		t.Fatal(err)
	}
	conf := (&App{}).buildEngineConfig(profile)
	if conf.TUNMode || conf.AddRoute {
		t.Fatal("silently enabled TUN for an existing profile")
	}
	if conf.AutoDetectInterface || conf.BindInterface != "WLAN" {
		t.Fatal("lost explicit interface binding")
	}
	if profile.DisableProxy || conf.SocksBind == "" || conf.HTTPBind == "" {
		t.Fatal("disabled proxies for an existing profile")
	}
}

func TestProxyAndTUNCanBeConfiguredIndependently(t *testing.T) {
	for _, tun := range []bool{false, true} {
		for _, disabled := range []bool{false, true} {
			conf := (&App{}).buildEngineConfig(Profile{TUNMode: tun, DisableProxy: disabled})
			if conf.TUNMode || conf.AddRoute {
				t.Fatal("proxy setting changed TUN mode")
			}
			if disabled && (conf.SocksBind != "" || conf.HTTPBind != "") ||
				!disabled && (conf.SocksBind != "127.0.0.1:1080" || conf.HTTPBind != "127.0.0.1:1081") {
				t.Fatalf("incorrect proxy listeners: disabled=%v socks=%q http=%q", disabled, conf.SocksBind, conf.HTTPBind)
			}
		}
	}
}

func TestProxySwitchPersistsOnlySelectedProfile(t *testing.T) {
	app := &App{configPath: filepath.Join(t.TempDir(), "profiles.json"), appConfig: AppConfig{
		ActiveProfileID: "second", Profiles: []Profile{{ID: "first"}, {ID: "second", TUNMode: true}},
	}}
	for _, enabled := range []bool{false, true} {
		if err := app.SetProxyEnabled(enabled); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(app.configPath)
		if err != nil {
			t.Fatal(err)
		}
		var saved AppConfig
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Profiles[0].DisableProxy || saved.Profiles[1].DisableProxy == enabled || !saved.Profiles[1].TUNMode {
			t.Fatalf("incorrect saved settings: %+v", saved.Profiles)
		}
	}
}

func TestProxySwitchRejectsActiveConnectionAndRestoresFailedSave(t *testing.T) {
	app := &App{configPath: filepath.Join(t.TempDir(), "missing", "profiles.json"), appConfig: AppConfig{
		ActiveProfileID: "selected", Profiles: []Profile{{ID: "selected"}},
	}}
	for _, state := range []string{"starting", "running", "save failure"} {
		app.starting = state == "starting"
		app.status.Running = state == "running"
		if err := app.SetProxyEnabled(false); err == nil {
			t.Fatalf("accepted switch during %s", state)
		}
		if app.appConfig.Profiles[0].DisableProxy {
			t.Fatalf("changed setting after %s", state)
		}
	}
}

func TestConnectingStatusShowsActualStageAfterAuthentication(t *testing.T) {
	app := &App{status: VPNStatus{TUNMode: true, LastError: "old failure"}}
	for _, progress := range []struct{ stage, message string }{
		{"认证中", "正在验证账号…"},
		{"建立隧道", "认证已通过，正在创建 VPN 网络栈…"},
		{"配置网卡", "正在配置 TUN 网卡…"},
		{"识别 DNS", "正在识别 DNS 区域…"},
		{"配置路由", "正在配置 62 条 VPN 资源路由…"},
		{"配置 DNS", "正在应用分域 DNS 规则…"},
		{"启动代理", "正在启动本地代理…"},
	} {
		app.handleEngineStatus(engine.Status{Phase: engine.PhaseConnecting, Stage: progress.stage, Message: progress.message})
		status := app.GetStatus()
		if !status.Running || status.State != "connecting" || status.Stage != progress.stage || status.Message != progress.message || status.LastError != "" {
			t.Fatalf("incorrect progress status: %+v", status)
		}
		data, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["stage"] != progress.stage || payload["message"] != progress.message {
			t.Fatalf("frontend did not receive progress: %s", data)
		}
	}
	app.handleEngineStatus(engine.Status{Phase: engine.PhaseConnected, Message: "VPN 已连接"})
	if status := app.GetStatus(); status.Stage != "" || status.State != "connected" {
		t.Fatalf("connection retained the previous setup stage: %+v", status)
	}
}

func TestReconnectingMessageIsProgressAndFailureRemainsAnError(t *testing.T) {
	app := &App{}
	app.handleEngineStatus(engine.Status{Phase: engine.PhaseReconnecting, Message: "正在重新登录…"})
	if status := app.GetStatus(); status.Stage != "重连中" || status.Message != "正在重新登录…" || status.LastError != "" {
		t.Fatalf("reconnection progress was treated as an error: %+v", status)
	}
	app.handleEngineStatus(engine.Status{Phase: engine.PhaseError, Message: "DNS 配置失败"})
	if status := app.GetStatus(); status.Running || status.State != "error" || status.LastError != "DNS 配置失败" || status.Stage != "" {
		t.Fatalf("failure did not clear connection progress: %+v", status)
	}
}

func TestStageClockPreservesReconnectCompatibility(t *testing.T) {
	app := &App{}
	app.handleEngineStatus(engine.Status{Phase: engine.PhaseReconnecting, Stage: "认证中"})
	if s := app.GetStatus(); s.Stage != "重连中" || s.StageDetail != "认证中" || s.StageStartedAt == 0 {
		t.Fatalf("missing reconnect stage: %+v", s)
	}
	app.mu.Lock()
	app.status.StageStartedAt = 123
	app.mu.Unlock()
	app.handleEngineStatus(engine.Status{Phase: engine.PhaseReconnecting, Stage: "认证中", Message: "still authenticating"})
	if s := app.GetStatus(); s.StageStartedAt != 123 {
		t.Fatal("same stage reset clock")
	}
	app.handleEngineStatus(engine.Status{Phase: engine.PhaseReconnecting, Stage: "重建隧道"})
	if s := app.GetStatus(); s.StageStartedAt <= 123 || s.StageDetail != "重建隧道" {
		t.Fatalf("stage change did not reset clock: %+v", s)
	}
	for _, phase := range []string{engine.PhaseConnected, engine.PhaseError, engine.PhaseDisconnected} {
		app.handleEngineStatus(engine.Status{Phase: phase})
		if s := app.GetStatus(); s.StageStartedAt != 0 || s.StageDetail != "" {
			t.Fatalf("terminal state retained timer: %+v", s)
		}
	}
}

func TestProxySwitchTargetsExplicitProfile(t *testing.T) {
	a := multiTestApp(t)
	a.appConfig.ActiveProfileID = "b"
	if err := a.SetProfileProxyEnabled("a", false); err != nil {
		t.Fatal(err)
	}
	if !a.appConfig.Profiles[0].DisableProxy || a.appConfig.Profiles[1].DisableProxy {
		t.Fatal("wrong profile modified")
	}
	a.connections = map[string]*vpnConnection{"a": {status: VPNStatus{Running: true}}}
	if err := a.SetProfileProxyEnabled("a", true); err == nil {
		t.Fatal("running target accepted")
	}
	if !a.appConfig.Profiles[0].DisableProxy {
		t.Fatal("running target modified")
	}
	if err := a.SetProfileProxyEnabled("missing", true); err == nil {
		t.Fatal("missing target accepted")
	}
}
