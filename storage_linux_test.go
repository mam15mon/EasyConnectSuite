package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"EasyConnectSuite/engine"
)

func TestLinuxStorageDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := applicationDataDir(), filepath.Join(home, ".config", "VPN"); got != want {
		t.Fatalf("default configuration path = %q, want %q", got, want)
	}
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	if got, want := applicationDataDir(), filepath.Join(config, "VPN"); got != want {
		t.Fatalf("XDG configuration path = %q, want %q", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if got, want := applicationDataDir(), filepath.Join(home, ".config", "VPN"); got != want {
		t.Fatalf("relative XDG configuration path = %q, want %q", got, want)
	}
}

func TestLinuxProfileConfigRestrictsNewAndExistingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	for _, existing := range []bool{false, true} {
		if existing {
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := writeProfileConfig(path, []byte("synthetic")); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions = %o", info.Mode().Perm())
		}
	}
}

func TestLinuxImportedTUNRequiresExplicitDisableBeforeConnecting(t *testing.T) {
	setTestStorage(t)
	a := multiTestApp(t)
	a.appConfig.Profiles[0].TUNMode = true
	id := a.appConfig.Profiles[0].ID
	started := false
	start := func(o engine.Options) (vpnEngine, error) {
		started = true
		if o.TUNEnabled {
			t.Fatal("TUN unexpectedly enabled")
		}
		e := &fakeVPNEngine{opts: o}
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return e, nil
	}
	result := a.startConnectionID(id, start)
	if !strings.Contains(result, "关闭 TUN") || started || !a.appConfig.Profiles[0].TUNMode {
		t.Fatalf("unsupported imported TUN: result=%q started=%t enabled=%t", result, started, a.appConfig.Profiles[0].TUNMode)
	}
	if err := a.SetProfileTUNMode(id, false); err != nil {
		t.Fatal(err)
	}
	if result = a.startConnectionID(id, start); result != "OK" || !started {
		t.Fatalf("proxy connection: %q started=%t", result, started)
	}
	a.stopConnection(id)
}
