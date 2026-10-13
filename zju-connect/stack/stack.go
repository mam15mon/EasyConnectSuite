package stack

import (
	"context"
	"net"

	"EasyConnectSuite/vpncore/client"
	"EasyConnectSuite/vpncore/internal/ippool"
	"EasyConnectSuite/vpncore/internal/zcdns"
)

type Stack interface {
	Run()
	SetupResolve(r zcdns.LocalServer)
	SetupIPPool(ipPool *ippool.IPPool[[]client.DomainResource])
	DialTCP(ctx context.Context, addr *net.TCPAddr) (net.Conn, error)
	DialUDP(ctx context.Context, addr *net.UDPAddr) (net.Conn, error)
}
