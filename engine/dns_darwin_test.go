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
	if err != nil || !strings.HasPrefix(string(data), "nameserver 198.19.128.2\n# EasyConnectSuite TUN pid=") {
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), darwinResolverTempPrefix) {
			t.Fatalf("temporary resolver remains after publication or conflict: %s", entry.Name())
		}
	}
}

func TestDarwinStaleResolverCleanupPreservesLiveAndUnownedFiles(t *testing.T) {
	dir := t.TempDir()
	marker := func(pid int) []byte {
		return []byte(fmt.Sprintf("nameserver 198.19.128.2\n# EasyConnectSuite TUN pid=%d token=%s\n", pid, strings.Repeat("a", 48)))
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
	if err := os.WriteFile(path, []byte("nameserver 198.19.128.2\n# EasyConnectSuite TUN pid=123 token=invalid\n"), 0644); err != nil {
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

func TestDarwinStaleTemporaryResolvers(t *testing.T) {
	dir := t.TempDir()
	name := func(pid int) string {
		return fmt.Sprintf("%s%d-%s-123", darwinResolverTempPrefix, pid, strings.Repeat("a", 48))
	}
	stale, live := name(101), name(202)
	foreign := darwinResolverTempPrefix + "101-not-our-format"
	for _, file := range []string{stale, live, foreign} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alive := func(pid int) (bool, error) { return pid == 202, nil }
	if pending, err := cleanupDarwinResolvers(dir, true, false, alive); err != nil || !pending {
		t.Fatalf("probe partial resolver: %t %v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(dir, stale)); err != nil {
		t.Fatalf("probe removed temporary file: %v", err)
	}
	if pending, err := cleanupDarwinResolvers(dir, false, false, alive); err != nil || !pending {
		t.Fatalf("cleanup partial resolver: %t %v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(dir, stale)); !os.IsNotExist(err) {
		t.Fatalf("partial resolver remains: %v", err)
	}
	for _, file := range []string{live, foreign} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("unrelated temporary file lost: %v", err)
		}
	}
}
