//go:build windows

package engine

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/resolve"
	M "github.com/sagernet/sing/common/metadata"

	"github.com/mythologyli/zju-connect/stack"
)

// Stub stack avoids login, sockets, adapters, routes, and DNS policy changes.
type recordingVPNStack struct {
	stack.Stack
	calls int
}

func (s *recordingVPNStack) DialTCP(context.Context, *net.TCPAddr) (net.Conn, error) {
	s.calls++
	a, b := net.Pipe()
	b.Close()
	return a, nil
}

func TestTUNDNSRoutesAuthorizedDomainAndChecksPort(t *testing.T) {
	vpn := &recordingVPNStack{}
	resources := []client.DomainResource{{PortMin: 443, PortMax: 443, Protocol: "tcp"}}
	domains := client.DomainResources{"apps.internal": resources}
	resolver := resolve.NewResolver(vpn, "", "", 30, domains, map[string][]net.IP{"service.apps.internal": {net.ParseIP("10.42.0.8")}}, false)
	defer resolver.Close()
	policy := &connectionPolicy{resolver: resolver, domains: domains}
	policy.tunDialer = dial.NewDialer(vpn, resolver, []client.IPResource{}, true, "")
	backend := &fakeRoutes{present: map[string]bool{}}
	ingress := &tunIngress{ctx: context.Background(), policy: policy, routes: newRouteManager(backend), bindings: make(map[netip.Addr][]client.DomainResource)}
	request := new(dns.Msg)
	request.SetQuestion("service.apps.internal.", dns.TypeA)
	data, _ := request.Pack()
	for i := 0; i < 3; i++ {
		responseData, err := ingress.answerDNS(data)
		if err != nil {
			t.Fatal(err)
		}
		response := new(dns.Msg)
		if err := response.Unpack(responseData); err != nil {
			t.Fatal(err)
		}
		if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
			t.Fatalf("response = %v", response)
		}
	}
	if !backend.present["10.42.0.8/32"] {
		t.Fatal("missing domain IP route")
	}
	if len(ingress.bindings[netip.MustParseAddr("10.42.0.8")]) != 1 {
		t.Fatal("duplicate resource bindings")
	}
	conn, err := ingress.dial(context.Background(), "tcp", M.ParseSocksaddr("10.42.0.8:443"))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	_, err = ingress.dial(context.Background(), "tcp", M.ParseSocksaddr("10.42.0.8:80"))
	if !errors.Is(err, dial.ErrACLDenied) {
		t.Fatalf("unauthorized port error = %v", err)
	}
	if vpn.calls != 1 {
		t.Fatalf("VPN got %d connections, want 1", vpn.calls)
	}
}

func TestTUNIPResourceMissCannotLoopThroughDirect(t *testing.T) {
	vpn := &recordingVPNStack{}
	resolver := resolve.NewResolver(vpn, "", "", 30, nil, nil, false)
	defer resolver.Close()
	policy := &connectionPolicy{resolver: resolver}
	policy.tunDialer = dial.NewDialer(vpn, resolver, []client.IPResource{ipRange("10.0.0.1", "10.0.0.1")}, true, "")
	ingress := &tunIngress{policy: policy, bindings: map[netip.Addr][]client.DomainResource{}}
	_, err := ingress.dial(context.Background(), "tcp", M.ParseSocksaddr("10.0.0.2:443"))
	if !errors.Is(err, dial.ErrACLDenied) {
		t.Fatalf("unexpected error: %v", err)
	}
	if vpn.calls != 0 {
		t.Fatal("sent an unauthorized destination through VPN")
	}
}

func TestDNSDoesNotOverrideMoreSpecificSystemRoutes(t *testing.T) {
	vpn := &recordingVPNStack{}
	domains := client.DomainResources{"corp.internal": {{PortMin: 443, PortMax: 443, Protocol: "tcp"}}}
	resolver := resolve.NewResolver(vpn, "", "", 30, domains, map[string][]net.IP{"app.corp.internal": {net.ParseIP("10.42.0.8")}}, false)
	defer resolver.Close()
	policy := &connectionPolicy{resolver: resolver, domains: domains, ips: []client.IPResource{ipRange("10.0.0.0", "10.255.255.255")}}
	backend := &fakeRoutes{present: map[string]bool{"10.0.0.0/8": true}}
	ingress := &tunIngress{ctx: context.Background(), policy: policy, routes: newRouteManager(backend), bindings: make(map[netip.Addr][]client.DomainResource)}
	request := new(dns.Msg)
	request.SetQuestion("app.corp.internal.", dns.TypeA)
	data, _ := request.Pack()
	if _, err := ingress.answerDNS(data); err != nil {
		t.Fatal(err)
	}
	if backend.present["10.42.0.8/32"] {
		t.Fatal("DNS added a /32 over the server's /8 and would defeat longest-prefix routing")
	}
}
