package engine

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// The resolved API applies settings to a single interface index. Callers must
// keep that interface alive and verify its identity before applying or reverting.
type resolvedAddress struct {
	Family  int32
	Address []byte
}
type resolvedDomain struct {
	Name      string
	RouteOnly bool
}
type resolvedCall func(context.Context, string, ...any) error

func resolvedDNSArguments(server string, names []string) ([]resolvedAddress, []resolvedDomain, error) {
	ip, err := netip.ParseAddr(server)
	if err != nil || !ip.Is4() {
		return nil, nil, fmt.Errorf("invalid TUN DNS server")
	}
	seen := make(map[string]bool)
	domains := make([]resolvedDomain, 0, len(names))
	for _, raw := range names {
		name := strings.TrimPrefix(strings.ToLower(raw), ".")
		if !validDomain(name) {
			return nil, nil, fmt.Errorf("invalid TUN DNS namespace")
		}
		if !seen[name] {
			seen[name] = true
			domains = append(domains, resolvedDomain{Name: name, RouteOnly: true})
		}
	}
	sort.Slice(domains, func(i, j int) bool { return domains[i].Name < domains[j].Name })
	return []resolvedAddress{{Family: unix.AF_INET, Address: ip.AsSlice()}}, domains, nil
}

// resolvedLinkDNS encapsulates D-Bus encoding. Its caller owns rollback and
// must not call it for an interface whose identity is no longer confirmed.
func resolvedLinkDNS(ctx context.Context, call resolvedCall, index int, server string, names []string) error {
	if index <= 0 {
		return fmt.Errorf("invalid TUN interface index")
	}
	addresses, domains, err := resolvedDNSArguments(server, names)
	if err != nil {
		return err
	}
	if err = call(ctx, "SetLinkDNS", int32(index), addresses); err != nil {
		return fmt.Errorf("set link DNS: %w", err)
	}
	if err = call(ctx, "SetLinkDomains", int32(index), domains); err != nil {
		return fmt.Errorf("set link domains: %w", err)
	}
	return nil
}

func resolvedSystemCall(ctx context.Context, method string, args ...any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	obj := conn.Object("org.freedesktop.resolve1", dbus.ObjectPath("/org/freedesktop/resolve1"))
	return obj.CallWithContext(ctx, "org.freedesktop.resolve1.Manager."+method, dbus.FlagNoAutoStart, args...).Err
}
