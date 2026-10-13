package engine

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/resolve"
)

type testAttachment struct {
	once     sync.Once
	done     chan struct{}
	updates  int
	failure  error
	closeErr error
}

func (a *testAttachment) Close(context.Context) error {
	a.once.Do(func() { close(a.done) })
	return a.closeErr
}
func (a *testAttachment) Update(context.Context, TUNSpec) error { a.updates++; return a.failure }
func (a *testAttachment) Done() <-chan struct{}                 { return a.done }

type testController struct {
	attachment *testAttachment
	entered    chan struct{}
	calls      int
	attachErr  error
}

func TestTUNWorkerLossAndUpdateFailureLeaveVPNRunning(t *testing.T) {
	for _, update := range []bool{false, true} {
		events := make(chan TUNStatus, 32)
		c := &testController{}
		e := &Engine{id: 22, opts: Options{ConnectionID: "profile", TUNController: c, OnTUNStatus: func(s TUNStatus) { events <- s }}}
		e.ctx, e.cancel = context.WithCancel(context.Background())
		p := &connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}}
		e.router.current.Store(p)
		e.running.Store(true)
		if err := e.SetTUN(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		if update {
			c.attachment.failure = errors.New("route conflict")
			e.updateTUN(p, false)
		} else {
			c.attachment.Close(context.Background())
		}
		deadline, cancel := context.WithTimeout(context.Background(), time.Second)
		found := false
		for !found {
			select {
			case s := <-events:
				found = s.State == "error"
			case <-deadline.Done():
				t.Fatal("TUN failure not reported")
			}
		}
		cancel()
		if !e.Running() || e.ctx.Err() != nil || e.router.current.Load() != p {
			t.Fatal("TUN failure stopped or changed VPN")
		}
		e.Stop()
	}
}

func (c *testController) Attach(ctx context.Context, s TUNSpec, b TUNBackend) (TUNAttachment, error) {
	c.calls++
	if c.attachErr != nil {
		return nil, c.attachErr
	}
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

func recoveryTestEngine(t *testing.T) (*Engine, *testController, chan TUNStatus) {
	t.Helper()
	events := make(chan TUNStatus, 64)
	c := &testController{}
	e := &Engine{id: 31, opts: Options{ConnectionID: "recovery", TUNController: c, OnTUNStatus: func(s TUNStatus) { events <- s }}}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.running.Store(true)
	e.router.current.Store(&connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}})
	if err := e.SetTUN(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	for len(events) > 0 {
		<-events
	}
	t.Cleanup(e.Stop)
	return e, c, events
}
func awaitTUNState(t *testing.T, events <-chan TUNStatus, state string) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case s := <-events:
			if s.State == state {
				return
			}
		case <-timer.C:
			t.Fatalf("TUN did not reach %s", state)
		}
	}
}
func TestTUNLossRecoversOnceAndRefreshesPolicy(t *testing.T) {
	e, c, events := recoveryTestEngine(t)
	first := c.attachment
	first.Close(context.Background())
	awaitTUNState(t, events, "on")
	e.tunSession.op.Lock()
	second := c.attachment
	if second == first || c.calls != 2 || e.tunSession.backend.ctx.Err() != nil {
		t.Fatal("TUN not rebuilt")
	}
	e.tunSession.op.Unlock()
	e.updateTUN(e.router.current.Load(), true)
	if second.updates != 1 {
		t.Fatal("recovered TUN missed resource update")
	}
	second.Close(context.Background())
	awaitTUNState(t, events, "error")
	e.tunSession.op.Lock()
	defer e.tunSession.op.Unlock()
	if c.calls != 2 || e.tunSession.attachment != nil || !e.Running() {
		t.Fatal("recovery loop or VPN shutdown")
	}
}
func TestTUNDisabledDoesNotRecoverOnPolicyRefresh(t *testing.T) {
	e, c, _ := recoveryTestEngine(t)
	if err := e.SetTUN(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	e.updateTUN(e.router.current.Load(), true)
	e.tunSession.op.Lock()
	defer e.tunSession.op.Unlock()
	if c.calls != 1 || e.tunSession.attachment != nil || e.tunSession.desired {
		t.Fatal("explicit disable lost")
	}
}
func TestTUNCleanupFailurePreventsRecovery(t *testing.T) {
	e, c, events := recoveryTestEngine(t)
	c.attachment.closeErr = errors.New("route removal failed")
	c.attachment.Close(context.Background())
	awaitTUNState(t, events, "error")
	e.tunSession.op.Lock()
	defer e.tunSession.op.Unlock()
	if c.calls != 1 || e.tunSession.attachment != nil {
		t.Fatal("cleanup failure concealed by recovery")
	}
}
func TestTUNRecoveryFailureDoesNotLoop(t *testing.T) {
	e, c, events := recoveryTestEngine(t)
	c.attachErr = errors.New("elevation canceled")
	c.attachment.Close(context.Background())
	awaitTUNState(t, events, "attaching")
	awaitTUNState(t, events, "error")
	e.updateTUN(e.router.current.Load(), true)
	e.tunSession.op.Lock()
	defer e.tunSession.op.Unlock()
	if c.calls != 2 || e.tunSession.attachment != nil {
		t.Fatal("failed recovery retried")
	}
}
func TestStopCancelsPendingTUNRecovery(t *testing.T) {
	e, c, _ := recoveryTestEngine(t)
	c.entered = make(chan struct{})
	c.attachment.Close(context.Background())
	select {
	case <-c.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("recovery did not start")
	}
	e.Stop()
	if e.Running() || e.tunSession.attachment != nil {
		t.Fatal("Stop retained recovering TUN")
	}
}

func TestTUNLaterWorkerLossCanRecoverAgain(t *testing.T) {
	e, c, events := recoveryTestEngine(t)
	c.attachment.Close(context.Background())
	awaitTUNState(t, events, "on")
	e.tunSession.op.Lock()
	second := c.attachment
	// Simulate a later suspend/resume after the replacement has stayed healthy.
	e.tunSession.recoveryAfter = time.Now().Add(-time.Second)
	e.tunSession.op.Unlock()
	second.Close(context.Background())
	awaitTUNState(t, events, "on")
	e.tunSession.op.Lock()
	defer e.tunSession.op.Unlock()
	if c.calls != 3 || c.attachment == second {
		t.Fatal("later worker loss remained permanently disabled")
	}
}

func TestTUNRecoveryReusesDNSNamespacesWithoutTunnelProbes(t *testing.T) {
	e, c, events := recoveryTestEngine(t)
	vpn := &recordingVPNStack{}
	p := e.router.current.Load()
	p.stack = vpn
	p.useRemoteDNS = true
	p.dnsName = "10.0.0.53"
	p.resourceDomains = client.DomainResources{"corp.internal": {{Protocol: "tcp", PortMin: 443, PortMax: 443}}}
	e.tunSession.spec.Names = []string{"corp.internal", ".corp.internal"}
	c.attachment.Close(context.Background())
	awaitTUNState(t, events, "on")
	e.tunSession.op.Lock()
	defer e.tunSession.op.Unlock()
	if vpn.calls != 0 {
		t.Fatal("automatic attachment probed a tunnel still being rebuilt")
	}
	names := e.tunSession.spec.Names
	if len(names) != 2 || names[0] != "corp.internal" || names[1] != ".corp.internal" {
		t.Fatalf("known DNS namespaces lost: %v", names)
	}
}
