package gvisor

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/internal/hook_func"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
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

func (c networkReconnectClient) NewL3Conn() (io.ReadWriteCloser, error) {
	if err := c.attempt(); err != nil {
		return nil, err
	}
	return c.connection, nil
}

func TestReconnectNetworkErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"unreachable", fmt.Errorf("request: %w", &net.OpError{Op: "dial", Err: syscall.EHOSTUNREACH}), true},
		{"timeout", fmt.Errorf("request: %w", os.ErrDeadlineExceeded), true},
		{"authentication", errors.New("invalid credentials"), false},
		{"protocol", errors.New("invalid L3 handshake"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isReconnectNetworkError(test.err); got != test.want {
				t.Fatalf("network error = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCloseStopsOfflineReconnectDuringCooldown(t *testing.T) {
	hook_func.ResetTerminalFuncs()
	t.Cleanup(hook_func.ResetTerminalFuncs)
	s := &Stack{endpoint: &Endpoint{}, gvisorStack: stack.New(stack.Options{}), done: make(chan struct{})}
	attempted := make(chan struct{})
	s.SetupRelogin(func() error {
		close(attempted)
		return &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}
	})
	s.SetupFatalHandler(func(err error) { t.Errorf("manual close reported fatal error: %v", err) })
	finished := make(chan bool, 1)
	go func() { finished <- s.rebuildL3Conn(nil, errors.New("EOF")) }()
	<-attempted
	s.Close()
	// A new engine resets the global terminal flag; the old stack must stay stopped.
	hook_func.ResetTerminalFuncs()
	select {
	case recovered := <-finished:
		if recovered {
			t.Fatal("closed stack reported recovery")
		}
	case <-time.After(time.Second):
		t.Fatal("manual close did not interrupt the reconnect cooldown")
	}
}

func TestReconnectStillLimitsOtherFailures(t *testing.T) {
	hook_func.ResetTerminalFuncs()
	t.Cleanup(hook_func.ResetTerminalFuncs)
	s := &Stack{endpoint: &Endpoint{}}
	attempts := 0
	s.SetupRelogin(func() error {
		attempts++
		s.lastReloginTime = time.Now().Add(-reloginCooldown)
		return errors.New("invalid L3 handshake")
	})
	fatal := false
	s.SetupFatalHandler(func(error) { fatal = true })
	if s.rebuildL3Conn(nil, errors.New("EOF")) || !fatal || attempts != maxReloginAttempts {
		t.Fatalf("login limit changed: fatal=%v attempts=%d", fatal, attempts)
	}
}

func TestReconnectStopsAfterOneRejectedLogin(t *testing.T) {
	hook_func.ResetTerminalFuncs()
	t.Cleanup(hook_func.ResetTerminalFuncs)
	s := &Stack{endpoint: &Endpoint{}}
	attempts := 0
	rejected := fmt.Errorf("server reply: %w", easyconnect.ErrLoginRejected)
	s.SetupRelogin(func() error {
		attempts++
		s.lastReloginTime = time.Now().Add(-reloginCooldown)
		return rejected
	})
	var fatal error
	s.SetupFatalHandler(func(err error) { fatal = err })
	if s.rebuildL3Conn(nil, errors.New("EOF")) || attempts != 1 || !errors.Is(fatal, easyconnect.ErrLoginRejected) {
		t.Fatalf("rejected credentials were retried: attempts=%d fatal=%v", attempts, fatal)
	}
}
