package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	"github.com/miekg/dns"
	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--tun-worker" {
		setupWorkerCrashLog()
		if err := runTUNWorker(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	// Application fixtures must not create directories beside real user profiles.
	testDataDir, err := os.MkdirTemp("", "VPN-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Setenv("APPDATA", testDataDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(testDataDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

type systemTestBackend struct{ target string }

func (b *systemTestBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" || (address != "198.51.100.77:443" && address != "198.51.100.78:443") {
		return nil, errors.New("denied system fixture destination")
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", b.target)
}
func (b *systemTestBackend) DNS(ctx context.Context, data []byte) ([]byte, []string, error) {
	q := new(dns.Msg)
	if err := q.Unpack(data); err != nil {
		return nil, nil, err
	}
	r := new(dns.Msg)
	r.SetReply(q)
	r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: net.ParseIP("198.51.100.77")}}
	wire, e := r.Pack()
	return wire, nil, e
}
func assertSystemResidue(t *testing.T, want int) {
	t.Helper()
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$routes=@(Get-NetRoute -AddressFamily IPv4 -PolicyStore ActiveStore | Where-Object { $_.InterfaceAlias -in @('EasyConnect Suite #60','EasyConnect Suite #61') -and $_.DestinationPrefix -in @('198.51.100.77/32','198.51.100.78/32') }); $dns=@(Get-DnsClientNrptRule | Where-Object { $_.Comment -in @('EasyConnectSuite resource TUN #60','EasyConnectSuite resource TUN #61') }); [Console]::Write(($routes.Count.ToString() + "," + $dns.Count.ToString()))`)
	data, e := cmd.CombinedOutput()
	if e != nil || strings.TrimSpace(string(data)) != fmt.Sprintf("%d,%d", want, want) {
		t.Fatalf("system resource counts %q want %d: %v", data, want, e)
	}
}

// Explicit opt-in: these tests show UAC and create only reserved fixture TUN slots/resources.
func TestTUNSystemUACAndTwoAttachments(t *testing.T) {
	if os.Getenv("ECS_TUN_SYSTEM_TEST") != "accept" {
		t.Skip("requires interactive UAC and an idle VPN application")
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("parent must have ordinary privileges for UAC acceptance")
	}
	pid := os.Getpid()
	manager := newTUNManager()
	defer manager.Close()
	ctx, c := context.WithTimeout(context.Background(), 3*time.Minute)
	defer c()
	if err := manager.Prepare(ctx, "system-61"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			go func() { defer conn.Close(); io.Copy(conn, conn) }()
		}
	}()
	backend := &systemTestBackend{target: listener.Addr().String()}
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "system-61", Session: "fixture", Attachment: "fixture-61", Policy: 1}, Slot: 61, Prefixes: []string{"198.51.100.77/32"}, Names: []string{".tun61.example"}}
	a, err := manager.Attach(ctx, spec, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	second := spec
	second.Profile = "system-60"
	second.Attachment = "fixture-60"
	second.Slot = 60
	second.Prefixes = []string{"198.51.100.78/32"}
	second.Names = []string{".tun60.example"}
	b, err := manager.Attach(ctx, second, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close(context.Background())
	assertSystemResidue(t, 2)
	for _, address := range []string{"198.51.100.77:443", "198.51.100.78:443"} {
		conn, e := net.DialTimeout("tcp", address, 10*time.Second)
		if e != nil {
			t.Fatal(e)
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, e = conn.Write([]byte("TUN system fixture")); e != nil {
			conn.Close()
			t.Fatal(e)
		}
		var got [18]byte
		_, e = io.ReadFull(conn, got[:])
		conn.Close()
		if e != nil || string(got[:]) != "TUN system fixture" {
			t.Fatalf("TUN system TCP %q %v", got, e)
		}
	}
	query := new(dns.Msg)
	query.SetQuestion("app.tun61.example.", dns.TypeA)
	answer, _, err := (&dns.Client{Timeout: 5 * time.Second}).Exchange(query, "198.19.128.246:53")
	if err != nil || len(answer.Answer) != 1 {
		t.Fatalf("TUN DNS %v %v", answer, err)
	}
	if os.Getpid() != pid || windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("parent identity or privileges changed")
	}
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
	assertSystemResidue(t, 1)
	if err := b.Close(ctx); err != nil {
		t.Fatal(err)
	}
	assertSystemResidue(t, 0)
	t.Logf("ordinary parent pid=%d; elevated worker pid=%d; two real adapters/routes/NRPT and TCP/DNS validated", pid, manager.pid)
}
func TestTUNSystemUACCancellation(t *testing.T) {
	if os.Getenv("ECS_TUN_SYSTEM_TEST") != "cancel" {
		t.Skip("requires clicking No on UAC")
	}
	manager := newTUNManager()
	defer manager.Close()
	if err := manager.Prepare(context.Background(), "system-cancel"); err == nil {
		t.Fatal("expected user to cancel UAC")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.peer != nil || len(manager.reservations) != 0 || len(manager.attachments) != 0 {
		t.Fatal("canceled UAC retained worker resources")
	}
}

func TestTUNSystemCrashFixture(t *testing.T) {
	if os.Getenv("ECS_TUN_CRASH_CHILD") != "1" {
		t.Skip("subprocess fixture")
	}
	manager := newTUNManager()
	ctx, c := context.WithTimeout(context.Background(), time.Minute)
	defer c()
	spec := engine.TUNSpec{Ref: tunipc.Ref{Profile: "crash-fixture", Session: "crash", Attachment: "crash-attachment", Policy: 1}, Slot: 61, Prefixes: []string{"198.51.100.77/32"}, Names: []string{".tun61.example"}}
	if _, err := manager.Attach(ctx, spec, &systemTestBackend{}); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("WORKER_READY %d\n", manager.pid)
	select {}
}
func TestTUNSystemParentCrashCleanup(t *testing.T) {
	if os.Getenv("ECS_TUN_SYSTEM_TEST") != "crash" {
		t.Skip("requires real system TUN fixture")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestTUNSystemCrashFixture$", "-test.v", "-test.timeout=90s")
	cmd.Env = append(os.Environ(), "ECS_TUN_CRASH_CHILD=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	ready := make(chan uint32, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		for scan.Scan() {
			var pid uint32
			if _, e := fmt.Sscanf(scan.Text(), "WORKER_READY %d", &pid); e == nil {
				ready <- pid
				return
			}
		}
	}()
	var workerPID uint32
	select {
	case workerPID = <-ready:
	case <-time.After(time.Minute):
		t.Fatal("crash fixture never attached")
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, workerPID)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	assertSystemResidue(t, 1)
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	result, err := windows.WaitForSingleObject(handle, 45000)
	if err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatal("Worker did not exit after parent crash", err)
	}
	assertSystemResidue(t, 0)
	t.Logf("force-killed ordinary parent; Worker %d cleaned routes and NRPT and exited", workerPID)
}
