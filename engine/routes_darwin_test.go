package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDarwinRoutesRetainsPartialOwnershipOnFailure(t *testing.T) {
	var calls [][]string
	routes := &darwinRoutes{name: "utun9", run: func(_ context.Context, args ...string) ([]byte, error) {
		if args[1] == "get" {
			return []byte("interface: utun9\n"), nil
		}
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 2 {
			return []byte("route exists"), errors.New("exit status 1")
		}
		return nil, nil
	}}
	manager := newRouteManager(routes)
	if err := manager.replace([]string{"192.0.2.0/24", "198.51.100.0/24"}); err == nil {
		t.Fatal("failed route must be reported")
	}
	if !manager.owned["192.0.2.0/24"] || manager.owned["198.51.100.0/24"] {
		t.Fatalf("incorrect ownership: %v", manager.owned)
	}
	if err := manager.close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"-n", "delete", "-net", "192.0.2.0/24", "-interface", "utun9"}
	if !reflect.DeepEqual(calls[2], want) {
		t.Fatalf("cleanup = %v", calls[2])
	}
}

func TestDarwinRoutesRejectsChangedInterface(t *testing.T) {
	routes := &darwinRoutes{name: "utun9", run: func(_ context.Context, args ...string) ([]byte, error) {
		if args[1] != "get" {
			t.Fatal("deleted route belonging to another interface")
		}
		return []byte("interface: lo0\n"), nil
	}}
	if _, err := routes.Apply(nil, []string{"192.0.2.0/24"}); err == nil {
		t.Fatal("changed route accepted")
	}
}

func TestDarwinRoutesRejectsInvalidInputsBeforeCommand(t *testing.T) {
	for _, test := range []struct{ name, prefix string }{{"en0", "192.0.2.0/24"}, {"utun;whoami", "192.0.2.0/24"}, {"utun1", "0.0.0.0/0"}, {"utun1", "192.0.2.1/24"}} {
		routes := &darwinRoutes{name: test.name, run: func(context.Context, ...string) ([]byte, error) {
			t.Fatal("invalid input reached route command")
			return nil, nil
		}}
		if _, err := routes.Apply([]string{test.prefix}, nil); err == nil {
			t.Fatalf("accepted %q %q", test.name, test.prefix)
		}
	}
}
