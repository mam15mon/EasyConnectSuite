package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"EasyConnectSuite/internal/tunipc"
	"github.com/miekg/dns"
)

func routeInterface(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if found && key == "interface" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func TestDarwinCrashFixture(t *testing.T) {
	if os.Getenv("ECS_DARWIN_CRASH_HELPER") != "1" {
		return
	}
	spec := TUNSpec{Ref: tunipc.Ref{Profile: "crash-test", Session: "test", Attachment: "test", Policy: 1}, Slot: 62, Prefixes: []string{"198.18.251.44/32"}, Names: []string{"ecs-crash-test.invalid"}}
	ingress, err := StartDarwinTUNIngress(context.Background(), spec, &darwinDNSBackend{calls: make(chan struct{}, 1)})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, ingress.name)
	select {}
}

func TestDarwinUTUNCrashEvidence(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestDarwinCrashFixture$")
	cmd.Env = append(os.Environ(), "ECS_DARWIN_CRASH_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
		} else {
			close(lines)
		}
	}()
	var name string
	select {
	case name = <-lines:
		if !strings.HasPrefix(name, "utun") {
			t.Fatalf("unexpected fixture output %q", name)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("crash fixture startup timed out")
	}
	path := "/etc/resolver/ecs-crash-test.invalid"
	t.Cleanup(func() {
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), "nameserver 198.19.128.250") {
			_ = os.Remove(path)
		}
	})
	partial, err := os.CreateTemp(darwinResolverDir, fmt.Sprintf("%s%d-%s-*", darwinResolverTempPrefix, cmd.Process.Pid, strings.Repeat("a", 48)))
	if err != nil {
		t.Fatal(err)
	}
	partialPath := partial.Name()
	if err := partial.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(partialPath) })
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	out, err := exec.Command("/sbin/route", "-n", "get", "198.18.251.44").CombinedOutput()
	if err == nil && routeInterface(out) == name {
		t.Fatalf("route still points to dead utun: %s", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected DNS residue for recovery verification: %v", err)
	}
	pending, err := CleanupStaleDarwinTUN(true)
	if err != nil || !pending {
		t.Fatalf("probe stale DNS: pending=%t err=%v", pending, err)
	}
	if _, err = CleanupStaleDarwinTUN(false); err != nil {
		t.Fatalf("clean stale DNS: %v", err)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale DNS remains: %v", err)
	}
	if _, err = os.Stat(partialPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial DNS temporary file remains: %v", err)
	}
}

type darwinEchoBackend struct{ calls chan string }

func (b *darwinEchoBackend) Dial(_ context.Context, network, address string) (net.Conn, error) {
	b.calls <- fmt.Sprintf("%s %s", network, address)
	client, server := net.Pipe()
	go func() { defer server.Close(); _, _ = io.Copy(server, server) }()
	return client, nil
}

type darwinDNSBackend struct{ calls chan struct{} }

func (*darwinDNSBackend) Dial(context.Context, string, string) (net.Conn, error) {
	return nil, fmt.Errorf("unexpected connection")
}
func (b *darwinDNSBackend) DNS(_ context.Context, data []byte) ([]byte, []string, error) {
	request := new(dns.Msg)
	if err := request.Unpack(data); err != nil {
		return nil, nil, err
	}
	response := new(dns.Msg)
	response.SetReply(request)
	if len(request.Question) == 1 && request.Question[0].Qtype == dns.TypeA {
		response.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}, A: net.ParseIP("198.18.250.42").To4()}}
		select {
		case b.calls <- struct{}{}:
		default:
		}
	}
	answer, err := response.Pack()
	return answer, nil, err
}

func TestDarwinUTUNSystemDNS(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	const domain = "ecs-tun-test.invalid"
	spec := TUNSpec{Ref: tunipc.Ref{Profile: "dns-test", Session: "test", Attachment: "test", Policy: 1}, Slot: 62, Prefixes: []string{"198.18.250.42/32"}, Names: []string{domain}}
	backend := &darwinDNSBackend{calls: make(chan struct{}, 1)}
	ingress, err := StartDarwinTUNIngress(context.Background(), spec, backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ingress.Close(); err != nil {
			t.Errorf("cleanup ingress: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var output []byte
	for attempt := 0; ; attempt++ {
		output, err = exec.CommandContext(ctx, "/usr/bin/dscacheutil", "-q", "host", "-a", "name", fmt.Sprintf("host%d.%s", attempt, domain)).CombinedOutput()
		if err == nil && strings.Contains(string(output), "198.18.250.42") {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("system DNS did not resolve within deadline after %d attempts: %s: %v", attempt+1, output, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	select {
	case <-backend.calls:
	case <-time.After(time.Second):
		t.Fatal("system DNS did not use VPN resolver")
	}
}

func TestDarwinRouteCleanupDoesNotDeleteReplacement(t *testing.T) {
	if os.Getenv("ECS_DARWIN_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires a disposable macOS host and root")
	}
	spec := TUNSpec{Ref: tunipc.Ref{Profile: "route-test", Session: "test", Attachment: "test", Policy: 1}, Slot: 62, Prefixes: []string{"198.18.251.43/32"}}
	ingress, err := StartDarwinTUNIngress(context.Background(), spec, &darwinEchoBackend{calls: make(chan string, 1)})
	if err != nil {
		t.Fatal(err)
	}
	name := ingress.name
	t.Cleanup(func() {
		if err := ingress.Close(); err != nil {
			t.Error(err)
		}
	})
	const target = "198.18.251.43/32"
	t.Cleanup(func() { _, _ = exec.Command("/sbin/route", "-n", "delete", "-net", target).CombinedOutput() })
	for _, args := range [][]string{{"-n", "delete", "-net", target, "-interface", name}, {"-n", "add", "-net", target, "-interface", "lo0"}} {
		if out, err := exec.Command("/sbin/route", args...).CombinedOutput(); err != nil {
			t.Fatalf("change test route: %s: %v", out, err)
		}
	}
	if err := ingress.Close(); err != nil {
		t.Fatalf("close ingress: %v", err)
	}
	out, err := exec.Command("/sbin/route", "-n", "get", "198.18.251.43").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "interface: lo0") {
		t.Fatalf("replacement route lost: %s: %v", out, err)
	}
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
	routes := newRouteManager(&darwinRoutes{name: name, ref: spec, run: func(ctx context.Context, args ...string) ([]byte, error) {
		out, err := exec.CommandContext(ctx, "/sbin/route", args...).CombinedOutput()
		t.Logf("route add output: %s (error: %v)", out, err)
		return out, err
	}})
	if err := routes.replace([]string{testRoute}); err != nil {
		t.Fatalf("add route: %v", err)
	}
	if out, routeErr := exec.Command("/sbin/route", "-n", "get", destination).CombinedOutput(); routeErr != nil || routeInterface(out) != name {
		t.Fatalf("new route does not use %s immediately after add: %s: %v", name, out, routeErr)
	}
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
		out, routeErr := exec.Command("/sbin/route", "-n", "get", destination).CombinedOutput()
		t.Fatalf("test packet did not reach %s; route get: %s: %v", name, out, routeErr)
	}
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
	if out, err := exec.Command("/sbin/route", "-n", "get", destination).CombinedOutput(); err == nil && routeInterface(out) == name {
		t.Fatalf("route remains on closed interface: %s", out)
	}
}
