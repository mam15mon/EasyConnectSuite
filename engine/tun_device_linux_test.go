package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

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
	packet := make([]byte, 1500)
	var n int
	if err = raw.Control(func(fd uintptr) {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		for attempt := 0; attempt < 10; attempt++ {
			var count int
			count, pollErr = unix.Poll(fds, 500)
			if pollErr != nil {
				return
			}
			if count == 0 || fds[0].Revents&unix.POLLIN == 0 {
				continue
			}
			n, pollErr = unix.Read(int(fd), packet)
			if pollErr == unix.EAGAIN {
				pollErr = nil
				continue
			}
			if pollErr != nil {
				return
			}
			// A newly enabled interface can emit unrelated IPv6 control packets.
			if n >= 28 && packet[0]>>4 == 4 && packet[9] == 17 && bytes.Contains(packet[:n], []byte("linux-tun-packet")) {
				return
			}
		}
		pollErr = fmt.Errorf("TUN packet did not arrive")
	}); err != nil {
		t.Fatal(err)
	}
	if pollErr != nil {
		t.Fatal(pollErr)
	}
	if n < 28 || packet[0]>>4 != 4 || packet[9] != 17 || !bytes.Contains(packet[:n], []byte("linux-tun-packet")) {
		t.Fatalf("unexpected packet length %d", n)
	}
	readResult := make(chan error, 1)
	go func() {
		buf := make([]byte, 1500)
		for {
			n, e := file.Read(buf)
			if e != nil {
				readResult <- e
				return
			}
			if n >= 28 && buf[0]>>4 == 4 && buf[9] == 17 && bytes.Contains(buf[:n], []byte("linux-tun-poll-read")) {
				readResult <- nil
				return
			}
		}
	}()
	if _, err = conn.Write([]byte("linux-tun-poll-read")); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-readResult:
		if err != nil {
			t.Fatalf("pollable read: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pollable read did not receive packet")
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

func TestLinuxTUNBlockedReadCloses(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	file, _, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := file.Read(make([]byte, 1500)); result <- err }()
	select {
	case err := <-result:
		file.Close()
		t.Fatalf("read returned before close: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, os.ErrClosed) {
			t.Fatalf("read after close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TUN read remained blocked after close")
	}
}

func TestLinuxConfigurePreservesDeviceCloseFailure(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "closed-tun-")
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = configureLinuxTUN(ctx, file, "test", linuxTestSpec())
	var cleanup *TUNCleanupError
	if !errors.Is(err, context.Canceled) || !errors.Is(err, os.ErrClosed) || !errors.As(err, &cleanup) {
		t.Fatalf("missing cancellation or cleanup failure: %v", err)
	}
}
