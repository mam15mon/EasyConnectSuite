package engine

import (
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	tun "github.com/mythologyli/sing-tun"
)

// This test runs only on disposable macOS CI machines with explicit root access.
func TestDarwinUTUNSystemLifecycle(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	if os.Geteuid() != 0 {
		t.Fatal("system test requires root")
	}
	name := tun.CalculateInterfaceName("")
	if name == "" {
		t.Fatal("could not select utun interface")
	}
	device, err := tun.New(tun.Options{
		Name: name, MTU: 1400,
		Inet4Address: []netip.Prefix{netip.MustParsePrefix("198.19.128.249/30")},
		AutoRoute:    false,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := device.Close(); err != nil {
				t.Errorf("close %s: %v", name, err)
			}
		}
	})
	iface, err := net.InterfaceByName(name)
	if err != nil {
		t.Fatalf("find %s: %v", name, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		t.Fatalf("read %s addresses: %v", name, err)
	}
	found := false
	for _, address := range addresses {
		if prefix, err := netip.ParsePrefix(address.String()); err == nil && prefix.Addr() == netip.MustParseAddr("198.19.128.249") {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s has no test address: %v", name, addresses)
	}
	if err := device.Close(); err != nil {
		t.Fatalf("close %s: %v", name, err)
	}
	closed = true
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := net.InterfaceByName(name); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s remains after close", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
