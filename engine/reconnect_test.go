package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"EasyConnectSuite/vpncore/lifecycle"
)

func TestTunnelFatalReturnsBeforeClosingNetworkStack(t *testing.T) {
	lifecycle.ResetTerminalFuncs()
	t.Cleanup(lifecycle.ResetTerminalFuncs)
	returned := make(chan struct{})
	release := make(chan struct{})
	statuses := make(chan Status, 2)
	e := &Engine{opts: Options{OnStatus: func(s Status) { statuses <- s }}}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.running.Store(true)
	closed := 0
	e.registerTerminal("CloseVPNStack", func(context.Context) error {
		// A network endpoint cannot be closed until its fatal callback returns.
		<-release
		closed++
		return nil
	})
	go func() {
		e.handleTunnelFatal(errors.New("network unreachable"))
		close(returned)
	}()

	blocked := false
	select {
	case <-returned:
	case <-time.After(time.Second):
		blocked = true
	}
	close(release)
	select {
	case status := <-statuses:
		if status.Phase != PhaseError || !strings.Contains(status.Message, "network unreachable") {
			t.Fatalf("unexpected terminal status: %+v", status)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not report failure")
	}
	<-returned
	e.Stop()
	if blocked {
		t.Fatal("fatal callback waited for the network stack it was running on")
	}
	if e.Running() || !e.fatal.Load() || closed != 1 {
		t.Fatalf("incomplete shutdown: running=%v fatal=%v closed=%d", e.Running(), e.fatal.Load(), closed)
	}
	select {
	case status := <-statuses:
		t.Fatalf("duplicate terminal status: %+v", status)
	default:
	}
}
