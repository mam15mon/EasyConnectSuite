//go:build linux

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/miekg/dns"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

type resolvedAnswerBackend struct {
	linuxEchoBackend
	answer string
}

func (b resolvedAnswerBackend) DNS(_ context.Context, packet []byte) ([]byte, []string, error) {
	request := new(dns.Msg)
	if err := request.Unpack(packet); err != nil {
		return nil, nil, err
	}
	reply := new(dns.Msg)
	reply.SetReply(request)
	for _, question := range request.Question {
		if question.Qtype == dns.TypeA {
			reply.Answer = append(reply.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}, A: net.ParseIP(b.answer)})
		}
	}
	response, err := reply.Pack()
	return response, nil, err
}

func isolatedResolved(t *testing.T) *dbus.Conn {
	t.Helper()
	if os.Getenv("ECS_REAL_RESOLVED_TEST") != "1" {
		t.Skip("requires isolated systemd-resolved")
	}
	for _, item := range []struct{ path, env string }{{"/proc/self/ns/net", "ECS_HOST_NET_NS"}, {"/proc/self/ns/mnt", "ECS_HOST_MNT_NS"}} {
		actual, err := os.Readlink(item.path)
		if err != nil || os.Getenv(item.env) == "" || actual == os.Getenv(item.env) {
			t.Fatalf("namespace %s not isolated: %v", item.path, err)
		}
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" || os.Getenv("DBUS_SYSTEM_BUS_ADDRESS") != os.Getenv("DBUS_SESSION_BUS_ADDRESS") {
		t.Fatal("system bus is not private")
	}
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(lo); err != nil {
		t.Fatal(err)
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func waitResolvedCondition(t *testing.T, reason string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal(reason)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertResolvedLinkGone(t *testing.T, conn *dbus.Conn, index int) {
	t.Helper()
	manager := conn.Object("org.freedesktop.resolve1", dbus.ObjectPath("/org/freedesktop/resolve1"))
	waitResolvedCondition(t, "resolved retained deleted TUN link", func() bool {
		var path dbus.ObjectPath
		err := manager.Call("org.freedesktop.resolve1.Manager.GetLink", 0, int32(index)).Store(&path)
		var remote dbus.Error
		return errors.As(err, &remote) && remote.Name == "org.freedesktop.resolve1.NoSuchLink"
	})
	if _, err := net.InterfaceByIndex(index); err == nil {
		t.Fatal("kernel retained deleted TUN link")
	}
}

func assertResolvedLink(t *testing.T, conn *dbus.Conn, ingress *tunIngress, name string) {
	t.Helper()
	index := ingress.backend.(*linuxRoutes).index
	manager := conn.Object("org.freedesktop.resolve1", dbus.ObjectPath("/org/freedesktop/resolve1"))
	var path dbus.ObjectPath
	if err := manager.Call("org.freedesktop.resolve1.Manager.GetLink", 0, int32(index)).Store(&path); err != nil {
		t.Fatal(err)
	}
	link := conn.Object("org.freedesktop.resolve1", path)
	var addresses []resolvedAddress
	var domains []resolvedDomain
	for _, item := range []struct {
		name string
		dest any
	}{{"DNS", &addresses}, {"Domains", &domains}} {
		value, err := link.GetProperty("org.freedesktop.resolve1.Link." + item.name)
		if err != nil {
			t.Fatal(err)
		}
		if err = dbus.Store([]any{value.Value()}, item.dest); err != nil {
			t.Fatal(err)
		}
	}
	defaultRoute, err := link.GetProperty("org.freedesktop.resolve1.Link.DefaultRoute")
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[0].Family != unix.AF_INET || !reflect.DeepEqual(addresses[0].Address, []byte(net.ParseIP(ingress.dns).To4())) || !reflect.DeepEqual(domains, []resolvedDomain{{Name: name, RouteOnly: true}}) || defaultRoute.Value() != false {
		t.Fatalf("unexpected resolved link configuration: DNS=%v domains=%v default=%v", addresses, domains, defaultRoute)
	}
}

func resolveThroughService(t *testing.T, conn *dbus.Conn, hostname, address string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var result []struct {
		Index   int32
		Family  int32
		Address []byte
	}
	var canonical string
	var flags uint64
	err := conn.Object("org.freedesktop.resolve1", dbus.ObjectPath("/org/freedesktop/resolve1")).CallWithContext(ctx, "org.freedesktop.resolve1.Manager.ResolveHostname", 0, int32(0), hostname, int32(unix.AF_INET), uint64(1)).Store(&result, &canonical, &flags)
	if err != nil {
		t.Fatalf("resolved lookup: %v", err)
	}
	if len(result) != 1 || net.IP(result[0].Address).String() != address {
		t.Fatalf("resolved lookup result: %+v", result)
	}
}

func TestResolvedRealIsolatedLinkLifecycle(t *testing.T) {
	conn := isolatedResolved(t)
	spec := linuxTestSpec()
	spec.Slot = 1
	spec.Prefixes = []string{"198.18.61.0/24"}
	spec.Names = []string{"example.invalid"}
	first, err := StartLinuxTUNIngress(context.Background(), spec, resolvedAnswerBackend{answer: "198.18.61.2"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	assertResolvedLink(t, conn, first, "example.invalid")
	resolveThroughService(t, conn, "host.example.invalid", "198.18.61.2")
	for _, network := range []string{"udp", "tcp"} {
		request := new(dns.Msg)
		request.SetQuestion("direct.example.invalid.", dns.TypeA)
		reply, _, err := (&dns.Client{Net: network, Timeout: 5 * time.Second}).Exchange(request, net.JoinHostPort(first.dns, "53"))
		if err != nil || reply == nil || len(reply.Answer) != 1 {
			t.Fatalf("TUN %s DNS: %v %v", network, reply, err)
		}
		if answer, ok := reply.Answer[0].(*dns.A); !ok || answer.A.String() != "198.18.61.2" {
			t.Fatalf("TUN %s DNS reply: %v", network, reply.Answer)
		}
	}
	secondSpec := spec
	secondSpec.Slot = 2
	secondSpec.Ref.Profile = "second"
	secondSpec.Ref.Attachment = "second"
	secondSpec.Prefixes = []string{"198.18.62.0/24"}
	secondSpec.Names = []string{"other.invalid"}
	second, err := StartLinuxTUNIngress(context.Background(), secondSpec, resolvedAnswerBackend{answer: "198.18.62.2"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	assertResolvedLink(t, conn, second, "other.invalid")
	oldIndex := first.backend.(*linuxRoutes).index
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	assertResolvedLinkGone(t, conn, oldIndex)
	assertResolvedLink(t, conn, second, "other.invalid")
	resolveThroughService(t, conn, "after-close.other.invalid", "198.18.62.2")
	// Policy replacement deletes the old owned link before creating a new one.
	spec.Policy++
	spec.Prefixes = []string{"198.18.63.0/24"}
	spec.Names = []string{"updated.invalid"}
	replacement, err := StartLinuxTUNIngress(context.Background(), spec, resolvedAnswerBackend{answer: "198.18.63.2"})
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if replacement.backend.(*linuxRoutes).index == oldIndex {
		t.Fatal("normal interface allocation unexpectedly reused deleted index")
	}
	assertResolvedLink(t, conn, replacement, "updated.invalid")
	resolveThroughService(t, conn, "host.updated.invalid", "198.18.63.2")
	assertResolvedLink(t, conn, second, "other.invalid")
	for _, ingress := range []*tunIngress{replacement, second} {
		index := ingress.backend.(*linuxRoutes).index
		if err = ingress.Close(); err != nil {
			t.Fatal(err)
		}
		assertResolvedLinkGone(t, conn, index)
	}
}

func TestResolvedRealIsolatedCanceledRequestKeepsInterface(t *testing.T) {
	conn := isolatedResolved(t)
	pid, err := strconv.Atoi(os.Getenv("ECS_TEST_RESOLVED_PID"))
	if err != nil || pid <= 1 {
		t.Fatal("isolated resolved PID missing")
	}
	client, err := newResolvedClient(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	otherSpec := linuxTestSpec()
	otherSpec.Slot = 3
	otherSpec.Ref.Profile = "unaffected"
	otherSpec.Ref.Attachment = "unaffected"
	otherSpec.Names = []string{"unaffected.invalid"}
	other, err := StartLinuxTUNIngress(context.Background(), otherSpec, resolvedAnswerBackend{answer: "198.18.62.2"})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err = unix.Kill(pid, unix.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	defer unix.Kill(pid, unix.SIGCONT)
	waitResolvedCondition(t, "resolved process did not stop", func() bool {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		return err == nil && strings.Contains(string(data), "\nState:\tT")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	spec := linuxTestSpec()
	spec.Names = []string{"cancel.invalid"}
	result := make(chan error, 1)
	go func() {
		ingress, err := startLinuxTUNIngressWithDNS(ctx, spec, resolvedAnswerBackend{answer: "198.18.61.2"}, client.Call)
		if ingress != nil {
			err = errors.Join(err, ingress.Close())
		}
		result <- err
	}()
	waitResolvedCondition(t, "DNS mutation was not sent", func() bool {
		client.pendingMu.Lock()
		defer client.pendingMu.Unlock()
		return client.pending != nil && client.pending.serial != 0
	})
	var index int
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Name, "ecs") && iface.Index != other.backend.(*linuxRoutes).index {
			if index != 0 {
				t.Fatal("unexpected second test TUN")
			}
			index = iface.Index
		}
	}
	if index == 0 {
		t.Fatal("pending DNS mutation has no TUN")
	}
	cancel()
	select {
	case err = <-result:
		t.Fatalf("canceled caller abandoned in-flight service request: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err = net.InterfaceByIndex(index); err != nil {
		t.Fatalf("TUN removed before service reply: %v", err)
	}
	if err = unix.Kill(pid, unix.SIGCONT); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) || errors.Is(err, errResolvedOutcomeUnknown) {
			t.Fatalf("confirmed cancellation cleanup: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("request did not complete after resumed service")
	}
	assertResolvedLinkGone(t, conn, index)
	assertResolvedLink(t, conn, other, "unaffected.invalid")
	resolveThroughService(t, conn, "after-cancel.unaffected.invalid", "198.18.62.2")
}

func TestResolvedRealIsolatedCrashHelper(t *testing.T) {
	path := os.Getenv("ECS_RESOLVED_CRASH_READY")
	if path == "" {
		t.Skip("subprocess only")
	}
	isolatedResolved(t)
	spec := linuxTestSpec()
	spec.Names = []string{"crash.invalid"}
	ingress, err := StartLinuxTUNIngress(context.Background(), spec, resolvedAnswerBackend{answer: "198.18.61.2"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(ingress.backend.(*linuxRoutes).index)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestResolvedRealIsolatedCrashCleanup(t *testing.T) {
	conn := isolatedResolved(t)
	path := filepath.Join(t.TempDir(), "ready.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestResolvedRealIsolatedCrashHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "ECS_RESOLVED_CRASH_READY="+path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var index int
	waitResolvedCondition(t, "crash helper did not attach", func() bool {
		data, err := os.ReadFile(path)
		return err == nil && json.Unmarshal(data, &index) == nil && index > 0
	})
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("crash helper exited successfully")
	}
	assertResolvedLinkGone(t, conn, index)
}
