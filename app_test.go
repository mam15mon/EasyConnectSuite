package main

import (
	"encoding/json"
	"testing"
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
