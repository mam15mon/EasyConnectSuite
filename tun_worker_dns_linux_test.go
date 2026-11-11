package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

func requireWorkerResolvedNamespace(t *testing.T) int {
	t.Helper()
	if os.Getenv("ECS_REAL_RESOLVED_TEST") != "1" {
		t.Skip("requires private resolved and isolated namespaces")
	}
	for _, kind := range []string{"net", "mnt"} {
		current, err := os.Readlink("/proc/self/ns/" + kind)
		host := os.Getenv("ECS_HOST_" + strings.ToUpper(kind) + "_NS")
		if err != nil || host == "" || current == host {
			t.Fatalf("%s namespace not isolated: %v", kind, err)
		}
	}
	if os.Getenv("DBUS_SYSTEM_BUS_ADDRESS") == "" || os.Getenv("DBUS_SYSTEM_BUS_ADDRESS") != os.Getenv("DBUS_SESSION_BUS_ADDRESS") {
		t.Fatal("system bus not isolated")
	}
	pid, err := strconv.Atoi(os.Getenv("ECS_TEST_RESOLVED_PID"))
	if err != nil || pid <= 1 {
		t.Fatal("private resolved PID missing")
	}
	return pid
}

func waitWorkerCondition(t *testing.T, label string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal(label)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLinuxWorkerResolvedCancellation(t *testing.T) {
	pid := requireWorkerResolvedNamespace(t)
	for _, operation := range []string{"attach", "update"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "worker.log")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			previous := log.Writer()
			log.SetOutput(io.MultiWriter(previous, file))
			defer func() { log.SetOutput(previous); file.Close() }()
			ref := tunipc.Ref{Profile: "cancel-test", Session: "session", Attachment: "attachment", Policy: 1}
			spec := engine.TUNSpec{Ref: ref, Prefixes: []string{"198.18.85.0/24"}}
			w := &workerServer{authenticated: true, entries: map[string]*workerEntry{}, reservations: map[string]bool{}}
			defer func() {
				if err := w.cleanup(); err != nil {
					t.Errorf("cleanup: %v", err)
				}
			}()
			if operation == "update" {
				if _, err = w.handle(context.Background(), tunipc.Message{Op: "attach", Ref: spec.Ref, Spec: &spec}); err != nil {
					t.Fatal(err)
				}
				spec.Policy++
			}
			spec.Names = []string{"cancel.invalid"}
			if err = unix.Kill(pid, unix.SIGSTOP); err != nil {
				t.Fatal(err)
			}
			defer unix.Kill(pid, unix.SIGCONT)
			waitWorkerCondition(t, "resolved did not stop", func() bool {
				data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
				return err == nil && strings.Contains(string(data), "\nState:\tT")
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, err := w.handle(ctx, tunipc.Message{Op: operation, Ref: spec.Ref, Spec: &spec})
				result <- err
			}()
			defer func() {
				cancel()
				_ = unix.Kill(pid, unix.SIGCONT)
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Error("worker request remained pending after resumed service")
				}
			}()
			waitWorkerCondition(t, "worker did not send DNS request", func() bool {
				data, err := os.ReadFile(path)
				return err == nil && strings.Contains(string(data), "stage=wait_service_reply")
			})
			ifaces, err := net.Interfaces()
			if err != nil {
				t.Fatal(err)
			}
			var index int
			for _, iface := range ifaces {
				if strings.HasPrefix(iface.Name, "ecs") {
					if index != 0 {
						t.Fatal("unexpected second test interface")
					}
					index = iface.Index
				}
			}
			if index == 0 {
				t.Fatal("pending DNS request had no TUN")
			}
			cancel()
			select {
			case err = <-result:
				t.Fatalf("worker abandoned sent request before reply: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if _, err = net.InterfaceByIndex(index); err != nil {
				t.Fatalf("pending request lost interface: %v", err)
			}
			if err = unix.Kill(pid, unix.SIGCONT); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("worker cancellation: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("worker did not complete after real service reply")
			}
			if len(w.entries) != 0 || w.cleanupErr != nil {
				t.Fatalf("canceled request retained entry or cleanup uncertainty: %d %v", len(w.entries), w.cleanupErr)
			}
			if _, err = net.InterfaceByIndex(index); err == nil {
				t.Fatal("canceled worker request retained interface")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "stage=send_SetLinkDNS") || strings.Contains(string(data), "stage=send_SetLinkDomains") {
				t.Fatal("worker sent another DNS mutation after cancellation")
			}
		})
	}
}

type linuxWorkerEchoBackend struct{ target string }

func (b linuxWorkerEchoBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, b.target)
}
func (linuxWorkerEchoBackend) DNS(context.Context, []byte) ([]byte, []string, error) {
	return nil, nil, nil
}

func TestLinuxWorkerRequestLifetime(t *testing.T) {
	requireWorkerResolvedNamespace(t)
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(lo); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		packet := make([]byte, 1024)
		for {
			n, source, err := listener.ReadFrom(packet)
			if err != nil {
				return
			}
			_, _ = listener.WriteTo(packet[:n], source)
		}
	}()
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "lifetime", Session: "session", Attachment: "attachment", Policy: 1}, Prefixes: []string{"198.18.86.0/24"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ingress, err := startWorkerIngress(ctx, spec, linuxWorkerEchoBackend{target: listener.LocalAddr().String()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ingress.Close()
	cancel()
	echo := func(address string) {
		t.Helper()
		conn, err := net.DialTimeout("udp4", address, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err = conn.Write([]byte("live-after-reply")); err != nil {
			t.Fatal(err)
		}
		packet := make([]byte, 1024)
		n, err := conn.Read(packet)
		if err != nil || string(packet[:n]) != "live-after-reply" {
			t.Fatalf("request context canceled live ingress: %q %v", packet[:n], err)
		}
	}
	echo("198.18.86.2:12345")
	spec.Policy++
	spec.Prefixes = []string{"198.18.87.0/24"}
	updateCtx, cancelUpdate := context.WithCancel(context.Background())
	defer cancelUpdate()
	if err = ingress.UpdateSpec(updateCtx, spec); err != nil {
		t.Fatal(err)
	}
	cancelUpdate()
	echo("198.18.87.2:12345")
	if err = ingress.Close(); err != nil {
		t.Fatal(err)
	}
}
