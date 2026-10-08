package engine

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/miekg/dns"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/stack"
)

type zoneDNSStack struct {
	stack.Stack
	mu      sync.Mutex
	queries []string
	targets []string
	zones   map[string]string
	err     error
}

func (s *zoneDNSStack) DialTCP(ctx context.Context, addr *net.TCPAddr) (net.Conn, error) {
	if s.err != nil {
		return nil, s.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a, b := net.Pipe()
	go func() {
		defer b.Close()
		conn := &dns.Conn{Conn: b}
		request, err := conn.ReadMsg()
		if err != nil {
			return
		}
		name := request.Question[0].Name
		s.mu.Lock()
		s.queries = append(s.queries, name)
		s.targets = append(s.targets, addr.String())
		s.mu.Unlock()
		answer := new(dns.Msg)
		answer.SetReply(request)
		if zone := s.zones[name]; zone != "" && request.Question[0].Qtype == dns.TypeSOA {
			answer.Ns = []dns.RR{zoneSOA(zone)}
		}
		_ = conn.WriteMsg(answer)
	}()
	return a, nil
}

func zoneSOA(zone string) *dns.SOA {
	return &dns.SOA{
		Hdr: dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeSOA, Class: dns.ClassINET},
		Ns:  "ns1.internal.example.", Mbox: "hostmaster.internal.example.",
	}
}

func TestDNSZoneDiscoveryIncludesAllResourceNamesAndSiblingConsole(t *testing.T) {
	const asc = "asc.res.zb-chinatower.com"
	const ops = "cn-beijing-bjtt-d01.ops.console.ops.zb-chinatower.com"
	const fixed = "fixed.res.zb-chinatower.com"
	vpn := &zoneDNSStack{zones: map[string]string{
		asc + ".": "res.zb-chinatower.com", ops + ".": "ops.zb-chinatower.com", fixed + ".": "res.zb-chinatower.com",
	}}
	policy := &connectionPolicy{
		stack: vpn, dnsName: "192.0.2.53", useRemoteDNS: true,
		domains:    client.DomainResources{asc: nil, ops: nil},
		dnsRecords: map[string][]net.IP{asc: {net.ParseIP("172.31.108.6")}, fixed: {net.ParseIP("172.31.108.7")}},
	}
	got := discoverDNSNamespaces(context.Background(), policy)
	want := []string{
		"." + asc, "." + ops, ".ops.zb-chinatower.com", ".res.zb-chinatower.com",
		asc, ops, fixed, "ops.zb-chinatower.com", "res.zb-chinatower.com",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("namespaces = %v, want %v", got, want)
	}
	vpn.mu.Lock()
	defer vpn.mu.Unlock()
	sort.Strings(vpn.queries)
	queries := []string{asc + ".", ops + ".", fixed + "."}
	sort.Strings(queries)
	if !reflect.DeepEqual(vpn.queries, queries) {
		t.Fatalf("duplicate or missing resource DNS queries: %v", vpn.queries)
	}
	for _, target := range vpn.targets {
		if target != "192.0.2.53:53" {
			t.Fatalf("zone discovery used another VPN's DNS: %s", target)
		}
	}
	if len(policy.domains) != 2 || len(policy.ips) != 0 {
		t.Fatal("zone discovery changed resource permissions")
	}
}

func TestDNSZoneSelectionRejectsGlobalAndUnrelatedNamespaces(t *testing.T) {
	for _, test := range []struct {
		name, host, zone string
		code             int
		valid            bool
	}{
		{"authority", "app.corp.example", "corp.example", dns.RcodeSuccess, true},
		{"apex", "corp.example", "corp.example", dns.RcodeSuccess, true},
		{"negative authority", "missing.corp.example", "corp.example", dns.RcodeNameError, true},
		{"case", "app.corp.example", "CORP.EXAMPLE", dns.RcodeSuccess, true},
		{"root", "app.corp.example", ".", dns.RcodeSuccess, false},
		{"top level", "app.corp.com", "com", dns.RcodeSuccess, false},
		{"public suffix", "app.corp.co.uk", "co.uk", dns.RcodeSuccess, false},
		{"private suffix", "app.corp.github.io", "github.io", dns.RcodeSuccess, false},
		{"other VPN", "app.corp.example", "atrust.example", dns.RcodeSuccess, false},
		{"partial label", "notcorp.example", "corp.example", dns.RcodeSuccess, false},
		{"invalid", "app.corp.example", "bad;command", dns.RcodeSuccess, false},
		{"failure", "app.corp.example", "corp.example", dns.RcodeServerFailure, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, inAnswer := range []bool{false, true} {
				answer := &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: test.code}}
				if inAnswer {
					answer.Answer = []dns.RR{zoneSOA(test.zone)}
				} else {
					answer.Ns = []dns.RR{zoneSOA(test.zone)}
				}
				if got := dnsZones(test.host, answer); (len(got) > 0) != test.valid {
					t.Fatalf("accepted zones = %v", got)
				}
			}
		})
	}
}

func TestDNSDiscoveryDoesNotInventZonesWithoutSOA(t *testing.T) {
	for _, discoveryErr := range []error{nil, errors.New("VPN DNS unavailable"), context.Canceled} {
		policy := &connectionPolicy{
			stack: &zoneDNSStack{err: discoveryErr}, dnsName: "192.0.2.53", useRemoteDNS: true,
			domains: client.DomainResources{"app.corp.example": nil},
		}
		if got, want := discoverDNSNamespaces(context.Background(), policy), dnsNamespaces(policy); !reflect.DeepEqual(got, want) {
			t.Fatalf("expanded namespaces without a zone: %v", got)
		}
	}
}
