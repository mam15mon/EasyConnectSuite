package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"EasyConnectSuite/engine/univpn"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"inet.af/netaddr"
)

type managedClient interface {
	client.Client
	Setup() error
	Close()
}

type uniVPNClient struct {
	ctx      context.Context
	cancel   context.CancelFunc
	options  univpn.Options
	session  atomic.Pointer[univpn.Session]
	mu       sync.Mutex
	updateIP func(net.IP) error
}

func newUniVPNClient(ctx context.Context, conf configs.Config, domain string, underlay client.UnderlayDialer) *uniVPNClient {
	ctx, cancel := context.WithCancel(ctx)
	return &uniVPNClient{ctx: ctx, cancel: cancel, options: univpn.Options{
		Server: net.JoinHostPort(conf.ServerAddress, strconv.Itoa(conf.ServerPort)), Domain: domain,
		Username: conf.Username, Password: conf.Password, DialContext: underlay.DialContext,
	}}
}
func (c *uniVPNClient) Setup() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.session.Swap(nil); old != nil {
		_ = old.Close()
	}
	s, err := univpn.Dial(c.ctx, c.options)
	if err != nil {
		if errors.Is(err, univpn.ErrLoginRejected) {
			return fmt.Errorf("%w: %v", easyconnect.ErrLoginRejected, err)
		}
		return err
	}
	if c.updateIP != nil {
		if err := c.updateIP(net.IP(s.IP().AsSlice())); err != nil {
			_ = s.Close()
			return err
		}
	}
	c.session.Store(s)
	return nil
}
func (c *uniVPNClient) Close() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.session.Swap(nil); s != nil {
		_ = s.Close()
	}
}
func (c *uniVPNClient) SetIPUpdateHandler(f func(net.IP) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateIP = f
}
func (c *uniVPNClient) IP() (net.IP, error) {
	if s := c.session.Load(); s != nil {
		return net.IP(s.IP().AsSlice()), nil
	}
	return nil, net.ErrClosed
}
func (c *uniVPNClient) IPResources() ([]client.IPResource, error) {
	s := c.session.Load()
	if s == nil {
		return nil, net.ErrClosed
	}
	resources := make([]client.IPResource, 0, len(s.Routes()))
	for _, prefix := range s.Routes() {
		p := netaddr.MustParseIPPrefix(prefix.String())
		r := p.Range()
		resources = append(resources, client.IPResource{IPMin: r.From().IPAddr().IP, IPMax: r.To().IPAddr().IP, PortMin: 1, PortMax: 65535, Protocol: "all"})
	}
	return resources, nil
}
func (c *uniVPNClient) IPSet() (*netaddr.IPSet, error) {
	s := c.session.Load()
	if s == nil {
		return nil, net.ErrClosed
	}
	var b netaddr.IPSetBuilder
	for _, p := range s.Routes() {
		b.AddPrefix(netaddr.MustParseIPPrefix(p.String()))
	}
	return b.IPSet()
}
func (c *uniVPNClient) DomainResources() (client.DomainResources, error) {
	return nil, client.ErrResourceNotFound
}
func (c *uniVPNClient) DNSResource() (map[string][]net.IP, error) {
	return nil, client.ErrResourceNotFound
}
func (c *uniVPNClient) DNSServer() (string, error)    { return "", client.ErrResourceNotFound }
func (c *uniVPNClient) DNSServers() ([]string, error) { return nil, client.ErrResourceNotFound }
func (c *uniVPNClient) CanUseTCPTunnel() bool         { return false }
func (c *uniVPNClient) DialTCP(context.Context, *net.TCPAddr) (net.Conn, error) {
	return nil, errors.New("UniVPN 使用 IPv4 数据通道")
}
func (c *uniVPNClient) NewL3Conn() (io.ReadWriteCloser, error) {
	if s := c.session.Load(); s != nil {
		return s, nil
	}
	return nil, net.ErrClosed
}
