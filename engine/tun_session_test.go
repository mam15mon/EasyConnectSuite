package engine

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/resolve"
)

type testAttachment struct {
	once    sync.Once
	done    chan struct{}
	updates int
}

func (a *testAttachment) Close(context.Context) error {
	a.once.Do(func() { close(a.done) })
	return nil
}
func (a *testAttachment) Update(context.Context, TUNSpec) error { a.updates++; return nil }
func (a *testAttachment) Done() <-chan struct{}                 { return a.done }

type testController struct {
	attachment *testAttachment
	entered    chan struct{}
}

func (c *testController) Attach(ctx context.Context, s TUNSpec, b TUNBackend) (TUNAttachment, error) {
	if c.entered != nil {
		close(c.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c.attachment = &testAttachment{done: make(chan struct{})}
	return c.attachment, nil
}

func TestTUNHotDetachKeepsResolverAndProxySession(t *testing.T) {
	vpn := &recordingVPNStack{}
	resolver := resolve.NewResolver(vpn, "", "", 30, nil, map[string][]net.IP{"stable.internal": {net.ParseIP("10.1.2.3")}}, false)
	defer resolver.Close()
	policy := &connectionPolicy{resolver: resolver, stack: vpn, resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}}
	c := &testController{}
	e := &Engine{id: 1, opts: Options{ConnectionID: "profile", TUNController: c}}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.router.current.Store(policy)
	e.running.Store(true)
	closed := 0
	e.registerTerminal("proxy listener", func(context.Context) error { closed++; return nil })
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	for i := 0; i < 3; i++ {
		if err := e.SetTUN(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		if e.TUNState().State != "on" {
			t.Fatal("not attached")
		}
		if err := e.SetTUN(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if e.router.current.Load() != policy || policy.resolver != resolver || closed != 0 || !e.Running() {
			t.Fatal("hot toggle replaced or stopped VPN")
		}
		go left.Write([]byte{byte(i)})
		var b [1]byte
		if _, err := right.Read(b[:]); err != nil || b[0] != byte(i) {
			t.Fatal("proxy stream interrupted", err)
		}
		if _, ip, err := resolver.Resolve(context.Background(), "stable.internal"); err != nil || !ip.Equal(net.ParseIP("10.1.2.3")) {
			t.Fatal("resolver closed", err)
		}
	}
	e.Stop()
	if closed != 1 {
		t.Fatal("VPN cleanup missing")
	}
}
func TestStopCancelsPendingTUNAttach(t *testing.T) {
	c := &testController{entered: make(chan struct{})}
	e := &Engine{id: 2, opts: Options{ConnectionID: "profile", TUNController: c}}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.router.current.Store(&connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}})
	e.running.Store(true)
	done := make(chan error, 1)
	go func() { done <- e.SetTUN(context.Background(), true) }()
	<-c.entered
	e.Stop()
	if !errors.Is(<-done, context.Canceled) {
		t.Fatal("attach not canceled by Stop")
	}
	if e.Running() {
		t.Fatal("still running")
	}
}

type resourceTestClient struct {
	client.Client
	domains client.DomainResources
}

func (c *resourceTestClient) IPResources() ([]client.IPResource, error) {
	return []client.IPResource{ipRange("172.16.0.0", "172.16.255.255")}, nil
}
func (c *resourceTestClient) DomainResources() (client.DomainResources, error) { return c.domains, nil }
func (c *resourceTestClient) DNSResource() (map[string][]net.IP, error) {
	return map[string][]net.IP{}, nil
}
func TestTUNViewExcludesProxyAugmentationAndSharesResolver(t *testing.T) {
	original := client.DomainResources{"corp.internal": {{PortMin: 443, PortMax: 443, Protocol: "tcp"}}}
	c := &resourceTestClient{domains: original}
	p, err := loadPolicy(configs.Config{DisableRemoteDNS: true}, c, &recordingVPNStack{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.resolver.Close()
	v, err := p.tunView()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ips) != 2 || len(v.ips) != 1 || len(p.domains) != 2 || len(v.domains) != 1 || len(original) != 1 || v.resolver != p.resolver || v.stack != p.stack {
		t.Fatal("TUN view replaced proxy policy or leaked ZJU resources")
	}
	if _, err = v.tunDialer.DialIPPort(context.Background(), "tcp", "10.0.0.1:443"); err == nil {
		t.Fatal("proxy-only resource authorized by TUN")
	}
}
