package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

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
