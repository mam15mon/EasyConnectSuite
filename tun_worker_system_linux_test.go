package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	"github.com/miekg/dns"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

func TestLinuxWorkerHelper(t *testing.T) {
	if os.Getenv("ECS_LINUX_WORKER_HELPER") != "1" {
		return
	}
	if err := runTUNWorker([]string{"--tun-worker", os.Getenv("ECS_LINUX_WORKER_SOCKET"), os.Getenv("ECS_LINUX_PARENT_PID")}); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxWorkerProcessLifecycle(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	name := tunipc.NewName()
	listener, err := tunipc.Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
	cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	conn, pid, err := listener.AcceptWorker(ctx, executable)
	if err != nil {
		t.Fatal(err)
	}
	if pid != uint32(cmd.Process.Pid) {
		conn.Close()
		t.Fatalf("unexpected worker PID %d", pid)
	}
	peer := tunipc.NewPeer(conn, func(ctx context.Context, msg tunipc.Message) (tunipc.Message, error) {
		if msg.Op != "dial" || (msg.Network != "tcp" && msg.Network != "udp") || msg.Ref.Attachment != "test" {
			return tunipc.Message{}, fmt.Errorf("unexpected worker request")
		}
		dataName := tunipc.NewName()
		dataListener, err := tunipc.Listen(dataName)
		if err != nil {
			return tunipc.Message{}, err
		}
		go func() {
			defer dataListener.Close()
			acceptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			dataConn, err := dataListener.Accept(acceptCtx, pid)
			if err != nil {
				return
			}
			defer dataConn.Close()
			stream := tunipc.NewDataConn(dataConn, msg.Network == "udp")
			buf := make([]byte, 2048)
			for {
				n, err := stream.Read(buf)
				if err != nil {
					return
				}
				if _, err = stream.Write(buf[:n]); err != nil {
					return
				}
			}
		}()
		return tunipc.Message{Pipe: dataName}, nil
	}, nil)
	defer peer.Close()
	if _, err = peer.Call(ctx, tunipc.Message{Op: "hello"}); err != nil {
		t.Fatal(err)
	}
	ref := tunipc.Ref{Profile: "test", Session: "test", Attachment: "test", Policy: 1}
	spec := tunipc.Spec{Ref: ref, Slot: 5, Prefixes: []string{"198.18.71.0/24"}}
	if _, err = peer.Call(ctx, tunipc.Message{Op: "prepare", Ref: ref}); err != nil {
		t.Fatal(err)
	}
	if _, err = peer.Call(ctx, tunipc.Message{Op: "attach", Ref: ref, Spec: &spec}); err != nil {
		t.Fatal(err)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			found = true
		}
	}
	if !found {
		t.Fatal("worker did not create an interface")
	}
	for _, network := range []string{"tcp4", "udp4"} {
		flow, err := net.DialTimeout(network, "198.18.71.2:12345", 5*time.Second)
		if err != nil {
			t.Fatalf("%s dial: %v", network, err)
		}
		_ = flow.SetDeadline(time.Now().Add(5 * time.Second))
		payload := []byte("worker-" + network)
		if _, err = flow.Write(payload); err != nil {
			flow.Close()
			t.Fatal(err)
		}
		reply := make([]byte, len(payload))
		if _, err = io.ReadFull(flow, reply); err != nil {
			flow.Close()
			t.Fatalf("%s read: %v", network, err)
		}
		flow.Close()
		if string(reply) != string(payload) {
			t.Fatalf("%s payload mismatch", network)
		}
	}
	// Simulate the parent disappearing while the attachment is active.
	peer.Close()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err = <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not exit after control channel closed")
	}
	ifaces, err = net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			t.Fatal("worker left an interface behind")
		}
	}
}

type linuxManagerEcho struct{}

func (linuxManagerEcho) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	a, b := net.Pipe()
	go func() { defer b.Close(); _, _ = io.Copy(b, b) }()
	return a, nil
}
func (linuxManagerEcho) DNS(_ context.Context, data []byte) ([]byte, []string, error) {
	request := new(dns.Msg)
	if err := request.Unpack(data); err != nil {
		return nil, nil, err
	}
	response := new(dns.Msg)
	response.SetReply(request)
	response.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.IPv4(198, 18, 72, 2)}}
	answer, err := response.Pack()
	return answer, nil, err
}

func TestLinuxManagerWorkerLifecycle(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	manager := newLinuxTUNManager()
	manager.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "manager", Session: "session", Attachment: "attachment", Policy: 1}, Slot: 7, Prefixes: []string{"198.18.72.0/24"}}
	if err := manager.Prepare(ctx, spec.Profile); err != nil {
		t.Fatal(err)
	}
	attachment, err := manager.Attach(ctx, spec, linuxManagerEcho{})
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"tcp4", "udp4"} {
		conn, err := net.DialTimeout(network, "198.18.72.2:23456", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		payload := []byte("manager-" + network)
		if _, err = conn.Write(payload); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		result := make([]byte, len(payload))
		if _, err = io.ReadFull(conn, result); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		conn.Close()
		if string(result) != string(payload) {
			t.Fatalf("unexpected %s data", network)
		}
	}
	request := new(dns.Msg)
	request.SetQuestion("synthetic.invalid.", dns.TypeA)
	client := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	response, _, err := client.Exchange(request, "198.19.129.30:53")
	if err != nil {
		t.Fatalf("DNS over worker: %v", err)
	}
	if len(response.Answer) != 1 {
		t.Fatalf("unexpected DNS answer count: %d", len(response.Answer))
	}
	answer, ok := response.Answer[0].(*dns.A)
	if !ok || !answer.A.Equal(net.IPv4(198, 18, 72, 2)) {
		t.Fatalf("unexpected DNS answer: %v", response.Answer[0])
	}
	if err := attachment.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-attachment.Done():
	case <-ctx.Done():
		t.Fatal("attachment did not stop")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			t.Fatal("manager left an interface behind")
		}
	}
}

func TestLinuxManagerWorkerCrash(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	manager := newLinuxTUNManager()
	var worker *exec.Cmd
	manager.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		worker = cmd
		return cmd, nil
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "crash", Session: "session", Attachment: "attachment", Policy: 1}, Slot: 8, Prefixes: []string{"198.18.73.0/24"}}
	attachment, err := manager.Attach(ctx, spec, linuxManagerEcho{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp4", "198.18.73.2:23456", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = conn.Write([]byte("before-crash")); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadFull(conn, make([]byte, len("before-crash"))); err != nil {
		t.Fatal(err)
	}
	if err = worker.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-attachment.Done():
	case <-ctx.Done():
		t.Fatal("attachment did not terminate after worker crash")
	}
	if _, err = conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("active connection survived worker crash")
	}
	if err = attachment.Close(ctx); err == nil {
		t.Fatal("worker crash was not reported")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if len(iface.Name) >= 3 && iface.Name[:3] == "ecs" {
			t.Fatal("worker crash left an interface behind")
		}
	}
}

func TestLinuxManagerPolicyReplacement(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	manager := newLinuxTUNManager()
	manager.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "update", Session: "session", Attachment: "attachment", Policy: 1}, Slot: 9, Prefixes: []string{"198.18.74.0/24"}}
	attachment, err := manager.Attach(ctx, spec, linuxManagerEcho{})
	if err != nil {
		t.Fatal(err)
	}
	defer attachment.Close(ctx)
	checkFlow := func(address string) {
		t.Helper()
		conn, err := net.DialTimeout("tcp4", address, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err = conn.Write([]byte("policy")); err != nil {
			t.Fatal(err)
		}
		reply := make([]byte, 6)
		if _, err = io.ReadFull(conn, reply); err != nil || string(reply) != "policy" {
			t.Fatalf("echo %q: %v", reply, err)
		}
	}
	checkFlow("198.18.74.2:23456")
	spec.Policy++
	spec.Prefixes = []string{"198.18.75.0/24"}
	if err := attachment.Update(ctx, spec); err != nil {
		t.Fatal(err)
	}
	_, oldPrefix, _ := net.ParseCIDR("198.18.74.0/24")
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: oldPrefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 0 {
		t.Fatalf("old route remained: %+v %v", routes, err)
	}
	checkFlow("198.18.75.2:23456")
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(lo); err != nil {
		t.Fatal(err)
	}
	_, otherPrefix, _ := net.ParseCIDR("198.18.76.0/24")
	other := &netlink.Route{LinkIndex: lo.Attrs().Index, Dst: otherPrefix, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN}
	if err = netlink.RouteAdd(other); err != nil {
		t.Fatal(err)
	}
	defer netlink.RouteDel(other)
	spec.Policy++
	spec.Prefixes = []string{otherPrefix.String()}
	if err = attachment.Update(ctx, spec); err == nil {
		t.Fatal("conflicting replacement accepted")
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: otherPrefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 1 || routes[0].LinkIndex != lo.Attrs().Index {
		t.Fatalf("other route changed: %+v %v", routes, err)
	}
	_, failedPrefix, _ := net.ParseCIDR("198.18.75.0/24")
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: failedPrefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 0 {
		t.Fatalf("failed attachment route remained: %+v %v", routes, err)
	}
	_ = attachment.Close(ctx)
}

func TestLinuxManagerMultipleAttachments(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	manager := newLinuxTUNManager()
	manager.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	aSpec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "first", Session: "s1", Attachment: "a1", Policy: 1}, Slot: 10, Prefixes: []string{"198.18.84.0/24"}}
	bSpec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "second", Session: "s2", Attachment: "a2", Policy: 1}, Slot: 11, Prefixes: []string{"198.18.85.0/24"}}
	a, err := manager.Attach(ctx, aSpec, linuxManagerEcho{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(ctx)
	b, err := manager.Attach(ctx, bSpec, linuxManagerEcho{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close(ctx)
	flow := func(address string) net.Conn {
		t.Helper()
		conn, err := net.DialTimeout("tcp4", address, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		return conn
	}
	first := flow("198.18.84.2:23456")
	defer first.Close()
	second := flow("198.18.85.2:23456")
	defer second.Close()
	echo := func(c net.Conn, message string) {
		t.Helper()
		if _, err := c.Write([]byte(message)); err != nil {
			t.Fatal(err)
		}
		reply := make([]byte, len(message))
		if _, err := io.ReadFull(c, reply); err != nil || string(reply) != message {
			t.Fatalf("echo %q: %v", reply, err)
		}
	}
	echo(first, "first")
	echo(second, "second")
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
	echo(second, "still-connected")
	_, oldPrefix, _ := net.ParseCIDR("198.18.84.0/24")
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: oldPrefix, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	if err != nil || len(routes) != 0 {
		t.Fatalf("first route remains: %+v %v", routes, err)
	}
	if err := b.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
