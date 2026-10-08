package engine

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/mythologyli/zju-connect/client"
)

type fakeRoutes struct {
	present map[string]bool
	deleted []string
	fail    string
}

func (f *fakeRoutes) Apply(add, remove []string) (routeResult, error) {
	var result routeResult
	for _, prefix := range add {
		if prefix == f.fail {
			return result, errors.New("simulated failure")
		}
		if !f.present[prefix] {
			f.present[prefix] = true
			result.Added = append(result.Added, prefix)
		}
	}
	for _, prefix := range remove {
		delete(f.present, prefix)
		f.deleted = append(f.deleted, prefix)
		result.Removed = append(result.Removed, prefix)
	}
	return result, nil
}

func TestRoutesPreserveExistingEntries(t *testing.T) {
	backend := &fakeRoutes{present: map[string]bool{"10.0.0.0/8": true}}
	manager := newRouteManager(backend)
	if err := manager.replace([]string{"10.0.0.0/8", "172.31.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.close(); err != nil {
		t.Fatal(err)
	}
	if !backend.present["10.0.0.0/8"] {
		t.Fatal("deleted an existing route")
	}
	if !reflect.DeepEqual(backend.deleted, []string{"172.31.0.0/16"}) {
		t.Fatalf("deleted = %v", backend.deleted)
	}
	if err := manager.add("10.0.0.5/32"); err == nil {
		t.Fatal("added route after shutdown")
	}
}

func TestRoutesRefreshRemovesPreviousSession(t *testing.T) {
	backend := &fakeRoutes{present: map[string]bool{}}
	manager := newRouteManager(backend)
	if err := manager.replace([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.add("192.0.2.42/32"); err != nil {
		t.Fatal(err)
	}
	if err := manager.replace([]string{"172.31.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	if len(backend.present) != 1 || !backend.present["172.31.0.0/16"] {
		t.Fatalf("stale routes: %v", backend.present)
	}
}

func TestPartialAddFailureCanBeCleanedUp(t *testing.T) {
	backend := &fakeRoutes{present: map[string]bool{}, fail: "172.31.0.0/16"}
	manager := newRouteManager(backend)
	if err := manager.replace([]string{"10.0.0.0/8", "172.31.0.0/16"}); err == nil {
		t.Fatal("expected failure")
	}
	if err := manager.close(); err != nil {
		t.Fatal(err)
	}
	if len(backend.present) != 0 {
		t.Fatalf("leaked route: %v", backend.present)
	}
}

func ipRange(start, end string) client.IPResource {
	return client.IPResource{IPMin: net.ParseIP(start), IPMax: net.ParseIP(end), PortMin: 1, PortMax: 65535, Protocol: "all"}
}

func TestResourcePrefixesKeepServerSpecificity(t *testing.T) {
	got, err := resourcePrefixes([]client.IPResource{
		ipRange("10.0.0.0", "10.255.255.255"),
		ipRange("172.30.0.0", "172.30.255.255"),
		ipRange("172.31.0.0", "172.31.255.255"),
		ipRange("10.42.0.0", "10.42.255.255"),
		ipRange("10.0.0.0", "10.255.255.255"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.0/8", "10.42.0.0/16", "172.30.0.0/16", "172.31.0.0/16"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixes = %v, want %v", got, want)
	}
}

func TestResourcePrefixesRejectInvalidRanges(t *testing.T) {
	for _, resource := range []client.IPResource{ipRange("10.0.0.2", "10.0.0.1"), ipRange("bad", "10.0.0.1"), ipRange("::1", "::2")} {
		if _, err := resourcePrefixes([]client.IPResource{resource}); err == nil {
			t.Fatal("accepted invalid range")
		}
	}
}

func TestDNSNamespacesStayScoped(t *testing.T) {
	policy := &connectionPolicy{
		domains:    client.DomainResources{"corp.example": nil, "*.apps.example": nil, "*": nil, "": nil, "bad;command": nil},
		dnsRecords: map[string][]net.IP{"host.internal": {net.ParseIP("10.0.0.1")}},
	}
	got := dnsNamespaces(policy)
	want := []string{".apps.example", ".corp.example", "corp.example", "host.internal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("namespaces = %v", got)
	}
}
