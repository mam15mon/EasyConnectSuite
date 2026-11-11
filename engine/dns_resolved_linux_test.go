package engine

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

func TestResolvedDNSArguments(t *testing.T) {
	addresses, domains, err := resolvedDNSArguments("198.19.128.2", []string{".EXAMPLE.invalid", "example.invalid", "internal.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[0].Family != unix.AF_INET || !reflect.DeepEqual(addresses[0].Address, []byte{198, 19, 128, 2}) {
		t.Fatalf("addresses: %+v", addresses)
	}
	if len(domains) != 2 || domains[0].Name != "example.invalid" || !domains[0].RouteOnly || domains[1].Name != "internal.invalid" {
		t.Fatalf("domains: %+v", domains)
	}
	if got := dbus.SignatureOf(addresses).String(); got != "a(iay)" {
		t.Fatalf("address D-Bus signature %q", got)
	}
	if got := dbus.SignatureOf(domains).String(); got != "a(sb)" {
		t.Fatalf("domain D-Bus signature %q", got)
	}
	for _, names := range [][]string{{"."}, {"bad..example"}, {"example/invalid"}} {
		if _, _, err := resolvedDNSArguments("198.19.128.2", names); err == nil {
			t.Fatalf("invalid names accepted: %v", names)
		}
	}
}
func TestResolvedLinkDNSOrderAndFailure(t *testing.T) {
	var calls []string
	stop := errors.New("domains failed")
	call := func(_ context.Context, method string, args ...any) error {
		calls = append(calls, method)
		if len(args) != 2 || args[0] != int32(7) {
			t.Fatalf("bad arguments: %v", args)
		}
		if method == "SetLinkDefaultRoute" && args[1] != false {
			t.Fatal("default DNS route enabled")
		}
		if method == "SetLinkDomains" {
			return stop
		}
		return nil
	}
	if err := resolvedLinkDNS(context.Background(), call, 7, "198.19.128.2", []string{"example.invalid"}); !errors.Is(err, stop) {
		t.Fatalf("lost error: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"SetLinkDefaultRoute", "SetLinkDNS", "SetLinkDomains"}) {
		t.Fatalf("calls: %v", calls)
	}
	calls = nil
	if err := resolvedLinkDNS(context.Background(), call, 0, "198.19.128.2", nil); err == nil || len(calls) != 0 {
		t.Fatal("invalid index reached D-Bus")
	}
}

func TestResolvedDNSOwnerConfirmedFailureAndIdentity(t *testing.T) {
	fail := errors.New("confirmed domain rejection")
	var methods []string
	verify := func() error { return nil }
	owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return verify() }, call: func(_ context.Context, method string, _ ...any) error {
		methods = append(methods, method)
		if method == "SetLinkDomains" {
			return fail
		}
		return nil
	}}
	if err := owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, fail) || errors.Is(err, errResolvedOutcomeUnknown) {
		t.Fatalf("apply error: %v", err)
	}
	if owner.unknown {
		t.Fatal("a service rejection is a confirmed outcome")
	}
	count := len(methods)
	if err := owner.Close(context.Background()); err != nil || len(methods) != count {
		t.Fatalf("cleanup must not send RevertLink: %v %v", err, methods)
	}
	verify = func() error { return errors.New("interface changed") }
	if err := owner.Apply(context.Background(), []string{"example.invalid"}); err == nil || len(methods) != count {
		t.Fatal("changed interface reached D-Bus")
	}
}

func TestResolvedDNSOwnerRetainsUnknownOutcome(t *testing.T) {
	for _, method := range []string{"SetLinkDefaultRoute", "SetLinkDNS", "SetLinkDomains"} {
		t.Run(method, func(t *testing.T) {
			var calls []string
			owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return nil }, call: func(_ context.Context, name string, _ ...any) error {
				calls = append(calls, name)
				if name == method {
					return errors.Join(dbus.ErrClosed, errResolvedOutcomeUnknown)
				}
				return nil
			}}
			if err := owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, errResolvedOutcomeUnknown) || !owner.unknown {
				t.Fatalf("lost uncertainty: %v", err)
			}
			count := len(calls)
			if err := owner.Close(context.Background()); !errors.Is(err, errResolvedOutcomeUnknown) {
				t.Fatalf("cleanup reported success: %v", err)
			}
			if err := owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, errResolvedOutcomeUnknown) {
				t.Fatalf("reapply accepted: %v", err)
			}
			if len(calls) != count {
				t.Fatalf("unknown outcome followed by mutation: %v", calls)
			}
		})
	}
}

func TestResolvedDNSOwnerUnsentRequestDoesNotRequireRollback(t *testing.T) {
	for _, method := range []string{"SetLinkDefaultRoute", "SetLinkDNS", "SetLinkDomains"} {
		t.Run(method, func(t *testing.T) {
			var calls []string
			owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return nil }, call: func(_ context.Context, name string, _ ...any) error {
				calls = append(calls, name)
				if name == method {
					return errors.Join(context.Canceled, errResolvedNotSent)
				}
				return nil
			}}
			if err := owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, errResolvedNotSent) || owner.unknown {
				t.Fatalf("unsent classification: %v", err)
			}
			count := len(calls)
			if err := owner.Close(context.Background()); err != nil || len(calls) != count {
				t.Fatalf("cleanup must only delete owned link: %v %v", err, calls)
			}
		})
	}
}

type resolvedTestService struct {
	mu           sync.Mutex
	methods      []string
	addresses    []resolvedAddress
	domains      []resolvedDomain
	defaultRoute bool
	failDomains  bool
	entered      chan struct{}
	release      chan struct{}
}

func (s *resolvedTestService) SetLinkDefaultRoute(index int32, value bool) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, "SetLinkDefaultRoute")
	s.defaultRoute = value
	return nil
}
func (s *resolvedTestService) SetLinkDNS(index int32, values []resolvedAddress) *dbus.Error {
	if s.entered != nil {
		close(s.entered)
		<-s.release
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, "SetLinkDNS")
	s.addresses = values
	return nil
}
func (s *resolvedTestService) SetLinkDomains(index int32, values []resolvedDomain) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, "SetLinkDomains")
	if s.failDomains {
		return dbus.NewError("org.test.Rejected", []any{"PRIVATE_DNS_REPLY_CANARY"})
	}
	s.domains = values
	return nil
}

func privateResolvedService(t *testing.T, service *resolvedTestService) *dbus.Conn {
	t.Helper()
	if os.Getenv("ECS_RESOLVED_BUS_TEST") != "1" {
		t.Skip("requires a private D-Bus session")
	}
	if os.Getenv("DBUS_SYSTEM_BUS_ADDRESS") != os.Getenv("DBUS_SESSION_BUS_ADDRESS") || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Fatal("system bus is not isolated")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err = conn.Export(service, dbus.ObjectPath("/org/freedesktop/resolve1"), "org.freedesktop.resolve1.Manager"); err != nil {
		t.Fatal(err)
	}
	reply, err := conn.RequestName("org.freedesktop.resolve1", dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	return conn
}

func TestResolvedPrivateBus(t *testing.T) {
	service := &resolvedTestService{defaultRoute: true}
	privateResolvedService(t, service)
	client, err := newResolvedClient(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", call: client.Call, verify: func() error { return nil }}
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	valid := !service.defaultRoute && len(service.addresses) == 1 && reflect.DeepEqual(service.addresses[0].Address, []byte{198, 19, 128, 2}) && len(service.domains) == 1 && service.domains[0].RouteOnly
	service.failDomains = true
	service.mu.Unlock()
	if !valid {
		t.Fatal("unexpected D-Bus payload")
	}
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); err == nil || errors.Is(err, errResolvedOutcomeUnknown) || strings.Contains(err.Error(), "PRIVATE_DNS_REPLY_CANARY") {
		t.Fatalf("service error classification/redaction: %v", err)
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.addresses) != 1 || len(service.methods) != 6 {
		t.Fatal("owner release changed DNS before link removal")
	}
}

func TestResolvedPrivateBusCancellationWaitsForServiceReply(t *testing.T) {
	service := &resolvedTestService{entered: make(chan struct{}), release: make(chan struct{})}
	privateResolvedService(t, service)
	client, err := newResolvedClient(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- client.Call(ctx, "SetLinkDNS", int32(7), []resolvedAddress{{Family: unix.AF_INET, Address: []byte{198, 19, 128, 2}}})
	}()
	select {
	case <-service.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request not received")
	}
	cancel()
	select {
	case err = <-result:
		close(service.release)
		t.Fatalf("cancellation returned before service reply: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(service.release)
	select {
	case err = <-result:
		if err != nil {
			t.Fatalf("completed request: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service completion not observed")
	}
	if err = client.Call(ctx, "SetLinkDomains", int32(7), []resolvedDomain{}); !errors.Is(err, errResolvedNotSent) {
		t.Fatalf("next request was not canceled: %v", err)
	}
}

func TestResolvedPrivateBusPinsUniqueOwner(t *testing.T) {
	first := &resolvedTestService{}
	conn := privateResolvedService(t, first)
	client, err := newResolvedClient(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = conn.ReleaseName("org.freedesktop.resolve1"); err != nil {
		t.Fatal(err)
	}
	second := &resolvedTestService{}
	privateResolvedService(t, second)
	if err = client.Call(context.Background(), "SetLinkDefaultRoute", int32(7), false); err != nil {
		t.Fatal(err)
	}
	first.mu.Lock()
	count := len(first.methods)
	first.mu.Unlock()
	second.mu.Lock()
	replacementCount := len(second.methods)
	second.mu.Unlock()
	if count != 1 || replacementCount != 0 {
		t.Fatalf("request retargeted to new owner: first=%d second=%d", count, replacementCount)
	}
}

func TestResolvedPrivateBusDisconnectIsUnknown(t *testing.T) {
	service := &resolvedTestService{entered: make(chan struct{}), release: make(chan struct{})}
	conn := privateResolvedService(t, service)
	client, err := newResolvedClient(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result := make(chan error, 1)
	go func() { result <- client.Call(context.Background(), "SetLinkDNS", int32(7), []resolvedAddress{}) }()
	select {
	case <-service.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request not received")
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	defer close(service.release)
	select {
	case err = <-result:
		if !errors.Is(err, errResolvedOutcomeUnknown) {
			t.Fatalf("unconfirmed request reported completed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service disconnect not observed")
	}
}

func TestResolvedReplyMustMatchSenderAndSerial(t *testing.T) {
	p := &resolvedPending{serial: 12, reply: make(chan error, 1)}
	client := &resolvedClient{owner: ":1.7", pending: p}
	message := func(sender string, serial uint32) *dbus.Message {
		return &dbus.Message{Type: dbus.TypeMethodReply, Headers: map[dbus.HeaderField]dbus.Variant{dbus.FieldSender: dbus.MakeVariant(sender), dbus.FieldReplySerial: dbus.MakeVariant(serial)}}
	}
	client.incoming(message(":1.8", 12))
	client.incoming(message(":1.7", 13))
	select {
	case <-p.reply:
		t.Fatal("unrelated reply completed request")
	default:
	}
	client.incoming(message(":1.7", 12))
	select {
	case err := <-p.reply:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("matching service reply ignored")
	}
}

func TestResolvedSystemCallConnectFailureIsUnsent(t *testing.T) {
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+t.TempDir()+"/missing")
	err := resolvedSystemCall(context.Background(), "SetLinkDNS", int32(7), []resolvedAddress{})
	if !errors.Is(err, errResolvedNotSent) || errors.Is(err, errResolvedOutcomeUnknown) {
		t.Fatalf("connect failure classification: %v", err)
	}
}

func TestResolvedSystemCallCanceledBeforeConnect(t *testing.T) {
	// A missing socket distinguishes the context check from a dial failure.
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+t.TempDir()+"/missing")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := resolvedSystemCall(ctx, "SetLinkDNS", int32(7), []resolvedAddress{})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errResolvedNotSent) || errors.Is(err, errResolvedOutcomeUnknown) {
		t.Fatalf("canceled request classification: %v", err)
	}
}

func TestResolvedSystemCallCanceledDuringAuthentication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+path)
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- resolvedSystemCall(ctx, "SetLinkDNS", int32(7), []resolvedAddress{}) }()
	var peer net.Conn
	select {
	case peer = <-accepted:
		defer peer.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("bus connection did not arrive")
	}
	// Read the authentication preamble, then leave the server silent.
	_ = peer.SetReadDeadline(time.Now().Add(5 * time.Second))
	var buf [16]byte
	if _, err := peer.Read(buf[:]); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errResolvedNotSent) || errors.Is(err, errResolvedOutcomeUnknown) {
			t.Fatalf("canceled authentication: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("authentication did not stop after cancellation")
	}
}

func TestResolvedSystemCallCanceledDuringHello(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+path)
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- resolvedSystemCall(ctx, "SetLinkDNS", int32(7), []resolvedAddress{}) }()
	var peer net.Conn
	select {
	case peer = <-accepted:
		defer peer.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("bus connection did not arrive")
	}
	if err = peer.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(peer)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "AUTH") {
		t.Fatalf("initial authentication: %q %v", line, err)
	}
	if _, err = peer.Write([]byte("REJECTED EXTERNAL\r\n")); err != nil {
		t.Fatal(err)
	}
	line, err = reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "AUTH EXTERNAL") {
		t.Fatalf("external authentication: %q %v", line, err)
	}
	if _, err = peer.Write([]byte("DATA\r\n")); err != nil {
		t.Fatal(err)
	}
	line, err = reader.ReadString('\n')
	if err != nil || line != "DATA\r\n" && !strings.HasPrefix(line, "DATA ") {
		t.Fatalf("authentication data: %q %v", line, err)
	}
	if _, err = peer.Write([]byte("OK 0123456789abcdef0123456789abcdef\r\n")); err != nil {
		t.Fatal(err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(line, "NEGOTIATE_UNIX_FD") {
		if _, err = peer.Write([]byte("ERROR unsupported\r\n")); err != nil {
			t.Fatal(err)
		}
		line, err = reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
	}
	if line != "BEGIN\r\n" {
		t.Fatalf("expected BEGIN, got %q", line)
	}
	msg, err := dbus.DecodeMessage(reader)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Headers[dbus.FieldMember].Value() != "Hello" {
		t.Fatalf("expected Hello, got %v", msg.Headers[dbus.FieldMember])
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errResolvedNotSent) || errors.Is(err, errResolvedOutcomeUnknown) {
			t.Fatalf("canceled Hello: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Hello did not stop after cancellation")
	}
}

func TestResolvedSystemCallSaturatedBusQueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.sock")
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Listen(fd, 0); err != nil {
		t.Fatal(err)
	}
	queued, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer queued.Close()
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- resolvedSystemCall(ctx, "SetLinkDNS", int32(7), []resolvedAddress{}) }()
	select {
	case err = <-result:
		t.Logf("saturated bus connection returned before cancellation: %v", err)
	case <-time.After(200 * time.Millisecond):
		cancel()
		select {
		case err = <-result:
			t.Logf("saturated bus connection returned after cancellation: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("saturated bus connection did not stop after cancellation")
		}
	}
	if !errors.Is(err, errResolvedNotSent) || errors.Is(err, errResolvedOutcomeUnknown) {
		t.Fatalf("saturated bus request classification: %v", err)
	}
}
