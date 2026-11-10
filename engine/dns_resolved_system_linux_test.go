//go:build linux

package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

// This test requires a real resolved instance on a private bus and in a private
// network and mount namespace. It never connects to the host system bus.
func TestResolvedRealIsolatedLinkLifecycle(t *testing.T) {
	if os.Getenv("ECS_REAL_RESOLVED_TEST") != "1" {
		t.Skip("requires isolated systemd-resolved")
	}
	for _, item := range []struct{ path, env string }{{"/proc/self/ns/net", "ECS_HOST_NET_NS"}, {"/proc/self/ns/mnt", "ECS_HOST_MNT_NS"}} {
		actual, err := os.Readlink(item.path)
		host := os.Getenv(item.env)
		if err != nil || host == "" || actual == host {
			t.Fatalf("namespace %s not isolated: %v", item.path, err)
		}
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" || os.Getenv("DBUS_SYSTEM_BUS_ADDRESS") != os.Getenv("DBUS_SESSION_BUS_ADDRESS") {
		t.Fatal("system bus is not private")
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	iface, err := net.InterfaceByName(name)
	if err != nil {
		t.Fatal(err)
	}
	manager := conn.Object("org.freedesktop.resolve1", dbus.ObjectPath("/org/freedesktop/resolve1"))
	var path dbus.ObjectPath
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = manager.Call("org.freedesktop.resolve1.Manager.GetLink", 0, int32(iface.Index)).Store(&path)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resolved did not discover TUN: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	verify := func() error {
		current, err := net.InterfaceByName(name)
		if err != nil {
			return err
		}
		if current.Index != iface.Index {
			return fmt.Errorf("interface index changed")
		}
		return nil
	}
	owner := &resolvedDNSOwner{index: iface.Index, server: "198.19.128.2", ref: linuxTestSpec(), call: resolvedSystemCall, verify: verify}
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("DNS cleanup: %v", err)
		}
	}()
	link := conn.Object("org.freedesktop.resolve1", path)
	read := func() ([]resolvedAddress, []resolvedDomain) {
		t.Helper()
		dnsValue, err := link.GetProperty("org.freedesktop.resolve1.Link.DNS")
		if err != nil {
			t.Fatal(err)
		}
		domainsValue, err := link.GetProperty("org.freedesktop.resolve1.Link.Domains")
		if err != nil {
			t.Fatal(err)
		}
		var addresses []resolvedAddress
		var domains []resolvedDomain
		if err = dbus.Store([]interface{}{dnsValue.Value()}, &addresses); err != nil {
			t.Fatal(err)
		}
		if err = dbus.Store([]interface{}{domainsValue.Value()}, &domains); err != nil {
			t.Fatal(err)
		}
		return addresses, domains
	}
	addresses, domains := read()
	if len(addresses) != 1 || addresses[0].Family != unix.AF_INET || !reflect.DeepEqual(addresses[0].Address, []byte{198, 19, 128, 2}) || len(domains) != 1 || domains[0].Name != "example.invalid" || !domains[0].RouteOnly {
		t.Fatalf("unexpected resolved configuration: %v %v", addresses, domains)
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	addresses, domains = read()
	if len(addresses) != 0 || len(domains) != 0 {
		t.Fatalf("resolved link not cleared: %v %v", addresses, domains)
	}

	// Model a second privileged client changing the same link after our apply.
	// RevertLink clears the whole link, so a successful revert is not proof of
	// safe coexistence with changes from other clients.
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); err != nil {
		t.Fatal(err)
	}
	external := []resolvedDomain{{Name: "external.invalid", RouteOnly: true}}
	if err = manager.Call("org.freedesktop.resolve1.Manager.SetLinkDomains", 0, int32(iface.Index), external).Err; err != nil {
		t.Fatal(err)
	}
	_, domains = read()
	if !reflect.DeepEqual(domains, external) {
		t.Fatalf("external change not observed: %v", domains)
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, domains = read()
	if len(domains) != 0 {
		t.Fatalf("unexpected link state after RevertLink: %v", domains)
	}
	t.Log("RevertLink removed a separately configured domain; coexistence remains unproven")

	// Observe the real service after the kernel removes the non-persistent TUN.
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		var removedPath dbus.ObjectPath
		err = manager.Call("org.freedesktop.resolve1.Manager.GetLink", 0, int32(iface.Index)).Store(&removedPath)
		if busErr, ok := err.(dbus.Error); ok && busErr.Name == "org.freedesktop.resolve1.NoSuchLink" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resolved still reports removed link or returned an unexpected error: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Prevent the deferred owner cleanup from issuing a request for the removed
	// index; this test deliberately observes link removal without RevertLink.
	owner.touched = false
	t.Log("resolved removed the link after its TUN descriptor closed")

	// Explicitly request the old kernel ifindex for an unrelated interface.
	replacement := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "ecs-reuse", Index: iface.Index}}
	if err = netlink.LinkAdd(replacement); err != nil {
		t.Fatalf("create replacement interface at old index: %v", err)
	}
	defer netlink.LinkDel(replacement)
	newIface, err := net.InterfaceByName("ecs-reuse")
	if err != nil || newIface.Index != iface.Index {
		t.Fatalf("old index was not reused: %+v %v", newIface, err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		err = manager.Call("org.freedesktop.resolve1.Manager.GetLink", 0, int32(iface.Index)).Store(&path)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resolved did not discover replacement interface: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	link = conn.Object("org.freedesktop.resolve1", path)
	addresses, domains = read()
	if len(addresses) != 0 || len(domains) != 0 {
		t.Fatalf("replacement inherited old link DNS: %v %v", addresses, domains)
	}
	t.Log("resolved replacement link with reused index had no old DNS settings")

	// A request holding only the old index cannot distinguish the replacement.
	// This deliberately sends after reuse; it does not model an in-flight polkit
	// request or prove that resolved schedules one late in a real deployment.
	stale := []resolvedDomain{{Name: "stale.invalid", RouteOnly: true}}
	if err = manager.Call("org.freedesktop.resolve1.Manager.SetLinkDomains", 0, int32(iface.Index), stale).Err; err != nil {
		t.Fatal(err)
	}
	_, domains = read()
	if !reflect.DeepEqual(domains, stale) {
		t.Fatalf("request to reused index did not modify replacement: %v", domains)
	}
	t.Log("request using the former index modified the replacement interface")
}
