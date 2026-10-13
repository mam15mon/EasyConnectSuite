package gvisor

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"EasyConnectSuite/vpncore/client"
	"EasyConnectSuite/vpncore/internal/hook_func"
)

func TestFatalCallbackCanAcquireReconnectLock(t *testing.T) {
	hook_func.ResetTerminalFuncs()
	s := &Stack{endpoint: &Endpoint{}, reloginFailures: maxReloginAttempts, reloginFunc: func() error { return errors.New("login failed") }}
	called := make(chan struct{})
	s.SetupFatalHandler(func(error) {
		// Engine teardown/status callbacks must run after releasing this lock.
		s.SetupRelogin(nil)
		close(called)
	})
	done := make(chan bool, 1)
	go func() { done <- s.rebuildL3Conn(nil, errors.New("EOF")) }()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("unexpected recovery")
		}
	case <-time.After(time.Second):
		t.Fatal("fatal callback deadlocked")
	}
	select {
	case <-called:
	default:
		t.Fatal("fatal callback missing")
	}
}

type reconnectClient struct { client.Client; connection io.ReadWriteCloser }
func (c reconnectClient) NewL3Conn() (io.ReadWriteCloser, error) { return c.connection, nil }

func TestRecoveredCallbackRunsAfterTunnelInstallation(t *testing.T) {
	hook_func.ResetTerminalFuncs()
	connection, peer := net.Pipe()
	defer connection.Close(); defer peer.Close()
	s := &Stack{endpoint: &Endpoint{client: reconnectClient{connection: connection}}}
	s.SetupRelogin(func() error { return nil })
	called := make(chan struct{})
	s.SetupRecovered(func() {
		if s.endpoint.l3Conn != connection { t.Error("reported recovery before installing the tunnel") }
		s.SetupRecovered(nil)
		close(called)
	})
	done := make(chan bool, 1)
	go func() { done <- s.rebuildL3Conn(nil, errors.New("EOF")) }()
	select {
	case ok := <-done: if !ok { t.Fatal("recovery failed") }
	case <-time.After(time.Second): t.Fatal("recovery callback deadlocked")
	}
	select { case <-called: default: t.Fatal("recovery callback missing") }
}
