package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
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
	ingress, err := StartLinuxTUNIngress(context.Background(), spec, linuxEchoBackend{tcp: tcp.Addr().String(), udp: udp.LocalAddr().String()})
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

func TestLinuxTUNIngressPartialRouteFailure(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	other, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(link); err != nil {
		t.Fatal(err)
	}
	_, conflict, _ := net.ParseCIDR("198.18.82.0/24")
	if err = netlink.RouteAdd(&netlink.Route{LinkIndex: link.Attrs().Index, Dst: conflict, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN}); err != nil {
		t.Fatal(err)
	}
	spec := linuxTestSpec()
	spec.Prefixes = []string{"198.18.81.0/24", "198.18.82.0/24"}
	ingress, err := StartLinuxTUNIngress(context.Background(), spec, linuxEchoBackend{})
	if ingress != nil {
		ingress.Close()
		t.Fatal("conflicting ingress created")
	}
	if err == nil {
		t.Fatal("route conflict accepted")
	}
	if errors.Is(err, os.ErrClosed) {
		t.Fatalf("successful cleanup reported as failure: %v", err)
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: conflict, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 1 || routes[0].LinkIndex != link.Attrs().Index {
		t.Fatalf("other route changed: %+v %v", routes, err)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" && iface.Name != name {
			t.Fatal("failed ingress interface remained")
		}
	}
}

func TestLinuxTUNIngressRejectsUnsupportedDNSBeforeCreatingInterface(t *testing.T) {
	spec := linuxTestSpec()
	spec.Names = []string{"example.invalid"}
	// This runs without TUN privileges: DNS rejection must precede device creation.
	ingress, err := StartLinuxTUNIngress(context.Background(), spec, linuxEchoBackend{})
	if ingress != nil {
		ingress.Close()
		t.Fatal("unsupported DNS attachment created")
	}
	if err == nil || !strings.Contains(err.Error(), "DNS 分流尚未实现") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLinuxTUNIngressDNSLifecycle(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	spec := linuxTestSpec()
	spec.Names = []string{"example.invalid"}
	var calls []string
	call := func(_ context.Context, method string, args ...any) error {
		calls = append(calls, method)
		if len(args) == 0 || args[0].(int32) <= 0 {
			t.Fatalf("invalid DNS interface: %v", args)
		}
		return nil
	}
	ingress, err := startLinuxTUNIngressWithDNS(context.Background(), spec, linuxEchoBackend{}, call)
	if err != nil {
		t.Fatal(err)
	}
	name := ingress.name
	if err = ingress.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"SetLinkDNS", "SetLinkDomains", "RevertLink"}) {
		t.Fatalf("DNS calls: %v", calls)
	}
	if _, err = net.InterfaceByName(name); err == nil {
		t.Fatal("TUN survived DNS cleanup")
	}
}

func TestLinuxTUNIngressDNSFailureCleanup(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	for _, rollbackFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollbackFails=%t", rollbackFails), func(t *testing.T) {
			other, otherName, err := createLinuxTUN(context.Background(), linuxTestSpec())
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			spec := linuxTestSpec()
			spec.Slot = 12
			spec.Names = []string{"example.invalid"}
			domainErr := errors.New("synthetic domain error")
			rollbackErr := errors.New("synthetic rollback error")
			var calls []string
			call := func(_ context.Context, method string, _ ...any) error {
				calls = append(calls, method)
				if method == "SetLinkDomains" {
					return domainErr
				}
				if method == "RevertLink" && rollbackFails {
					return rollbackErr
				}
				return nil
			}
			ingress, err := startLinuxTUNIngressWithDNS(context.Background(), spec, linuxEchoBackend{}, call)
			if ingress != nil {
				ingress.Close()
				t.Fatal("failed DNS attached")
			}
			if !errors.Is(err, domainErr) {
				t.Fatalf("missing configuration error: %v", err)
			}
			if !errors.Is(err, errResolvedOutcomeUnknown) {
				t.Fatalf("missing unconfirmed cleanup state: %v", err)
			}
			want := []string{"SetLinkDNS", "SetLinkDomains"}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls: %v", calls)
			}
			if _, err := net.InterfaceByName(otherName); err != nil {
				t.Fatalf("other interface removed: %v", err)
			}
			ifaces, err := net.Interfaces()
			if err != nil {
				t.Fatal(err)
			}
			for _, iface := range ifaces {
				if strings.HasPrefix(iface.Name, "ecs") && iface.Name != otherName {
					t.Fatalf("failed TUN remained: %s", iface.Name)
				}
			}
		})
	}
}
