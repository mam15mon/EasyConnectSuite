package engine

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync/atomic"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/stack"
	"inet.af/netaddr"
)

// A new immutable policy is published after each successful login. Listeners
// keep the router, not a stale resolver or a previous session's resource list.
type connectionPolicy struct {
	resolver     *resolve.Resolver
	stack        stack.Stack
	dialer       *dial.Dialer
	tunDialer    *dial.Dialer
	ips          []client.IPResource
	domains      client.DomainResources
	dnsRecords   map[string][]net.IP
	dnsName      string
	useRemoteDNS bool
	entries      []ResourceEntry
}

type policyRouter struct {
	current atomic.Pointer[connectionPolicy]
}

func (r *policyRouter) Resolve(ctx context.Context, host string) (context.Context, net.IP, error) {
	p := r.current.Load()
	if p == nil {
		return ctx, nil, fmt.Errorf("VPN policy unavailable")
	}
	return p.resolver.Resolve(ctx, host)
}

func (r *policyRouter) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	p := r.current.Load()
	if p == nil {
		return nil, fmt.Errorf("VPN policy unavailable")
	}
	return p.dialer.Dial(ctx, network, address)
}

func (r *policyRouter) DialIPPort(ctx context.Context, network, address string) (net.Conn, error) {
	p := r.current.Load()
	if p == nil {
		return nil, fmt.Errorf("VPN policy unavailable")
	}
	return p.dialer.DialIPPort(ctx, network, address)
}

func (r *policyRouter) RemoteUDPResolver() (*net.Resolver, error) {
	return r.current.Load().resolver.RemoteUDPResolver()
}
func (r *policyRouter) RemoteTCPResolver() (*net.Resolver, error) {
	return r.current.Load().resolver.RemoteTCPResolver()
}

func loadPolicy(conf configs.Config, c client.Client, vpnStack stack.Stack) (*connectionPolicy, error) {
	ips, ipErr := c.IPResources()
	domains, domainErr := c.DomainResources()
	records, _ := c.DNSResource()
	// Missing IP resources must not turn a TUN request into unrestricted VPN
	// routing. An empty non-nil list still permits authorized domain resources.
	if conf.TUNMode {
		if ipErr != nil && domainErr != nil {
			return nil, fmt.Errorf("VPN 未返回可用的资源列表，无法建立资源 TUN")
		}
		if ips == nil {
			ips = []client.IPResource{}
		}
	} else if !conf.DisableZJUConfig {
		ips = append([]client.IPResource{{IPMin: net.ParseIP("10.0.0.0"), IPMax: net.ParseIP("10.255.255.255"), PortMin: 1, PortMax: 65535, Protocol: "all"}}, ips...)
		if domains == nil {
			domains = make(client.DomainResources)
		}
		domains["zju.edu.cn"] = []client.DomainResource{{PortMin: 1, PortMax: 65535, Protocol: "all"}}
	}
	for _, domain := range conf.CustomProxyDomain {
		if domains == nil {
			domains = make(client.DomainResources)
		}
		domains[domain] = append(domains[domain], client.DomainResource{PortMin: 1, PortMax: 65535, Protocol: "all"})
	}
	useRemote := !conf.DisableRemoteDNS
	remote := conf.RemoteDNSServer
	if useRemote && remote == "auto" {
		var err error
		remote, err = c.DNSServer()
		if err != nil {
			useRemote = false
			remote = ""
		}
	}
	secondary := conf.SecondaryDNSServer
	if secondary == "auto" {
		secondary = "114.114.114.114"
		if servers, err := c.DNSServers(); err == nil && len(servers) > 1 {
			secondary = servers[1]
		}
	}
	resolver := resolve.NewResolver(vpnStack, remote, secondary, conf.DNSTTL, domains, records, useRemote)
	for _, record := range conf.CustomDNSList {
		if ip := net.ParseIP(record.IP); ip != nil {
			resolver.SetPermanentDNS(record.HostName, ip)
		}
	}
	p := &connectionPolicy{stack: vpnStack, resolver: resolver, ips: ips, domains: domains, dnsRecords: records, dnsName: remote, useRemoteDNS: useRemote}
	p.dialer = dial.NewDialer(vpnStack, resolver, ips, conf.ProxyAll, conf.DialDirectProxy)
	// A miss on a TUN route must fail rather than dial DIRECT back into the
	// same system route. SOCKS5 keeps its existing selective routing behavior.
	p.tunDialer = dial.NewDialer(vpnStack, resolver, ips, true, "")
	p.entries = buildResourceEntries(ips, domains, records)
	log.Printf("VPN resources: %d IP ranges, %d domains; DNS %s", len(ips), len(domains), remote)
	return p, nil
}

// Preserve server prefix lengths. Never manufacture narrower routes to defeat
// another VPN's more-specific route, or add a catch-all default route.
func resourcePrefixes(resources []client.IPResource) ([]string, error) {
	seen := make(map[string]bool)
	var prefixes []string
	for _, resource := range resources {
		start, ok := netaddr.FromStdIP(resource.IPMin)
		if !ok || !start.Is4() {
			return nil, fmt.Errorf("invalid resource start IP")
		}
		end, ok := netaddr.FromStdIP(resource.IPMax)
		if !ok || !end.Is4() || end.Less(start) {
			return nil, fmt.Errorf("invalid resource end IP")
		}
		for _, prefix := range netaddr.IPRangeFrom(start, end).Prefixes() {
			value := prefix.String()
			if !seen[value] {
				seen[value] = true
				prefixes = append(prefixes, value)
			}
		}
	}
	sort.Strings(prefixes)
	return prefixes, nil
}
