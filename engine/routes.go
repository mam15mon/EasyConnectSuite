package engine

import (
	"bytes"
	"errors"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"github.com/mythologyli/zju-connect/client"
)

type routeResult struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Error   string   `json:"error"`
	Pending bool     `json:"pending"`
}

type routeBackend interface {
	Apply(add, remove []string) (routeResult, error)
}

// owned includes only routes actually created by us, never a pre-existing
// route or an entry on aTrust, Clash, or a physical interface.
type routeManager struct {
	mu      sync.Mutex
	backend routeBackend
	owned   map[string]bool
	closed  bool
}

func newRouteManager(backend routeBackend) *routeManager {
	return &routeManager{backend: backend, owned: make(map[string]bool)}
}

func (m *routeManager) replace(prefixes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("TUN routes closed")
	}
	desired := make(map[string]bool)
	for _, prefix := range prefixes {
		desired[prefix] = true
	}
	var add, remove []string
	for prefix := range desired {
		if !m.owned[prefix] {
			add = append(add, prefix)
		}
	}
	for prefix := range m.owned {
		if !desired[prefix] {
			remove = append(remove, prefix)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return m.apply(add, remove)
}

func (m *routeManager) add(prefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("TUN routes closed")
	}
	if m.owned[prefix] {
		return nil
	}
	return m.apply([]string{prefix}, nil)
}

func (m *routeManager) apply(add, remove []string) error {
	result, err := m.backend.Apply(add, remove)
	for _, prefix := range result.Added {
		m.owned[prefix] = true
	}
	for _, prefix := range result.Removed {
		delete(m.owned, prefix)
	}
	if result.Error != "" {
		err = errors.Join(err, errors.New(result.Error))
	}
	return err
}

func (m *routeManager) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	var remove []string
	for prefix := range m.owned {
		remove = append(remove, prefix)
	}
	sort.Strings(remove)
	return m.apply(nil, remove)
}

func dnsNamespaces(policy *connectionPolicy) []string {
	set := make(map[string]bool)
	for domain := range policy.domains {
		domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
		wildcard := strings.HasPrefix(domain, "*.") || strings.HasPrefix(domain, ".")
		domain = strings.TrimPrefix(strings.TrimPrefix(domain, "*"), ".")
		if !validDomain(domain) {
			continue
		}
		set["."+domain] = true
		if !wildcard {
			set[domain] = true
		}
	}
	for host := range policy.dnsRecords {
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		if validDomain(host) {
			set[host] = true
		}
	}
	var result []string
	for domain := range set {
		result = append(result, domain)
	}
	sort.Strings(result)
	return result
}

func validDomain(domain string) bool {
	if domain == "" || len(domain) > 253 {
		return false
	}
	if _, err := netip.ParseAddr(domain); err == nil {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, char := range label {
			if char != '-' && char != '_' && !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') {
				return false
			}
		}
	}
	return true
}

// DNS discovery must not add a host route over a server-provided IP route:
// doing so would defeat the user's longest-prefix policy for other VPNs.
func ipHasResourceRoute(resources []client.IPResource, ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	for _, resource := range resources {
		start, end := resource.IPMin.To4(), resource.IPMax.To4()
		if start != nil && end != nil && bytes.Compare(ip4, start) >= 0 && bytes.Compare(ip4, end) <= 0 {
			return true
		}
	}
	return false
}
