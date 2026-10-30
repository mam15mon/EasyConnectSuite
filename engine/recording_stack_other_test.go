//go:build !windows

package engine

import (
	"context"
	"net"

	"EasyConnectSuite/vpncore/stack"
)

type recordingVPNStack struct {
	stack.Stack
	calls int
}

func (s *recordingVPNStack) DialTCP(context.Context, *net.TCPAddr) (net.Conn, error) {
	s.calls++
	a, b := net.Pipe()
	b.Close()
	return a, nil
}
