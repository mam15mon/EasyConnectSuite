package engine

import (
	"context"
	"os"
	"testing"

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
	if _, err := b.Apply([]string{"192.0.2.0/24"}, nil); err == nil {
		t.Fatal("conflicting route accepted")
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
