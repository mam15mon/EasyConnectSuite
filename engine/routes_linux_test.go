package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

func TestLinuxRoutesRejectsInvalidInputs(t *testing.T) {
	r := &linuxRoutes{ref: linuxTestSpec()}
	if _, err := r.Apply(nil, []string{"192.0.2.0/24"}); err == nil {
		t.Fatal("explicit deletion accepted")
	}
	if _, err := r.Apply([]string{"0.0.0.0/0"}, nil); err == nil {
		t.Fatal("invalid device or default route accepted")
	}
}

func TestLinuxRoutesOwnership(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	first, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, secondName, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, name := range []string{name, secondName} {
		link, err := netlink.LinkByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := netlink.LinkSetUp(link); err != nil {
			t.Fatal(err)
		}
	}
	firstLink, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	secondLink, err := netlink.LinkByName(secondName)
	if err != nil {
		t.Fatal(err)
	}
	a := &linuxRoutes{device: first, name: name, index: firstLink.Attrs().Index, ref: linuxTestSpec()}
	b := &linuxRoutes{device: second, name: secondName, index: secondLink.Attrs().Index, ref: linuxTestSpec()}
	wrong := &linuxRoutes{device: first, name: name, index: secondLink.Attrs().Index, ref: linuxTestSpec()}
	if _, err := wrong.Apply([]string{"203.0.113.0/24"}, nil); err == nil {
		t.Fatal("wrong interface index accepted")
	}
	if result, err := a.Apply([]string{"192.0.2.0/24"}, nil); err != nil || len(result.Added) != 1 {
		t.Fatalf("add: %+v %v", result, err)
	}
	raw, err := first.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags int
	var flagErr error
	if err := raw.Control(func(fd uintptr) { flags, flagErr = unix.FcntlInt(fd, unix.F_GETFL, 0) }); err != nil {
		t.Fatal(err)
	}
	if flagErr != nil {
		t.Fatal(flagErr)
	}
	if flags&unix.O_NONBLOCK == 0 {
		t.Fatal("route verification disabled non-blocking TUN I/O")
	}
	if _, err := b.Apply([]string{"192.0.2.0/24"}, nil); err == nil {
		t.Fatal("conflicting route accepted")
	}
	_, existingPrefix, err := net.ParseCIDR("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if err := netlink.RouteAdd(&netlink.Route{LinkIndex: firstLink.Attrs().Index, Dst: existingPrefix, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN, Priority: 77}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Apply([]string{"203.0.113.0/24"}, nil); err == nil {
		t.Fatal("existing route with another metric was accepted")
	}
	if _, err := b.Apply([]string{"198.51.100.0/24"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_TABLE)
	if err != nil {
		t.Fatal(err)
	}
	foundOther := false
	for _, route := range routes {
		if route.Dst != nil && route.Dst.String() == "192.0.2.0/24" {
			t.Fatal("closed interface route remains")
		}
		if route.Dst != nil && route.Dst.String() == "198.51.100.0/24" {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatal("other interface route was removed")
	}
}

func TestLinuxRoutesConcurrentConflict(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	first, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	other, otherName, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	otherLink, err := netlink.LinkByName(otherName)
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(link); err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(otherLink); err != nil {
		t.Fatal(err)
	}
	_, prefix, err := net.ParseCIDR("198.18.17.0/24")
	if err != nil {
		t.Fatal(err)
	}
	var injectionErr error
	r := &linuxRoutes{device: first, name: name, index: link.Attrs().Index, ref: linuxTestSpec(), beforeAdd: func() {
		injectionErr = netlink.RouteAdd(&netlink.Route{LinkIndex: otherLink.Attrs().Index, Dst: prefix, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN, Priority: 77})
	}}
	result, err := r.Apply([]string{prefix.String()}, nil)
	if injectionErr != nil {
		t.Fatal(injectionErr)
	}
	if err == nil || len(result.Added) != 0 {
		t.Fatalf("race not rejected: %+v %v", result, err)
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatal("conflicted interface remains")
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: prefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].LinkIndex != otherLink.Attrs().Index {
		t.Fatalf("other route not preserved: %+v", routes)
	}
}

func TestLinuxRoutesPartialFailureClosesOwner(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	first, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	other, otherName, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	otherLink, err := netlink.LinkByName(otherName)
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(link); err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(otherLink); err != nil {
		t.Fatal(err)
	}
	_, conflict, _ := net.ParseCIDR("198.18.19.0/24")
	if err = netlink.RouteAdd(&netlink.Route{LinkIndex: otherLink.Attrs().Index, Dst: conflict, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN}); err != nil {
		t.Fatal(err)
	}
	r := &linuxRoutes{device: first, name: name, index: link.Attrs().Index, ref: linuxTestSpec()}
	result, err := r.Apply([]string{"198.18.18.0/24", conflict.String()}, nil)
	if err == nil || len(result.Added) != 0 {
		t.Fatalf("partial failure: %+v %v", result, err)
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatal("partial interface remains")
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: conflict, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 1 || routes[0].LinkIndex != otherLink.Attrs().Index {
		t.Fatalf("other route changed: %+v %v", routes, err)
	}
}

func TestLinuxTUNCrashHelper(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_CRASH_HELPER") != "1" {
		return
	}
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(link); err != nil {
		t.Fatal(err)
	}
	r := &linuxRoutes{device: file, name: name, index: link.Attrs().Index, ref: linuxTestSpec()}
	if _, err = r.Apply([]string{"198.18.31.0/24"}, nil); err != nil {
		t.Fatal(err)
	}
	fmt.Println("READY:", name)
	for {
		time.Sleep(time.Hour)
	}
}

func TestLinuxTUNCrashCleanup(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	other, otherName, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	otherLink, err := netlink.LinkByName(otherName)
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(otherLink); err != nil {
		t.Fatal(err)
	}
	_, otherPrefix, _ := net.ParseCIDR("198.18.32.0/24")
	if err = netlink.RouteAdd(&netlink.Route{LinkIndex: otherLink.Attrs().Index, Dst: otherPrefix, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxTUNCrashHelper$")
	cmd.Env = append(os.Environ(), "ECS_LINUX_TUN_CRASH_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var name string
			if _, err := fmt.Sscanf(scanner.Text(), "READY: %s", &name); err == nil {
				ready <- name
				return
			}
		}
		ready <- ""
	}()
	var childName string
	select {
	case childName = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("child did not create TUN")
	}
	if childName == "" {
		t.Fatal("child exited before creating TUN")
	}
	if _, err = net.InterfaceByName(childName); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if _, err = net.InterfaceByName(childName); err == nil {
		t.Fatal("crashed process TUN remained")
	}
	if _, err = net.InterfaceByName(otherName); err != nil {
		t.Fatalf("other TUN was removed: %v", err)
	}
	_, childPrefix, _ := net.ParseCIDR("198.18.31.0/24")
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: childPrefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 0 {
		t.Fatalf("crashed process routes remain: %+v %v", routes, err)
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: otherPrefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 1 || routes[0].LinkIndex != otherLink.Attrs().Index {
		t.Fatalf("other route changed: %+v %v", routes, err)
	}
}

func TestLinuxRoutesPartialFailurePreservesCloseError(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(link); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var injected error
	r := &linuxRoutes{device: file, name: name, index: link.Attrs().Index, ref: linuxTestSpec(), beforeAdd: func() {
		calls++
		if calls == 2 {
			injected = file.Close()
		}
	}}
	result, err := r.Apply([]string{"198.18.20.0/24", "198.18.21.0/24"}, nil)
	if injected != nil {
		t.Fatal(injected)
	}
	var cleanup *TUNCleanupError
	if calls != 2 || err == nil || !errors.As(err, &cleanup) || !errors.Is(err, os.ErrClosed) || len(result.Added) != 1 {
		t.Fatalf("partial cleanup failure lost: calls=%d result=%+v err=%v", calls, result, err)
	}
}
