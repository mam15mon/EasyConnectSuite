package engine

import (
	"context"
	"errors"
	"os"
	"reflect"
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
		if method == "SetLinkDomains" {
			return stop
		}
		return nil
	}
	if err := resolvedLinkDNS(context.Background(), call, 7, "198.19.128.2", []string{"example.invalid"}); !errors.Is(err, stop) {
		t.Fatalf("lost error: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"SetLinkDNS", "SetLinkDomains"}) {
		t.Fatalf("calls: %v", calls)
	}
	calls = nil
	if err := resolvedLinkDNS(context.Background(), call, 0, "198.19.128.2", nil); err == nil || len(calls) != 0 {
		t.Fatal("invalid index reached D-Bus")
	}
}

func TestResolvedDNSOwnerRollbackAndIdentity(t *testing.T) {
	fail := errors.New("domain update failed")
	var methods []string
	verify := func() error { return nil }
	owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return verify() }, call: func(_ context.Context, method string, _ ...any) error {
		methods = append(methods, method)
		if method == "SetLinkDomains" {
			return fail
		}
		return nil
	}}
	if err := owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, fail) {
		t.Fatalf("apply error: %v", err)
	}
	if !reflect.DeepEqual(methods, []string{"SetLinkDNS", "SetLinkDomains", "RevertLink"}) || owner.touched {
		t.Fatalf("rollback: %v touched=%v", methods, owner.touched)
	}
	methods = nil
	verify = func() error { return errors.New("interface changed") }
	if err := owner.Apply(context.Background(), []string{"example.invalid"}); err == nil || len(methods) != 0 {
		t.Fatal("changed interface reached D-Bus")
	}
}
func TestResolvedDNSOwnerRetainsFailedCleanup(t *testing.T) {
	fail := errors.New("revert failed")
	owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return nil }, call: func(_ context.Context, method string, _ ...any) error {
		if method == "RevertLink" {
			return fail
		}
		return nil
	}}
	if err := owner.Apply(context.Background(), []string{"example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); !errors.Is(err, fail) || !owner.touched {
		t.Fatalf("cleanup state: %v %v", err, owner.touched)
	}
	owner.call = func(context.Context, string, ...any) error { return nil }
	if err := owner.Close(context.Background()); err != nil || owner.touched {
		t.Fatalf("retry: %v", err)
	}
}

type resolvedTestService struct {
	mu          sync.Mutex
	methods     []string
	addresses   []resolvedAddress
	domains     []resolvedDomain
	failDomains bool
}

func (s *resolvedTestService) SetLinkDNS(index int32, values []resolvedAddress) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, "SetLinkDNS")
	if index != 7 {
		return dbus.NewError("org.test.Invalid", nil)
	}
	s.addresses = values
	return nil
}
func (s *resolvedTestService) SetLinkDomains(index int32, values []resolvedDomain) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, "SetLinkDomains")
	if index != 7 || s.failDomains {
		return dbus.NewError("org.test.Rejected", nil)
	}
	s.domains = values
	return nil
}
func (s *resolvedTestService) RevertLink(index int32) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methods = append(s.methods, "RevertLink")
	if index != 7 {
		return dbus.NewError("org.test.Invalid", nil)
	}
	s.addresses = nil
	s.domains = nil
	return nil
}
func TestResolvedPrivateBus(t *testing.T) {
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
	defer conn.Close()
	service := &resolvedTestService{}
	if err = conn.Export(service, dbus.ObjectPath("/org/freedesktop/resolve1"), "org.freedesktop.resolve1.Manager"); err != nil {
		t.Fatal(err)
	}
	reply, err := conn.RequestName("org.freedesktop.resolve1", dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", ref: linuxTestSpec(), call: resolvedSystemCall, verify: func() error { return nil }}
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	valid := len(service.addresses) == 1 && reflect.DeepEqual(service.addresses[0].Address, []byte{198, 19, 128, 2}) && len(service.domains) == 1 && service.domains[0].Name == "example.invalid" && service.domains[0].RouteOnly
	service.mu.Unlock()
	if !valid {
		t.Fatal("unexpected D-Bus payload")
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.failDomains = true
	service.methods = nil
	service.mu.Unlock()
	if err = owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, errResolvedOutcomeUnknown) {
		t.Fatalf("D-Bus error did not retain uncertainty: %v", err)
	}
	service.mu.Lock()
	methods := append([]string(nil), service.methods...)
	remaining := len(service.addresses)
	service.mu.Unlock()
	if !reflect.DeepEqual(methods, []string{"SetLinkDNS", "SetLinkDomains"}) || remaining != 1 || !owner.touched || !owner.unknown {
		t.Fatalf("unconfirmed outcome: %v remaining=%d", methods, remaining)
	}
}

// delayedResolvedService models a service that completes a timed-out request
// after the caller has already received a successful RevertLink reply.
type delayedResolvedService struct {
	entered    chan struct{}
	release    chan struct{}
	finished   chan struct{}
	mu         sync.Mutex
	configured bool
}

func (s *delayedResolvedService) SetLinkDNS(_ int32, _ []resolvedAddress) *dbus.Error {
	close(s.entered)
	<-s.release
	s.mu.Lock()
	s.configured = true
	s.mu.Unlock()
	close(s.finished)
	return nil
}
func (s *delayedResolvedService) RevertLink(_ int32) *dbus.Error {
	s.mu.Lock()
	s.configured = false
	s.mu.Unlock()
	return nil
}

func TestResolvedPrivateBusLateRequestAfterRollback(t *testing.T) {
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
	defer conn.Close()
	service := &delayedResolvedService{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	if err = conn.Export(service, dbus.ObjectPath("/org/freedesktop/resolve1"), "org.freedesktop.resolve1.Manager"); err != nil {
		t.Fatal(err)
	}
	reply, err := conn.RequestName("org.freedesktop.resolve1", dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- resolvedSystemCall(ctx, "SetLinkDNS", int32(7), []resolvedAddress{{Family: unix.AF_INET, Address: []byte{198, 19, 128, 2}}})
	}()
	select {
	case <-service.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("service did not receive request")
	}
	cancel()
	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not return after cancellation")
	}
	if err = resolvedSystemCall(context.Background(), "RevertLink", int32(7)); err != nil {
		t.Fatal(err)
	}
	close(service.release)
	select {
	case <-service.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("delayed request did not finish")
	}
	service.mu.Lock()
	configured := service.configured
	service.mu.Unlock()
	if !configured {
		t.Fatal("expected late request to change state after rollback")
	}
}

func TestResolvedDNSOwnerDoesNotClaimCanceledRequestClean(t *testing.T) {
	for _, method := range []string{"SetLinkDNS", "SetLinkDomains", "RevertLink"} {
		for _, failure := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(method+"/"+failure.Error(), func(t *testing.T) {
				var calls []string
				owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return nil }, call: func(_ context.Context, name string, _ ...any) error {
					calls = append(calls, name)
					if name == method {
						return failure
					}
					return nil
				}}
				err := owner.Apply(context.Background(), []string{"example.invalid"})
				if method == "RevertLink" {
					if err != nil {
						t.Fatal(err)
					}
					err = owner.Close(context.Background())
				}
				if !errors.Is(err, failure) || !errors.Is(err, errResolvedOutcomeUnknown) || !owner.unknown || !owner.touched {
					t.Fatalf("lost unconfirmed outcome: %v", err)
				}
				count := len(calls)
				if err = owner.Close(context.Background()); !errors.Is(err, errResolvedOutcomeUnknown) {
					t.Fatalf("close reported clean: %v", err)
				}
				if err = owner.Apply(context.Background(), []string{"example.invalid"}); !errors.Is(err, errResolvedOutcomeUnknown) {
					t.Fatalf("reapply accepted: %v", err)
				}
				if len(calls) != count {
					t.Fatalf("unconfirmed request followed by another call: %v", calls)
				}
			})
		}
	}
}

func TestResolvedDNSOwnerDoesNotRollbackUnsentRequest(t *testing.T) {
	for _, failingMethod := range []string{"SetLinkDNS", "SetLinkDomains"} {
		t.Run(failingMethod, func(t *testing.T) {
			var methods []string
			owner := &resolvedDNSOwner{index: 7, server: "198.19.128.2", verify: func() error { return nil }, call: func(_ context.Context, method string, _ ...any) error {
				methods = append(methods, method)
				if method == failingMethod {
					return errResolvedNotSent
				}
				return nil
			}}
			err := owner.Apply(context.Background(), []string{"example.invalid"})
			if !errors.Is(err, errResolvedNotSent) {
				t.Fatalf("missing unsent error: %v", err)
			}
			if failingMethod == "SetLinkDNS" {
				if owner.touched || owner.unknown || !reflect.DeepEqual(methods, []string{"SetLinkDNS"}) {
					t.Fatalf("first call changed state: %v %+v", methods, owner)
				}
				if err = owner.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				if !owner.touched || !owner.unknown || !errors.Is(err, errResolvedOutcomeUnknown) {
					t.Fatalf("partial update not retained: %v %+v", err, owner)
				}
				if err = owner.Close(context.Background()); !errors.Is(err, errResolvedOutcomeUnknown) {
					t.Fatalf("cleanup: %v", err)
				}
			}
			for _, method := range methods {
				if method == "RevertLink" {
					t.Fatal("unsent request triggered whole-link rollback")
				}
			}
		})
	}
}
