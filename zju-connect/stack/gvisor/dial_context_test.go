package gvisor

import (
	"context"
	"net"
	"testing"
	"time"
)

type l3DialClient struct{ changingIPClient }

func (*l3DialClient) CanUseTCPTunnel() bool { return false }

func TestL3DialTCPHonorsContext(t *testing.T) {
	for _, cancelImmediately := range []bool{false, true} {
		c := &l3DialClient{changingIPClient{ip: net.IP{192, 0, 2, 2}}}
		s, err := NewOwnedStack(c, func() bool { return false })
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		if cancelImmediately {
			cancel()
		}
		finished := make(chan error, 1)
		go func() {
			conn, err := s.DialTCP(ctx, &net.TCPAddr{IP: net.IP{192, 0, 2, 1}, Port: 443})
			if err == nil {
				conn.Close()
			}
			finished <- err
		}()
		select {
		case err := <-finished:
			if err == nil {
				t.Error("dial succeeded without a responding tunnel")
			}
		case <-time.After(time.Second):
			t.Error("L3 TCP dial ignored context cancellation/deadline")
		}
		cancel()
		s.Close()
	}
}
