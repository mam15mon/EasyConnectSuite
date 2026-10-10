package gvisor

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/internal/hook_func"
)

func TestReconnectWaitsForNetworkBeyondLoginLimit(t *testing.T) {
	for _, handshake := range []bool{false, true} {
		t.Run(fmt.Sprintf("handshake=%v", handshake), func(t *testing.T) {
			hook_func.ResetTerminalFuncs()
			t.Cleanup(hook_func.ResetTerminalFuncs)
			conn, peer := net.Pipe()
			t.Cleanup(func() { conn.Close(); peer.Close() })
			s := &Stack{endpoint: &Endpoint{}}
			attempts := 0
			offline := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}
			attempt := func() error {
				attempts++
				// Advance the existing cooldown timestamp without slowing the test.
				s.lastReloginTime = time.Now().Add(-reloginCooldown)
				if attempts <= maxReloginAttempts+1 {
					return fmt.Errorf("request failed: %w", offline)
				}
				return nil
			}
			if handshake {
				s.endpoint.client = networkReconnectClient{reconnectClient: reconnectClient{connection: conn}, attempt: attempt}
				s.SetupRelogin(func() error { return nil })
			} else {
				s.endpoint.client = reconnectClient{connection: conn}
				s.SetupRelogin(attempt)
			}
			s.SetupFatalHandler(func(err error) { t.Errorf("gave up while offline: %v", err) })
			if !s.rebuildL3Conn(nil, offline) {
				t.Fatal("did not recover when network returned")
			}
			if attempts != maxReloginAttempts+2 || s.endpoint.l3Conn != conn || s.reloginFailures != 0 {
				t.Fatalf("incomplete recovery: attempts=%d failures=%d", attempts, s.reloginFailures)
			}
		})
	}
}

type networkReconnectClient struct {
	reconnectClient
	attempt func() error
}

func (c networkReconnectClient) NewL3Conn() (interfaceConn io.ReadWriteCloser, err error) {
	if err := c.attempt(); err != nil {
		return nil, err
	}
	return c.connection, nil
}

func TestReconnectStillLimitsLoginFailures(t *testing.T) {
	hook_func.ResetTerminalFuncs()
	t.Cleanup(hook_func.ResetTerminalFuncs)
	s := &Stack{endpoint: &Endpoint{}}
	attempts := 0
	s.SetupRelogin(func() error {
		attempts++
		s.lastReloginTime = time.Now().Add(-reloginCooldown)
		return errors.New("invalid credentials")
	})
	fatal := false
	s.SetupFatalHandler(func(error) { fatal = true })
	if s.rebuildL3Conn(nil, errors.New("EOF")) || !fatal || attempts != maxReloginAttempts {
		t.Fatalf("login limit changed: fatal=%v attempts=%d", fatal, attempts)
	}
}
