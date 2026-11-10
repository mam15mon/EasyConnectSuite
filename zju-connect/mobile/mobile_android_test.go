package mobile

import (
	"EasyConnectSuite/vpncore/stack/tun"
	"testing"
	"time"
)

func TestOldTunnelCompletionKeepsNewTunnelRunning(t *testing.T) {
	old, current := &tun.Stack{}, &tun.Stack{}
	tunnelMu.Lock()
	activeTunnel = current
	tunnelRunning.Store(true)
	tunnelMu.Unlock()
	defer finishTunnel(current)
	finishTunnel(old)
	if !TunnelRunning() {
		t.Fatal("old tunnel completion cleared the new tunnel's running state")
	}
	tunnelMu.Lock()
	got := activeTunnel
	tunnelMu.Unlock()
	if got != current {
		t.Fatal("old tunnel completion changed the active tunnel")
	}
	finishTunnel(current)
	if TunnelRunning() {
		t.Fatal("active tunnel completion did not clear running state")
	}
}

func TestQueuedLoginDoesNotStartAfterLogout(t *testing.T) {
	loginMu.Lock()
	vpnClientMu.Lock()
	epoch := logoutGeneration
	vpnClientMu.Unlock()
	done := make(chan string, 1)
	go func() { done <- loginAtEpoch("127.0.0.1:1", "test", "test", "", epoch) }()
	Logout()
	loginMu.Unlock()
	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("cancelled login returned %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled queued login did not return")
	}
	vpnClientMu.Lock()
	client, underlay := vpnClient, vpnUnderlay
	vpnClientMu.Unlock()
	if client != nil || underlay != nil {
		t.Fatal("cancelled queued login installed a client")
	}
}
