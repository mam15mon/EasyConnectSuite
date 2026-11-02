package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinDNSManagesOnlyOwnedResolvers(t *testing.T) {
	dir := t.TempDir()
	r := &darwinRoutes{dns: "198.19.128.2", resolverDir: dir}
	other := filepath.Join(dir, "other.example")
	if err := os.WriteFile(other, []byte("nameserver 192.0.2.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.setDNS([]string{".corp.example", "corp.example"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "corp.example")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "nameserver 198.19.128.2\n" {
		t.Fatalf("resolver = %q %v", data, err)
	}
	if err := r.setDNS([]string{"other.example"}); err == nil {
		t.Fatal("existing resolver was overwritten")
	}
	data, err = os.ReadFile(other)
	if err != nil || string(data) != "nameserver 192.0.2.1\n" {
		t.Fatalf("other resolver changed: %q %v", data, err)
	}
	if err := r.setDNS(nil); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinDNSRefusesChangedResolverCleanup(t *testing.T) {
	dir := t.TempDir()
	r := &darwinRoutes{dns: "198.19.128.2", resolverDir: dir}
	if err := r.setDNS([]string{"corp.example"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "corp.example")
	if err := os.WriteFile(path, []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.setDNS(nil); err == nil {
		t.Fatal("changed resolver was removed")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "changed\n" {
		t.Fatal("changed resolver lost", err)
	}
}

func TestDarwinDNSRejectsUnsafeNamespace(t *testing.T) {
	r := &darwinRoutes{dns: "198.19.128.2", resolverDir: t.TempDir()}
	for _, value := range []string{"../escape", ".", "..", "192.0.2.1"} {
		if err := r.setDNS([]string{value}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
