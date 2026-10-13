package engine

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/stack"
)

type pendingTUNStack struct {
	stack.Stack
	dial func() (net.Conn, error)
}

func (s *pendingTUNStack) DialTCP(context.Context, *net.TCPAddr) (net.Conn, error) {
	return s.dial()
}
func (s *pendingTUNStack) DialUDP(context.Context, *net.UDPAddr) (net.Conn, error) {
	return s.dial()
}

// A packet send can synchronously wait for relogin. Policy publication must
// finish before that send is released, even when its context cannot unblock it.
func TestTUNPolicyRefreshDoesNotWaitForDial(t *testing.T) {
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			vpn := &pendingTUNStack{dial: func() (net.Conn, error) {
				close(entered)
				<-release
				return left, nil
			}}
			resources := []client.IPResource{{IPMin: net.IP{192, 0, 2, 1}, IPMax: net.IP{192, 0, 2, 1}, PortMin: 1, PortMax: 65535, Protocol: "all"}}
			policy := &connectionPolicy{stack: vpn, resourceIPs: resources, resourceDomains: client.DomainResources{}}
			e, _, _ := recoveryTestEngine(t)
			e.updateTUN(policy, false)
			backend := e.tunSession.backend
			finished := make(chan error, 1)
			go func() {
				conn, err := backend.Dial(context.Background(), network, "192.0.2.1:443")
				if conn != nil {
					conn.Close()
				}
				finished <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("dial did not reach VPN stack")
			}
			updated := make(chan struct{})
			go func() {
				e.updateTUN(&connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}}, false)
				close(updated)
			}()
			blocked := false
			select {
			case <-updated:
			case <-time.After(time.Second):
				blocked = true
			}
			unblock()
			<-updated
			err := <-finished
			if blocked {
				t.Fatal("reconnect policy update waited for a dial that needs tunnel recovery")
			}
			if err == nil {
				t.Fatal("dial authorized by replaced policy was published")
			}
			if _, err := right.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				t.Fatalf("stale connection was not closed: %v", err)
			}
		})
	}
}

func TestTUNPolicyRefreshDiscardsPendingDNS(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeSOA} {
		t.Run(dns.TypeToString[qtype], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var enteredOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			vpn := &pendingTUNStack{dial: func() (net.Conn, error) {
				left, right := net.Pipe()
				go func() {
					defer right.Close()
					conn := &dns.Conn{Conn: right}
					request, err := conn.ReadMsg()
					if err != nil {
						return
					}
					enteredOnce.Do(func() { close(entered) })
					<-release
					answer := new(dns.Msg)
					answer.SetReply(request)
					if request.Question[0].Qtype == dns.TypeA {
						answer.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: net.IP{192, 0, 2, 10}}}
					}
					_ = conn.WriteMsg(answer)
				}()
				return left, nil
			}}
			domains := client.DomainResources{"app.internal": {{PortMin: 443, PortMax: 443, Protocol: "tcp"}}}
			resolver := resolve.NewResolver(vpn, "192.0.2.53", "", 30, domains, nil, true)
			defer resolver.Close()
			policy := &connectionPolicy{stack: vpn, resolver: resolver, dnsName: "192.0.2.53", useRemoteDNS: true, resourceIPs: []client.IPResource{}, resourceDomains: domains}
			e, _, _ := recoveryTestEngine(t)
			e.updateTUN(policy, false)
			backend := e.tunSession.backend
			request := new(dns.Msg)
			request.SetQuestion("app.internal.", qtype)
			data, err := request.Pack()
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				answer, routes, err := backend.DNS(context.Background(), data)
				if len(answer) != 0 || len(routes) != 0 {
					err = nil // Any published result from the old policy is a failure.
				}
				finished <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("DNS did not reach VPN stack")
			}
			updated := make(chan struct{})
			go func() {
				e.updateTUN(&connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}}, false)
				close(updated)
			}()
			blocked := false
			select {
			case <-updated:
			case <-time.After(time.Second):
				blocked = true
			}
			unblock()
			<-updated
			err = <-finished
			if blocked {
				t.Fatal("reconnect policy update waited for pending DNS")
			}
			if err == nil || len(backend.bindings) != 0 {
				t.Fatalf("stale DNS reply or ACL binding published: err=%v bindings=%v", err, backend.bindings)
			}
		})
	}
}
