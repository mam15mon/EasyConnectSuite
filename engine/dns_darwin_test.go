package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if err != nil || !strings.HasPrefix(string(data), "# EasyConnectSuite TUN pid=") || !strings.HasSuffix(string(data), "\nnameserver 198.19.128.2\n") {
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

func TestDarwinStaleResolverCleanupPreservesLiveAndUnownedFiles(t *testing.T) {
	dir := t.TempDir()
	marker := func(pid int) []byte {
		return []byte(fmt.Sprintf("# EasyConnectSuite TUN pid=%d token=%s\nnameserver 198.19.128.2\n", pid, strings.Repeat("a", 48)))
	}
	for name, data := range map[string][]byte{"stale.example": marker(101), "live.example": marker(202), "foreign.example": []byte("nameserver 192.0.2.1\n")} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	alive := func(pid int) (bool, error) { return pid == 202, nil }
	if pending, err := cleanupDarwinResolvers(dir, true, false, alive); err != nil || !pending {
		t.Fatalf("probe: %t %v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.example")); err != nil {
		t.Fatal("probe changed files", err)
	}
	if pending, err := cleanupDarwinResolvers(dir, false, false, alive); err != nil || !pending {
		t.Fatalf("cleanup: %t %v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.example")); !os.IsNotExist(err) {
		t.Fatal("stale resolver remains", err)
	}
	for _, name := range []string{"live.example", "foreign.example"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s lost: %v", name, err)
		}
	}
}

func TestDarwinStaleResolverRejectsMalformedMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corp.example")
	if err := os.WriteFile(path, []byte("# EasyConnectSuite TUN pid=123 token=invalid\nnameserver 198.19.128.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanupDarwinResolvers(dir, false, false, func(int) (bool, error) { return false, nil }); err == nil {
		t.Fatal("malformed ownership marker accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("malformed file removed", err)
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
