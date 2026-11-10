//go:build linux

package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"EasyConnectSuite/engine"
)

// The test gateway is started by build/linux/headless-smoke-test.sh in CI.
func TestLinuxGUIBackendMockGateway(t *testing.T) {
	if os.Getenv("ECS_GUI_GATEWAY_TEST") != "1" {
		t.Skip("requires isolated mock gateway")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	app := NewApp()
	profile, err := app.SaveProfile(Profile{ID: "mock-gui", Name: "mock", Protocol: "easyconnect", Server: "127.0.0.1", Port: 18445, Username: "mock-user", Password: "gui-private-canary", SocksPort: 19180, HTTPPort: 19181, DisableZjuConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewApp()
	found := false
	for _, saved := range reloaded.GetAppConfig().Profiles {
		if saved.ID == profile.ID && saved.Server == profile.Server && saved.SocksPort == profile.SocksPort {
			found = true
		}
	}
	if !found {
		t.Fatal("GUI profile did not persist")
	}
	result := reloaded.startConnectionID(profile.ID, func(opts engine.Options) (vpnEngine, error) { return engine.Start(opts) })
	if result != "OK" {
		t.Fatalf("GUI connection: %s", result)
	}
	t.Cleanup(func() { reloaded.stopConnection(profile.ID) })
	for _, port := range []string{"127.0.0.1:19180", "127.0.0.1:19181"} {
		conn, err := net.DialTimeout("tcp", port, 3*time.Second)
		if err != nil {
			t.Fatalf("GUI proxy %s: %v", port, err)
		}
		conn.Close()
	}
	for _, proxy := range []string{"socks", "http"} {
		addr := "127.0.0.1:19180"
		if proxy == "http" {
			addr = "127.0.0.1:19181"
		}
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err = conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if proxy == "socks" {
			if _, err = conn.Write([]byte{5, 1, 0}); err != nil {
				t.Fatal(err)
			}
			reply := make([]byte, 2)
			if _, err = io.ReadFull(conn, reply); err != nil || !bytes.Equal(reply, []byte{5, 0}) {
				t.Fatalf("SOCKS negotiation: %v %v", reply, err)
			}
			if _, err = conn.Write([]byte{5, 1, 0, 1, 10, 90, 0, 3, 0x30, 0x39}); err != nil {
				t.Fatal(err)
			}
			reply = make([]byte, 10)
			if _, err = io.ReadFull(conn, reply); err != nil || reply[1] != 0 {
				t.Fatalf("SOCKS connect: %v %v", reply, err)
			}
		} else {
			if _, err = io.WriteString(conn, "CONNECT 10.90.0.3:12345 HTTP/1.1\r\nHost: 10.90.0.3:12345\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(conn)
			line, err := reader.ReadString('\n')
			if err != nil || line != "HTTP/1.1 200 Connection established\r\n" {
				t.Fatalf("HTTP connect: %q %v", line, err)
			}
			for {
				line, err = reader.ReadString('\n')
				if err != nil {
					t.Fatal(err)
				}
				if line == "\r\n" {
					break
				}
			}
		}
		payload := []byte("gui-mock-" + proxy)
		if _, err = conn.Write(payload); err != nil {
			t.Fatal(err)
		}
		received := make([]byte, len(payload))
		if _, err = io.ReadFull(conn, received); err != nil || !bytes.Equal(received, payload) {
			t.Fatalf("%s payload: %q %v", proxy, received, err)
		}
		conn.Close()
	}
	if result = reloaded.stopConnection(profile.ID); result != "OK" {
		t.Fatalf("GUI stop: %s", result)
	}
	for _, port := range []string{"127.0.0.1:19180", "127.0.0.1:19181"} {
		conn, err := net.DialTimeout("tcp", port, time.Second)
		if err == nil {
			conn.Close()
			t.Fatalf("GUI proxy remained open: %s", port)
		}
	}
}
