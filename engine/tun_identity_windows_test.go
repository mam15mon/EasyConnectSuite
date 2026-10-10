//go:build windows

package engine

import (
	"net/netip"
	"testing"
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
