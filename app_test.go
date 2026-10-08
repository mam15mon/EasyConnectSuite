package main

import (
	"encoding/json"
	"testing"

	"EasyConnectSuite/engine"
)

func TestTUNProfileUsesServerResourcesAndRetainsLocalProxies(t *testing.T) {
	app := &App{}
	conf := app.buildEngineConfig(Profile{Server: "vpn.example", Username: "user", TUNMode: true, DisableZjuConfig: false})
	if !conf.TUNMode || !conf.AddRoute || !conf.DisableZJUConfig {
		t.Fatal("TUN must use only this server's resources")
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
