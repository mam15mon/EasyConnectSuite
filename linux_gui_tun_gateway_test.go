//go:build linux

package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"github.com/sagernet/netlink"
)

// The shell harness supplies isolated network/mount namespaces and resolved.
// It launches the real Worker directly; it does not test a polkit dialog.
func TestLinuxGUIBackendTUNMockGateway(t *testing.T) {
	if os.Getenv("ECS_GUI_TUN_GATEWAY_TEST") != "1" {
		t.Skip("requires isolated TUN mock gateway and resolved")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	manager := newLinuxTUNManager()
	manager.launch = func(name string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxWorkerHelper$")
		cmd.Env = append(os.Environ(), "ECS_LINUX_WORKER_HELPER=1", "ECS_LINUX_WORKER_SOCKET="+name, "ECS_LINUX_PARENT_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stderr = os.Stderr
		return cmd, cmd.Start()
	}
	app := NewApp()
	app.tunManager = manager
	profile, err := app.SaveProfile(Profile{ID: "gui-tun", Name: "isolated TUN", Protocol: "easyconnect", Server: os.Getenv("ECS_MOCK_GATEWAY_HOST"), Port: 18445, Username: "mock-user", Password: "gui-tun-private-canary", SocksPort: 19180, HTTPPort: 19181, DisableZjuConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.stopConnection(profile.ID); manager.Close() })
	if result := app.startConnectionID(profile.ID, func(opts engine.Options) (vpnEngine, error) { return engine.Start(opts) }); result != "OK" {
		t.Fatalf("GUI engine start: %s", result)
	}
	loginCount := func() int {
		data, err := os.ReadFile(os.Getenv("ECS_MOCK_GATEWAY_LOG"))
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(data), "login=")
	}
	beforeLogins := loginCount()
	beforeStatus := app.GetStatus()
	flows := make([]net.Conn, 0, 2)
	for _, port := range []string{"19180", "19181"} {
		conn, err := net.DialTimeout("tcp4", "127.0.0.1:"+port, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if port == "19180" {
			if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
				t.Fatal(err)
			}
			var greeting [2]byte
			if _, err := io.ReadFull(conn, greeting[:]); err != nil || greeting != [2]byte{5, 0} {
				t.Fatalf("SOCKS greeting: %v %v", greeting, err)
			}
			if _, err := conn.Write([]byte{5, 1, 0, 1, 10, 90, 0, 3, 0x30, 0x39}); err != nil {
				t.Fatal(err)
			}
			var reply [10]byte
			if _, err := io.ReadFull(conn, reply[:]); err != nil || reply[1] != 0 {
				t.Fatalf("SOCKS connect: %v %v", reply, err)
			}
		} else {
			if _, err := io.WriteString(conn, "CONNECT 10.90.0.3:12345 HTTP/1.1\r\nHost: 10.90.0.3:12345\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("HTTP CONNECT: %d", response.StatusCode)
			}
		}
		flows = append(flows, conn)
		linuxGUITUNEcho(t, conn, "before-tun")
	}
	for _, enabled := range []bool{true, false, true} {
		if err := app.SetProfileTUNMode(profile.ID, enabled); err != nil {
			t.Fatal(err)
		}
		status := app.GetStatus()
		if !status.Running || status.State != "connected" || status.ClientIP != beforeStatus.ClientIP || status.TUNMode != enabled {
			t.Fatalf("unexpected GUI status after TUN=%v: %+v", enabled, status)
		}
		for _, conn := range flows {
			linuxGUITUNEcho(t, conn, "proxy-survives-tun-toggle")
		}
		if !enabled {
			linuxGUIAssertNoTUN(t)
			continue
		}
		// SetTUN returns after attachment; DNS discovery then publishes policy 2
		// asynchronously. Wait for its completed Worker reply before inspecting
		// the interface, which that update deliberately replaces.
		deadline := time.Now().Add(15 * time.Second)
		for {
			manager.op.Lock()
			manager.mu.Lock()
			updated := false
			for _, attachment := range manager.attachments {
				attachment.mu.RLock()
				updated = attachment.spec.Profile == profile.ID && attachment.spec.Policy >= 2
				attachment.mu.RUnlock()
				if updated {
					break
				}
			}
			manager.mu.Unlock()
			manager.op.Unlock()
			if updated {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("initial DNS policy update did not complete")
			}
			time.Sleep(10 * time.Millisecond)
		}
		routes, err := netlink.RouteGet(net.IPv4(10, 90, 0, 3))
		if err != nil || len(routes) == 0 {
			t.Fatalf("TUN resource route: %v", err)
		}
		link, err := netlink.LinkByIndex(routes[0].LinkIndex)
		if err != nil || !strings.HasPrefix(link.Attrs().Name, "ecs") {
			t.Fatalf("resource does not route through TUN: %v", err)
		}
		for _, network := range []string{"tcp4", "udp4"} {
			conn, err := net.DialTimeout(network, "10.90.0.3:12345", 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			payload := "gui-system-" + network
			if network == "udp4" {
				payload = "ecs-udp-test"
			}
			linuxGUITUNEcho(t, conn, payload)
			conn.Close()
		}
		output, err := exec.Command("resolvectl", "query", "--type=A", "--cache=no", "synthetic.internal").CombinedOutput()
		if err != nil || !bytes.Contains(output, []byte("10.90.0.3")) {
			t.Fatalf("system split DNS: %s %v", output, err)
		}
	}
	if loginCount() != beforeLogins {
		t.Fatal("TUN hot toggle reauthenticated the VPN")
	}
	if result := app.stopConnection(profile.ID); result != "OK" {
		t.Fatal(result)
	}
	manager.Close()
	linuxGUIAssertNoTUN(t)
	for _, port := range []string{"19180", "19181"} {
		if conn, err := net.DialTimeout("tcp4", "127.0.0.1:"+port, time.Second); err == nil {
			conn.Close()
			t.Fatalf("proxy %s remained open after stop", port)
		}
	}
}

func linuxGUITUNEcho(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, payload); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, reply); err != nil || string(reply) != payload {
		t.Fatalf("echo %q: %q %v", payload, reply, err)
	}
}

func linuxGUIAssertNoTUN(t *testing.T) {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Name, "ecs") && iface.Name != "ecs-client" {
			t.Fatalf("TUN interface remained after detach: %s", iface.Name)
		}
	}
}
