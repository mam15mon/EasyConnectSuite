package engine

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"EasyConnectSuite/vpncore/client"
)

func TestTUNDiscoveryDoesNotBlockLifecycle(t *testing.T) {
	for _, action := range []string{"refresh", "detach", "reattach", "stop", "cancel"} {
		t.Run(action, func(t *testing.T) {
			e, c, _ := recoveryTestEngine(t)
			path := filepath.Join(t.TempDir(), "engine.log")
			observer := &Engine{opts: Options{LogFile: path}}
			observer.startLogCapture()
			defer observer.stopLogCapture()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			zones := &zoneDNSStack{zones: map[string]string{"app.corp.example.": "corp.example"}}
			vpn := &pendingTUNStack{dial: func() (net.Conn, error) {
				close(entered)
				<-release // Model an endpoint lock that ignores context cancellation.
				return zones.DialTCP(context.Background(), &net.TCPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53})
			}}
			p := &connectionPolicy{stack: vpn, dnsName: "192.0.2.53", useRemoteDNS: true,
				resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{"app.corp.example": nil}}
			e.router.current.Store(p)
			e.updateTUN(p, false)
			first := c.attachment
			before := first.updates
			discovered := make(chan struct{})
			go func() { e.updateTUN(p, true); close(discovered) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("discovery did not start")
			}
			changed := make(chan struct{})
			next := &connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}}
			go func() {
				defer close(changed)
				switch action {
				case "refresh":
					e.updateTUN(next, false)
					e.router.current.Store(next)
				case "detach", "reattach":
					if err := e.SetTUN(context.Background(), false); err != nil {
						t.Error(err)
					}
					if action == "reattach" {
						e.router.current.Store(next)
						if err := e.SetTUN(context.Background(), true); err != nil {
							t.Error(err)
						}
					}
				case "stop":
					e.Stop()
				case "cancel":
					e.cancel()
				}
			}()
			blocked := false
			select {
			case <-changed:
			case <-time.After(time.Second):
				blocked = true
			}
			unblock()
			<-changed
			select {
			case <-discovered:
			case <-time.After(time.Second):
				t.Fatal("discovery did not finish")
			}
			if blocked {
				t.Fatal("lifecycle waited for DNS discovery")
			}
			want := before
			if action == "refresh" {
				want++
			}
			if first.updates != want {
				t.Fatalf("stale discovery published: updates=%d want=%d", first.updates, want)
			}
			if action == "reattach" && c.attachment.updates != 0 {
				t.Fatal("old discovery updated new attachment")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			outcome := "outcome=failed"
			if action == "refresh" {
				outcome = "outcome=discarded"
				e.updateTUN(p, true) // A delayed recovered callback must not reinstate p.
				if first.updates != want {
					t.Fatal("delayed callback replaced current policy")
				}
			}
			for _, marker := range []string{"stage=dns_namespace_discovery", "stage=validate_discovery", outcome, "stage_elapsed="} {
				if !strings.Contains(string(data), marker) {
					t.Errorf("missing diagnostic %s", marker)
				}
			}
		})
	}
}

func TestInitialTUNAttachDiscoversZonesOutsideLock(t *testing.T) {
	e, _, _ := recoveryTestEngine(t)
	if err := e.SetTUN(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	zones := &zoneDNSStack{zones: map[string]string{"app.corp.example.": "corp.example"}}
	vpn := &pendingTUNStack{dial: func() (net.Conn, error) {
		close(entered)
		<-release
		return zones.DialTCP(context.Background(), &net.TCPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53})
	}}
	p := &connectionPolicy{stack: vpn, dnsName: "192.0.2.53", useRemoteDNS: true,
		resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{"app.corp.example": nil}}
	e.router.current.Store(p)
	attached := make(chan error, 1)
	go func() { attached <- e.SetTUN(context.Background(), true) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("discovery did not start")
	}
	select {
	case err := <-attached:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("initial attach waited for DNS")
	}
	e.tunSession.op.Lock()
	if e.tunSession.spec.Policy != 1 || !containsNamespace(e.tunSession.spec.Names, "app.corp.example") {
		t.Error("initial resource names not installed before discovery")
	}
	e.tunSession.op.Unlock()
	unblock()
	deadline := time.Now().Add(time.Second)
	for {
		e.tunSession.op.Lock()
		complete := containsNamespace(e.tunSession.spec.Names, "corp.example") && e.tunSession.spec.Policy == 2
		e.tunSession.op.Unlock()
		if complete {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("discovered zone not published")
		}
		time.Sleep(time.Millisecond)
	}
}

func containsNamespace(names []string, name string) bool {
	for _, item := range names {
		if item == name {
			return true
		}
	}
	return false
}
