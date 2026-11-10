package engine

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sagernet/netlink"
)

type linuxEchoBackend struct{ tcp, udp string }

func (b linuxEchoBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	target := b.tcp
	if network == "udp" {
		target = b.udp
	}
	return (&net.Dialer{}).DialContext(ctx, network, target)
}
func (linuxEchoBackend) DNS(context.Context, []byte) ([]byte, []string, error) { return nil, nil, nil }

func TestLinuxTUNIngressTCPUDP(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(lo); err != nil {
		t.Fatal(err)
	}
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	go func() {
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := udp.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udp.WriteTo(buf[:n], addr)
		}
	}()
	spec := linuxTestSpec()
	spec.Prefixes = []string{"198.18.61.0/24"}
	ingress, err := startLinuxTUNIngress(context.Background(), spec, linuxEchoBackend{tcp: tcp.Addr().String(), udp: udp.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	defer ingress.Close()
	for _, network := range []string{"tcp4", "udp4"} {
		t.Run(network, func(t *testing.T) {
			conn, err := net.DialTimeout(network, "198.18.61.2:12345", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			payload := []byte("linux-ingress-" + network)
			if _, err = conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len(payload))
			if _, err = io.ReadFull(conn, got); err != nil {
				t.Fatal(err)
			}
			if string(got) != string(payload) {
				t.Fatalf("unexpected payload: %q", got)
			}
		})
	}
	active, err := net.DialTimeout("tcp4", "198.18.61.2:12345", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	if _, err = active.Write([]byte("still-active")); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadFull(active, make([]byte, len("still-active"))); err != nil {
		t.Fatal(err)
	}
	name := ingress.name
	if err := ingress.Close(); err != nil {
		t.Fatal(err)
	}
	_ = active.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err = active.Read(make([]byte, 1)); err == nil {
		t.Fatal("active connection survived ingress close")
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatal("interface still exists after ingress close")
	}
}
