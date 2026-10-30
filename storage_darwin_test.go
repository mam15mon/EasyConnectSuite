package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMacStorageAndDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	want := filepath.Join(dir, "Library", "Application Support", "VPN")
	if applicationDataDir() != want {
		t.Fatalf("storage path = %q", applicationDataDir())
	}
	if err := os.MkdirAll(filepath.Dir(want), 0700); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	if a.configPath != filepath.Join(want, "vpn_profiles.json") {
		t.Fatalf("config path = %q", a.configPath)
	}
	p := a.appConfig.Profiles[0]
	if p.TUNMode || p.DisableProxy {
		t.Fatalf("default profile cannot use proxy: %+v", p)
	}
}
