package main

import (
	"os"
	"path/filepath"
	"testing"
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
