package mobile

import (
	"EasyConnectSuite/vpncore/stack/tun"
	"testing"
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
