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
