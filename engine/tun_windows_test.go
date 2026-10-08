//go:build windows

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/miekg/dns"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/resolve"
	M "github.com/sagernet/sing/common/metadata"

	"github.com/mythologyli/zju-connect/stack"
)

func TestTUNVirtualAddressesStayInUpperHalfAndOutsideFakeIPPool(t *testing.T) {
	upper := netip.MustParsePrefix("198.19.128.0/17")
	clash := netip.MustParsePrefix("198.19.0.0/17")
	atrust := netip.MustParsePrefix("198.18.0.0/16")
	prefix := netip.MustParsePrefix(tunPrefix)
	dnsAddress := netip.MustParseAddr(tunDNS)
	if prefix.Bits() != 30 || !prefix.Contains(dnsAddress) {
		t.Fatal("TUN and DNS must share the dedicated /30")
	}
	for _, address := range []netip.Addr{prefix.Addr(), dnsAddress} {
		if !upper.Contains(address) || clash.Contains(address) || atrust.Contains(address) {
			t.Fatalf("virtual address overlaps another client: %s", address)
		}
	}
	resolver := resolve.NewResolver(&recordingVPNStack{}, "", "", 30, nil, nil, false)
	defer resolver.Close()
	pool := netip.MustParsePrefix("198.19.192.0/18")
	for i := 0; i < 16382; i++ {
		ip := resolver.IPPool.GenerateIP("host-"+strconv.Itoa(i)+".internal", nil)
		address, ok := netip.AddrFromSlice(ip.To4())
		if !ok || !upper.Contains(address) || !pool.Contains(address) || prefix.Contains(address) || clash.Contains(address) || atrust.Contains(address) {
			t.Fatalf("fake IP overlaps TUN, DNS or another client: %s", ip)
		}
	}
}

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

func TestTUNDNSSiblingDomainStillUsesServerIPAndPortPermissions(t *testing.T) {
	vpn := &recordingVPNStack{}
	const host = "one.console.res.zb-chinatower.com"
	ip := net.ParseIP("172.31.108.6")
	domains := client.DomainResources{"asc.res.zb-chinatower.com": {{PortMin: 1, PortMax: 65535, Protocol: "all"}}}
	resolver := resolve.NewResolver(vpn, "", "", 30, domains, map[string][]net.IP{host: {ip}}, false)
	defer resolver.Close()
	resource := ipRange("172.31.0.0", "172.31.255.255")
	resource.PortMin, resource.PortMax, resource.Protocol = 443, 443, "tcp"
	policy := &connectionPolicy{resolver: resolver, domains: domains, ips: []client.IPResource{resource}}
	policy.tunDialer = dial.NewDialer(vpn, resolver, policy.ips, true, "")
	backend := &fakeRoutes{present: map[string]bool{"172.31.0.0/16": true}}
	ingress := &tunIngress{ctx: context.Background(), policy: policy, routes: newRouteManager(backend), bindings: make(map[netip.Addr][]client.DomainResource)}
	request := new(dns.Msg)
	request.SetQuestion(host+".", dns.TypeA)
	data, err := request.Pack()
	if err != nil {
		t.Fatal(err)
	}
	data, err = ingress.answerDNS(data)
	if err != nil {
		t.Fatal(err)
	}
	response := new(dns.Msg)
	if err := response.Unpack(data); err != nil {
		t.Fatal(err)
	}
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 || !response.Answer[0].(*dns.A).A.Equal(ip) {
		t.Fatalf("unexpected DNS response: %v", response)
	}
	conn, err := ingress.dial(context.Background(), "tcp", M.ParseSocksaddr(ip.String()+":443"))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	for _, address := range []string{ip.String() + ":80", "172.32.108.6:443"} {
		if _, err := ingress.dial(context.Background(), "tcp", M.ParseSocksaddr(address)); !errors.Is(err, dial.ErrACLDenied) {
			t.Fatalf("unauthorized destination %s: %v", address, err)
		}
	}
	if vpn.calls != 1 || len(ingress.bindings) != 0 || len(backend.present) != 1 {
		t.Fatal("sibling DNS resolution changed server permissions or routing")
	}
}

func TestTUNDNSRefreshCannotRestoreClosedOrReplacedSession(t *testing.T) {
	policy := &connectionPolicy{domains: client.DomainResources{"old.internal.example": nil}}
	for _, closed := range []bool{false, true} {
		ingress := &tunIngress{ctx: context.Background(), policy: &connectionPolicy{}, closed: closed}
		// A nil backend would panic if the obsolete refresh changed DNS rules.
		if err := ingress.RefreshDNS(policy); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWindowsRouteSnapshotPreservesOwnershipAndPartialFailures(t *testing.T) {
	// Override all route cmdlets: this executes the production PowerShell
	// control flow without touching adapters or the machine's routing table.
	const mocks = `
$script:reads = 0
$script:present = @{
  '10.0.0.0/8' = [pscustomobject]@{ DestinationPrefix = '10.0.0.0/8'; NextHop = '0.0.0.0' }
  '192.0.2.0/24' = [pscustomobject]@{ DestinationPrefix = '192.0.2.0/24'; NextHop = '0.0.0.0' }
}
function Get-NetRoute {
  param($InterfaceIndex, $PolicyStore, $DestinationPrefix)
  if ($InterfaceIndex -ne 42 -or $PolicyStore -ne 'ActiveStore') { throw 'Wrong interface or store' }
  $script:reads++
  $script:present.Values
}
function New-NetRoute {
  param($InterfaceIndex, $DestinationPrefix, $NextHop, $RouteMetric, $PolicyStore)
  if ($InterfaceIndex -ne 42 -or $NextHop -ne '0.0.0.0' -or $RouteMetric -ne 0 -or $PolicyStore -ne 'ActiveStore') { throw 'Wrong route settings' }
  if ($script:present.ContainsKey($DestinationPrefix)) { throw 'Recreated an existing route' }
  if ($DestinationPrefix -eq '203.0.113.0/24') { throw 'simulated route failure' }
  $script:present[$DestinationPrefix] = [pscustomobject]@{ DestinationPrefix = $DestinationPrefix; NextHop = $NextHop }
}
function Remove-NetRoute {
  [CmdletBinding()]
  param([Parameter(ValueFromPipeline=$true)]$InputObject, $Confirm)
  process { $script:present.Remove($InputObject.DestinationPrefix) }
}
`
	const checks = `
if ($script:reads -ne 1) { throw 'Repeated route table queries' }
if (-not $script:present.ContainsKey('10.0.0.0/8')) { throw 'Removed a pre-existing route' }
`
	for _, test := range []struct {
		name, payload  string
		added, removed []string
		failure        string
	}{
		{"replace", `{"action":"routes","index":42,"add":["10.0.0.0/8","172.31.0.0/16"],"remove":["192.0.2.0/24"]}`, []string{"172.31.0.0/16"}, []string{"192.0.2.0/24"}, ""},
		{"partial failure", `{"action":"routes","index":42,"add":["172.31.0.0/16","203.0.113.0/24"],"remove":["192.0.2.0/24"]}`, []string{"172.31.0.0/16"}, nil, "simulated route failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", mocks+networkScript+checks)
			command.Stdin = strings.NewReader(test.payload)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("route script failed: %v: %s", err, output)
			}
			var result routeResult
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("invalid operation result: %v: %s", err, output)
			}
			if !slices.Equal(result.Added, test.added) || !slices.Equal(result.Removed, test.removed) || result.Error != test.failure {
				t.Fatalf("route result = %+v", result)
			}
		})
	}
}
