package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageMigrationPreservesFilesAndRemovesLegacyPath(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	legacy := filepath.Join(os.Getenv("APPDATA"), "EasyConnectVPN")
	files := map[string]string{
		"vpn_profiles.json":     `{"profiles":[{"id":"synthetic"}]}`,
		"logs/engine.log":       "synthetic current log\n",
		"logs/engine.log.old":   "synthetic previous log\n",
		"logs/crash.log":        "synthetic GUI startup\n",
		"logs/worker/crash.log": "synthetic nested worker evidence\n",
	}
	for name, data := range files {
		path := filepath.Join(legacy, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyStorage(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy storage still exists: %v", err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(applicationDataDir(), filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Fatalf("migration lost or changed %s: %v", name, err)
		}
	}
	if err := migrateLegacyStorage(); err != nil {
		t.Fatalf("completed migration was not idempotent: %v", err)
	}
}

func TestStorageMigrationRejectsConflictingDirectoriesWithoutChangingEither(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	legacy := filepath.Join(os.Getenv("APPDATA"), "EasyConnectVPN")
	for _, dir := range []string{legacy, applicationDataDir()} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "vpn_profiles.json"), []byte(filepath.Base(dir)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyStorage(); err == nil {
		t.Fatal("conflicting stores were silently merged or overwritten")
	}
	for _, dir := range []string{legacy, applicationDataDir()} {
		got, err := os.ReadFile(filepath.Join(dir, "vpn_profiles.json"))
		if err != nil || string(got) != filepath.Base(dir) {
			t.Fatal("conflict changed existing configuration", err)
		}
	}
}

func TestStorageMigrationDoesNotCreateLegacyStorageForFreshInstall(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if err := migrateLegacyStorage(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(os.Getenv("APPDATA"))
	if err != nil || len(entries) != 0 {
		t.Fatal("fresh startup created a legacy or empty migration directory", err)
	}
	if applicationLogDir() != filepath.Join(os.Getenv("APPDATA"), "VPN", "logs") {
		t.Fatal("logs do not use the VPN storage directory")
	}
}
