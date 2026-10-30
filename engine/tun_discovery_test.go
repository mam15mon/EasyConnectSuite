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
			e.opts.LogFile = path
			e.startLogCapture()
			defer e.stopLogCapture()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			zones := &zoneDNSStack{zones: map[string]string{"app.corp.example.": "corp.example"}}
			vpn := &pendingTUNStack{dial: func() (net.Conn, error) {
				close(entered)
				<-release // Model an endpoint lock that ignores context cancellation.
				return zones.DialTCP(context.Background(), nil)
			}}
			// zoneDNSStack records the target; supply it in this fixture.
			vpn.dial = func() (net.Conn, error) {
				close(entered)
				<-release
				return zones.DialTCP(context.Background(), &net.TCPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53})
			}
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
			for _, marker := range []string{"stage=dns_namespace_discovery", "stage=validate_discovery", "outcome=discarded", "stage_elapsed="} {
				if !strings.Contains(string(data), marker) {
					t.Errorf("missing diagnostic %s", marker)
				}
			}
		})
	}
}
