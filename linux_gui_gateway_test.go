//go:build linux

package main

import (
	"net"
	"os"
	"testing"
	"time"

	"EasyConnectSuite/engine"
)

// The test gateway is started by build/linux/headless-smoke-test.sh in CI.
func TestLinuxGUIBackendMockGateway(t *testing.T) {
	if os.Getenv("ECS_GUI_GATEWAY_TEST") != "1" {
		t.Skip("requires isolated mock gateway")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	app := NewApp()
	profile, err := app.SaveProfile(Profile{ID: "mock-gui", Name: "mock", Protocol: "easyconnect", Server: "127.0.0.1", Port: 18445, Username: "mock-user", Password: "gui-private-canary", SocksPort: 19180, HTTPPort: 19181, DisableZjuConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewApp()
	found := false
	for _, saved := range reloaded.GetAppConfig().Profiles {
		if saved.ID == profile.ID && saved.Server == profile.Server && saved.SocksPort == profile.SocksPort {
			found = true
		}
	}
	if !found {
		t.Fatal("GUI profile did not persist")
	}
	result := reloaded.startConnectionID(profile.ID, func(opts engine.Options) (vpnEngine, error) { return engine.Start(opts) })
	if result != "OK" {
		t.Fatalf("GUI connection: %s", result)
	}
	t.Cleanup(func() { reloaded.stopConnection(profile.ID) })
	for _, port := range []string{"127.0.0.1:19180", "127.0.0.1:19181"} {
		conn, err := net.DialTimeout("tcp", port, 3*time.Second)
		if err != nil {
			t.Fatalf("GUI proxy %s: %v", port, err)
		}
		conn.Close()
	}
	if result = reloaded.stopConnection(profile.ID); result != "OK" {
		t.Fatalf("GUI stop: %s", result)
	}
	for _, port := range []string{"127.0.0.1:19180", "127.0.0.1:19181"} {
		conn, err := net.DialTimeout("tcp", port, time.Second)
		if err == nil {
			conn.Close()
			t.Fatalf("GUI proxy remained open: %s", port)
		}
	}
}
