package engine

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"EasyConnectSuite/internal/tunipc"
	"github.com/miekg/dns"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/resolve"
)

type TUNSpec = tunipc.Spec

var errTUNPolicyChanged = errors.New("TUN policy changed during request")

type TUNBackend interface {
	Dial(context.Context, string, string) (net.Conn, error)
	DNS(context.Context, []byte) ([]byte, []string, error)
}
type TUNController interface {
	Attach(context.Context, TUNSpec, TUNBackend) (TUNAttachment, error)
}
type TUNAttachment interface {
	Update(context.Context, TUNSpec) error
	Close(context.Context) error
	Done() <-chan struct{}
}
type TUNStatus struct {
	State string
	Error string
}

// The backend owns authorization and the shared resolver; ingress owns only system resources.
type tunBackend struct {
	mu         sync.RWMutex
	bindingsMu sync.Mutex
	policy     *connectionPolicy
	bindings   map[netip.Addr][]client.DomainResource
	ctx        context.Context
}

func newTUNBackend(ctx context.Context, p *connectionPolicy) *tunBackend {
	return &tunBackend{ctx: ctx, policy: p, bindings: map[netip.Addr][]client.DomainResource{}}
}
func (b *tunBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	ap, e := netip.ParseAddrPort(address)
	if e != nil || !ap.Addr().Is4() || ap.Port() == 0 || (network != "tcp" && network != "udp") {
		return nil, errors.New("invalid TUN destination")
	}
	b.mu.RLock()
	if b.ctx.Err() != nil {
		b.mu.RUnlock()
		return nil, net.ErrClosed
	}
	policy := b.policy
	b.bindingsMu.Lock()
	rules := append([]client.DomainResource(nil), b.bindings[ap.Addr()]...)
	b.bindingsMu.Unlock()
	// Sending may synchronously enter relogin, which updates this policy.
	// Never keep the policy lock across a VPN network operation.
	b.mu.RUnlock()
	if len(rules) > 0 {
		ctx = context.WithValue(ctx, resolve.ContextKeyDomainResource, append([]client.DomainResource(nil), rules...))
	}
	ctx, c := context.WithTimeout(ctx, 15*time.Second)
	defer c()
	stop := context.AfterFunc(b.ctx, c)
	defer stop()
	conn, err := policy.tunDialer.DialIPPort(ctx, network, address)
	if err != nil {
		return nil, err
	}
	b.mu.RLock()
	changed := b.policy != policy
	b.mu.RUnlock()
	if ctx.Err() != nil || changed {
		_ = conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errTUNPolicyChanged
	}
	return conn, nil
}
func tunDomainRules(domains client.DomainResources, host string) []client.DomainResource {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	var result []client.DomainResource
	for domain, rules := range domains {
		domain = strings.TrimSuffix(strings.ToLower(domain), ".")
		domain = strings.TrimPrefix(domain, "*")
		if host == domain || strings.HasSuffix(host, "."+strings.TrimPrefix(domain, ".")) {
			result = append(result, rules...)
		}
	}
	return result
}
func (b *tunBackend) DNS(ctx context.Context, data []byte) ([]byte, []string, error) {
	if len(data) > 65535 {
		return nil, nil, errors.New("DNS request too large")
	}
	request := new(dns.Msg)
	if e := request.Unpack(data); e != nil {
		return nil, nil, e
	}
	response := new(dns.Msg)
	response.SetReply(request)
	response.RecursionAvailable = true
	pack := func() ([]byte, []string, error) { d, e := response.Pack(); return d, nil, e }
	if len(request.Question) != 1 {
		response.Rcode = dns.RcodeFormatError
		return pack()
	}
	b.mu.RLock()
	policy := b.policy
	b.mu.RUnlock()
	if b.ctx.Err() != nil {
		return nil, nil, net.ErrClosed
	}
	ctx, c := context.WithTimeout(ctx, 10*time.Second)
	defer c()
	stop := context.AfterFunc(b.ctx, c)
	defer stop()
	question := request.Question[0]
	if question.Qtype != dns.TypeA && question.Qtype != dns.TypeAAAA {
		if !policy.useRemoteDNS || net.ParseIP(policy.dnsName) == nil {
			response.Rcode = dns.RcodeNotImplemented
			return pack()
		}
		answer, e := exchangeVPNDNS(ctx, policy, request)
		b.mu.RLock()
		defer b.mu.RUnlock()
		if b.policy != policy {
			return nil, nil, errTUNPolicyChanged
		}
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if e != nil {
			response.Rcode = dns.RcodeServerFailure
			return pack()
		}
		d, e := answer.Pack()
		return d, nil, e
	}
	_, ip, e := policy.resolver.Resolve(ctx, strings.TrimSuffix(question.Name, "."))
	// Publish the answer and domain bindings only for the policy that
	// authorized this lookup; reconnect may have replaced it during I/O.
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.policy != policy {
		return nil, nil, errTUNPolicyChanged
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if e != nil {
		response.Rcode = dns.RcodeServerFailure
		return pack()
	}
	if ip.To4() == nil || question.Qtype == dns.TypeAAAA {
		return pack()
	}
	rules := tunDomainRules(policy.domains, question.Name)
	var routes []string
	if len(rules) > 0 {
		addr, _ := netip.AddrFromSlice(ip.To4())
		b.bindingsMu.Lock()
		existing := b.bindings[addr]
		seen := map[client.DomainResource]bool{}
		for _, r := range existing {
			seen[r] = true
		}
		for _, r := range rules {
			if !seen[r] {
				existing = append(existing, r)
				seen[r] = true
			}
		}
		b.bindings[addr] = existing
		b.bindingsMu.Unlock()
		if !ipHasResourceRoute(policy.ips, ip) {
			routes = []string{addr.String() + "/32"}
		}
	}
	response.Answer = append(response.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: ip.To4()})
	d, e := response.Pack()
	return d, routes, e
}
