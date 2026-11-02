package engine

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"EasyConnectSuite/internal/tunipc"
)

type darwinEchoBackend struct{ calls chan string }

func (b *darwinEchoBackend) Dial(_ context.Context, network, address string) (net.Conn, error) {
	b.calls <- fmt.Sprintf("%s %s", network, address)
	client, server := net.Pipe()
	go func() { defer server.Close(); _, _ = io.Copy(server, server) }()
	return client, nil
}
func (*darwinEchoBackend) DNS(context.Context, []byte) ([]byte, []string, error) {
	return nil, nil, fmt.Errorf("unexpected DNS request")
}

func TestDarwinUTUNForwardsTCPAndUDP(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	spec := TUNSpec{Ref: tunipc.Ref{Profile: "flow-test", Session: "test", Attachment: "test", Policy: 1}, Slot: 62, Prefixes: []string{"198.18.250.42/32"}}
	backend := &darwinEchoBackend{calls: make(chan string, 2)}
	ingress, err := StartDarwinTUNIngress(context.Background(), spec, backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ingress.Close(); err != nil {
			t.Errorf("cleanup ingress: %v", err)
		}
	})
	for _, network := range []string{"tcp", "udp"} {
		conn, err := net.DialTimeout(network+"4", "198.18.250.42:43210", 5*time.Second)
		if err != nil {
			t.Fatalf("%s dial: %v", network, err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err = conn.Write([]byte("hello-tun")); err != nil {
			conn.Close()
			t.Fatalf("%s write: %v", network, err)
		}
		var reply [9]byte
		if _, err = io.ReadFull(conn, reply[:]); err != nil || string(reply[:]) != "hello-tun" {
			conn.Close()
			t.Fatalf("%s reply %q: %v", network, reply, err)
		}
		conn.Close()
		select {
		case got := <-backend.calls:
			if got != network+" 198.18.250.42:43210" {
				t.Fatalf("backend saw %q", got)
			}
		case <-time.After(time.Second):
			t.Fatal("backend not invoked")
		}
	}
}

// This test runs only on disposable macOS CI machines with explicit root access.
func TestDarwinUTUNSystemLifecycle(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	if os.Geteuid() != 0 {
		t.Fatal("system test requires root")
	}
	spec := TUNSpec{Ref: tunipc.Ref{Profile: "system-test", Session: "test", Attachment: "test", Policy: 1}, Slot: 62}
	device, name, err := createDarwinTUN(context.Background(), spec)
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
	routes := newRouteManager(&darwinRoutes{name: name, ref: spec})
	if err := routes.replace([]string{testRoute}); err != nil {
		t.Fatalf("add route: %v", err)
	}
	routeRemoved := false
	t.Cleanup(func() {
		if !routeRemoved {
			if err := routes.close(); err != nil {
				t.Errorf("delete route: %v", err)
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
	if err := routes.close(); err != nil {
		t.Fatalf("delete route: %v", err)
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
