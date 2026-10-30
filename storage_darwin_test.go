package main

import (
	"bytes"
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

func TestMacCanDisableImportedTUN(t *testing.T) {
	setTestStorage(t)
	a := multiTestApp(t)
	a.appConfig.Profiles[0].TUNMode = true
	id := a.appConfig.Profiles[0].ID
	if err := a.SetProfileTUNMode(id, true); err == nil {
		t.Fatal("macOS TUN must report unsupported")
	}
	if err := a.SetProfileTUNMode(id, false); err != nil {
		t.Fatal(err)
	}
	if a.appConfig.Profiles[0].TUNMode {
		t.Fatal("imported TUN setting remains enabled")
	}
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"tunMode": false`)) {
		t.Fatalf("disabled setting not persisted: %s", data)
	}
}

func TestMacCanDeleteStoppedProfileAfterTUNManagerCreated(t *testing.T) {
	a := multiTestApp(t)
	a.tunManager = newTUNManager()
	if !a.DeleteProfile("b") {
		t.Fatal("inactive profile deletion failed")
	}
}
