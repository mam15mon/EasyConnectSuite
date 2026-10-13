package engine

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"EasyConnectSuite/vpncore/log"
	"github.com/miekg/dns"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"
)

// Ask only this VPN's DNS about its resource names. SOA identifies the zone
// containing a resource, including sibling names absent from the resource list.
func discoverDNSNamespaces(ctx context.Context, policy *connectionPolicy) []string {
	names := dnsNamespaces(policy)
	if !policy.useRemoteDNS || net.ParseIP(policy.dnsName) == nil || len(names) == 0 {
		return names
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	seeds := make(map[string]bool)
	set := make(map[string]bool)
	for _, name := range names {
		set[name] = true
		seeds[strings.TrimPrefix(name, ".")] = true
	}
	var mu sync.Mutex
	var queries errgroup.Group
	queries.SetLimit(4)
	for host := range seeds {
		queries.Go(func() error {
			request := new(dns.Msg)
			request.SetQuestion(dns.Fqdn(host), dns.TypeSOA)
			answer, err := exchangeVPNDNS(ctx, policy, request)
			if err != nil {
				log.Printf("TUN DNS zone lookup failed for %s: %v", host, err)
				return nil
			}
			zones := dnsZones(host, answer)
			mu.Lock()
			defer mu.Unlock()
			for _, zone := range zones {
				set[zone] = true
				set["."+zone] = true
			}
			return nil
		})
	}
	_ = queries.Wait()
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func exchangeVPNDNS(ctx context.Context, policy *connectionPolicy, request *dns.Msg) (*dns.Msg, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	upstream, err := policy.stack.DialTCP(ctx, &net.TCPAddr{IP: net.ParseIP(policy.dnsName), Port: 53})
	if err != nil {
		return nil, err
	}
	defer upstream.Close()
	answer, _, err := (&dns.Client{Net: "tcp"}).ExchangeWithConnContext(ctx, request, &dns.Conn{Conn: upstream})
	if err != nil {
		return nil, err
	}
	if answer.Rcode != dns.RcodeSuccess && answer.Rcode != dns.RcodeNameError {
		return nil, fmt.Errorf("VPN DNS returned %s", dns.RcodeToString[answer.Rcode])
	}
	return answer, nil
}

func dnsZones(host string, answer *dns.Msg) []string {
	if answer.Rcode != dns.RcodeSuccess && answer.Rcode != dns.RcodeNameError {
		return nil
	}
	var zones []string
	for _, section := range [][]dns.RR{answer.Answer, answer.Ns} {
		for _, record := range section {
			soa, ok := record.(*dns.SOA)
			if !ok || soa.Hdr.Class != dns.ClassINET {
				continue
			}
			zone := strings.TrimSuffix(strings.ToLower(soa.Hdr.Name), ".")
			if !validDomain(zone) || !strings.Contains(zone, ".") || (host != zone && !strings.HasSuffix(host, "."+zone)) {
				continue
			}
			suffix, _ := publicsuffix.PublicSuffix(zone)
			if zone == suffix {
				continue
			}
			zones = append(zones, zone)
		}
	}
	return zones
}
