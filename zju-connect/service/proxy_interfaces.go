package service

import (
	"context"
	"net"
)

// Interfaces let an embedded client refresh resources without restarting
// proxy listeners. Existing *dial.Dialer and *resolve.Resolver still satisfy them.
type ProxyDialer interface {
	Dial(context.Context, string, string) (net.Conn, error)
	DialIPPort(context.Context, string, string) (net.Conn, error)
}

type ProxyResolver interface {
	Resolve(context.Context, string) (context.Context, net.IP, error)
	RemoteUDPResolver() (*net.Resolver, error)
	RemoteTCPResolver() (*net.Resolver, error)
}
