package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"EasyConnectSuite/internal/tunipc"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

func linuxTestSpec() TUNSpec {
	return TUNSpec{Ref: tunipc.Ref{Profile: "test", Session: "test", Attachment: "test", Policy: 1}}
}

func TestLinuxTUNRejectsInvalidAndCanceledRequests(t *testing.T) {
	if _, _, err := createLinuxTUN(context.Background(), TUNSpec{}); err == nil {
		t.Fatal("invalid spec accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := createLinuxTUN(ctx, linuxTestSpec()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func TestLinuxTUNDeviceOwnership(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	routesBefore, err := os.ReadFile("/proc/net/route")
	if err != nil {
		t.Fatal(err)
	}
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := net.InterfaceByName(name); err != nil {
		file.Close()
		t.Fatal(err)
	}
	second, otherName, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		file.Close()
		t.Fatal(err)
	}
	defer second.Close()
	if name == otherName {
		file.Close()
		t.Fatal("interfaces share a name")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := net.InterfaceByName(otherName); err != nil {
		t.Fatalf("other interface was removed: %v", err)
	}
	routesAfter, err := os.ReadFile("/proc/net/route")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(routesBefore, routesAfter) {
		t.Fatal("TUN creation changed the routing table")
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatal("TUN interface remained after its owner closed")
	}
}

func TestLinuxTUNConfiguredPacket(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := configureLinuxTUN(context.Background(), file, name, linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	iface, err := net.InterfaceByName(name)
	if err != nil || iface.Index != index || iface.Flags&net.FlagUp == 0 || iface.MTU != 1400 {
		t.Fatalf("interface: %+v %v", iface, err)
	}
	_, network, _ := net.ParseCIDR("198.19.128.0/30")
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: network, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 0 {
		t.Fatalf("unexpected automatic route: %+v %v", routes, err)
	}
	r := &linuxRoutes{device: file, name: name, index: index, ref: linuxTestSpec()}
	if _, err = r.Apply([]string{"198.18.44.0/24"}, nil); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialUDP("udp4", &net.UDPAddr{IP: net.ParseIP("198.19.128.1")}, &net.UDPAddr{IP: net.ParseIP("198.18.44.2"), Port: 12345})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.Write([]byte("linux-tun-packet")); err != nil {
		t.Fatal(err)
	}
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var pollErr error
	if err = raw.Control(func(fd uintptr) {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		var count int
		count, pollErr = unix.Poll(fds, 5000)
		if pollErr == nil && (count != 1 || fds[0].Revents&unix.POLLIN == 0) {
			pollErr = fmt.Errorf("TUN packet did not arrive")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if pollErr != nil {
		t.Fatal(pollErr)
	}
	packet := make([]byte, 1500)
	n, err := file.Read(packet)
	if err != nil {
		t.Fatal(err)
	}
	if n < 28 || packet[0]>>4 != 4 || packet[9] != 17 || !bytes.Contains(packet[:n], []byte("linux-tun-packet")) {
		t.Fatalf("unexpected packet length %d", n)
	}
}

func TestLinuxTUNConfigureFailureCleansDevice(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = configureLinuxTUN(context.Background(), file, "wrong-name", linuxTestSpec()); err == nil {
		file.Close()
		t.Fatal("wrong name accepted")
	}
	if _, err = net.InterfaceByName(name); err == nil {
		t.Fatal("failed configuration left interface behind")
	}
}
