//go:build windows

package engine

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"EasyConnectSuite/internal/tunipc"
)

func TestTUNSlotsAreDistinctAndInsideReservedRange(t *testing.T) {
	seen := map[string]bool{}
	reserved := netip.MustParsePrefix("198.19.128.0/24")
	for i := 0; i < 64; i++ {
		name, prefix, dns, tag, err := tunIdentity(i)
		if err != nil {
			t.Fatal(err)
		}
		p := netip.MustParsePrefix(prefix)
		if seen[name] || seen[dns] || seen[tag] || !p.Contains(netip.MustParseAddr(dns)) || !reserved.Contains(p.Addr()) {
			t.Fatal("TUN identities overlap")
		}
		seen[name] = true
		seen[dns] = true
		seen[tag] = true
	}
	if _, _, _, _, err := tunIdentity(64); err == nil {
		t.Fatal("slot outside reserved range")
	}
}

type tunCleanupCheck struct{ check func() error }

func (check *tunCleanupCheck) Close() error { return check.check() }

func TestTUNCloseAllowsPendingHandlersBeforeReleasingClaims(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ingress := &tunIngress{ctx: ctx, cancel: cancel}
	tunPolicies.Lock()
	tunPolicies.claims[ingress] = tunClaim{prefixes: []string{"10.42.0.0/16"}}
	tunPolicies.Unlock()
	observed := make(chan bool, 1)
	ingress.connections.Store(&tunCleanupCheck{check: func() error {
		tunPolicies.Lock()
		_, reserved := tunPolicies.claims[ingress]
		ingress.mu.RLock()
		closed := ingress.closed
		ingress.mu.RUnlock()
		tunPolicies.Unlock()
		observed <- reserved && closed && ctx.Err() != nil
		return nil
	}}, true)
	done := make(chan error, 1)
	go func() { done <- ingress.Close() }()
	select {
	case err := <-done:
		if err != nil || !<-observed {
			t.Fatal("pending handler did not observe closed with claims reserved", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TUN teardown blocked pending handler on policy lock")
	}
	tunPolicies.Lock()
	_, reserved := tunPolicies.claims[ingress]
	tunPolicies.Unlock()
	if reserved {
		t.Fatal("completed teardown retained route claims")
	}
}
func TestTUNClaimsRejectSamePrefixAndDNSButAllowSpecificRoutes(t *testing.T) {
	tunPolicies.Lock()
	defer tunPolicies.Unlock()
	a, b := &tunIngress{}, &tunIngress{}
	tunPolicies.claims[a] = tunClaim{prefixes: []string{"10.0.0.0/8"}, names: []string{".internal.example"}}
	defer delete(tunPolicies.claims, a)
	if err := checkTUNConflicts(b, []string{"10.0.0.0/8"}, nil); err == nil {
		t.Fatal("identical resource claimed twice")
	}
	if err := checkTUNConflicts(b, []string{"10.42.0.0/16"}, nil); err != nil {
		t.Fatal("more-specific route rejected")
	}
	if err := checkTUNConflicts(b, nil, []string{"apps.internal.example"}); err == nil {
		t.Fatal("overlapping DNS rules accepted")
	}
	if err := checkTUNConflicts(a, []string{"10.0.0.0/8"}, []string{".internal.example"}); err != nil {
		t.Fatal("owner could not refresh")
	}
}

type policyFailureBackend struct {
	*fakeRoutes
	err error
}

func (b *policyFailureBackend) setDNS([]string) error { return b.err }

func TestTUNFailedPolicyUpdateRetainsOldAndNewClaims(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backend := &policyFailureBackend{fakeRoutes: &fakeRoutes{present: map[string]bool{}}}
	ingress := &tunIngress{ctx: ctx, cancel: cancel, backend: backend, routes: newRouteManager(backend)}
	defer func() { tunPolicies.Lock(); delete(tunPolicies.claims, ingress); tunPolicies.Unlock() }()
	old := TUNSpec{Ref: tunipc.Ref{Profile: "p", Session: "s", Attachment: "a", Policy: 1}, Prefixes: []string{"192.0.2.0/24"}, Names: []string{"old.invalid"}}
	if err := ingress.UpdateSpec(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic DNS failure")
	backend.err = failure
	next := old
	next.Policy++
	next.Prefixes = []string{"198.51.100.0/24"}
	next.Names = []string{"new.invalid"}
	if err := ingress.UpdateSpec(context.Background(), next); !errors.Is(err, failure) {
		t.Fatalf("update: %v", err)
	}
	if err := ingress.Close(); !errors.Is(err, failure) {
		t.Fatalf("cleanup: %v", err)
	}
	tunPolicies.Lock()
	defer tunPolicies.Unlock()
	for _, claim := range []TUNSpec{old, next} {
		if err := checkTUNConflicts(&tunIngress{}, claim.Prefixes, nil); err == nil {
			t.Fatalf("route claim lost: %v", claim.Prefixes)
		}
		if err := checkTUNConflicts(&tunIngress{}, nil, claim.Names); err == nil {
			t.Fatalf("DNS claim lost: %v", claim.Names)
		}
	}
}
