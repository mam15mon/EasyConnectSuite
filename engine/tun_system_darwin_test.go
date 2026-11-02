package engine

import (
	"net"
	"net/netip"
	"os"
	"os/exec"
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
	const destination = "198.18.250.42"
	const testRoute = "198.18.250.42/32"
	if output, err := exec.Command("/sbin/route", "-n", "add", "-net", testRoute, "-interface", name).CombinedOutput(); err != nil {
		t.Fatalf("add route: %v: %s", err, output)
	}
	routeRemoved := false
	t.Cleanup(func() {
		if !routeRemoved {
			if output, err := exec.Command("/sbin/route", "-n", "delete", "-net", testRoute, "-interface", name).CombinedOutput(); err != nil {
				t.Errorf("delete route: %v: %s", err, output)
			}
		}
	})
	packets := make(chan []byte, 1)
	readErrors := make(chan error, 1)
	go func() {
		packet := make([]byte, 2048)
		n, err := device.Read(packet)
		if err != nil {
			readErrors <- err
			return
		}
		packets <- packet[:n]
	}()
	conn, err := net.Dial("udp4", net.JoinHostPort(destination, "43210"))
	if err != nil {
		t.Fatalf("dial test destination: %v", err)
	}
	if _, err := conn.Write([]byte("tun-test")); err != nil {
		conn.Close()
		t.Fatalf("write test packet: %v", err)
	}
	conn.Close()
	select {
	case packet := <-packets:
		// utun frames start with a four-byte address family before the IPv4 header.
		if len(packet) < 24 || packet[4]>>4 != 4 || packet[13] != 17 || net.IP(packet[20:24]).String() != destination {
			t.Fatalf("unexpected utun packet: %x", packet)
		}
	case err := <-readErrors:
		t.Fatalf("read utun packet: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("test packet did not reach utun")
	}
	if output, err := exec.Command("/sbin/route", "-n", "delete", "-net", testRoute, "-interface", name).CombinedOutput(); err != nil {
		t.Fatalf("delete route: %v: %s", err, output)
	}
	routeRemoved = true
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
