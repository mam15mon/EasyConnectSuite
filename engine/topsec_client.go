package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"EasyConnectSuite/engine/topsec"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/log"
	"inet.af/netaddr"
)

type topSAPClient struct {
	ctx      context.Context
	cancel   context.CancelFunc
	options  topsec.Options
	session  atomic.Pointer[topsec.Session]
	mu       sync.Mutex
	updateIP func(net.IP) error
}

func newTopSAPClient(ctx context.Context, conf configs.Config, insecureSkipVerify bool, underlay client.UnderlayDialer) *topSAPClient {
	ctx, cancel := context.WithCancel(ctx)
	c := &topSAPClient{ctx: ctx, cancel: cancel, options: topsec.Options{
		Server: net.JoinHostPort(conf.ServerAddress, strconv.Itoa(conf.ServerPort)), Username: conf.Username, Password: conf.Password, DialContext: underlay.DialContext,
	}}
	if insecureSkipVerify {
		// User-selected per-profile option; both channels must still share a key.
		c.options.TLSConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	}
	return c
}
func (c *topSAPClient) Setup() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.session.Swap(nil); old != nil {
		_ = old.Close()
	}
	s, err := topsec.Dial(c.ctx, c.options)
	if err != nil {
		if errors.Is(err, topsec.ErrLoginRejected) {
			var rejection *topsec.AuthenticationError
			if errors.As(err, &rejection) {
				log.Printf("TopSAP authentication rejected: Ret=%d", rejection.Code)
			}
			return fmt.Errorf("%w: %w", easyconnect.ErrLoginRejected, err)
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
func (c *topSAPClient) Close() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.session.Swap(nil); s != nil {
		_ = s.Close()
	}
}
func (c *topSAPClient) SetIPUpdateHandler(f func(net.IP) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateIP = f
}
func (c *topSAPClient) IP() (net.IP, error) {
	if s := c.session.Load(); s != nil {
		return net.IP(s.IP().AsSlice()), nil
	}
	return nil, net.ErrClosed
}
func (c *topSAPClient) IPResources() ([]client.IPResource, error) {
	s := c.session.Load()
	if s == nil {
		return nil, net.ErrClosed
	}
	resources := make([]client.IPResource, 0, len(s.Routes()))
	for _, prefix := range s.Routes() {
		r := netaddr.MustParseIPPrefix(prefix.String()).Range()
		resources = append(resources, client.IPResource{IPMin: r.From().IPAddr().IP, IPMax: r.To().IPAddr().IP, PortMin: 1, PortMax: 65535, Protocol: "all"})
	}
	return resources, nil
}
func (c *topSAPClient) IPSet() (*netaddr.IPSet, error) {
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
func (c *topSAPClient) DomainResources() (client.DomainResources, error) {
	return nil, client.ErrResourceNotFound
}
func (c *topSAPClient) DNSResource() (map[string][]net.IP, error) {
	return nil, client.ErrResourceNotFound
}
func (c *topSAPClient) DNSServer() (string, error) {
	servers, err := c.DNSServers()
	if err != nil {
		return "", err
	}
	return servers[0], nil
}
func (c *topSAPClient) DNSServers() ([]string, error) {
	if s := c.session.Load(); s != nil {
		servers := s.DNSServers()
		if len(servers) != 0 {
			return servers, nil
		}
		return nil, client.ErrResourceNotFound
	}
	return nil, net.ErrClosed
}
func (c *topSAPClient) CanUseTCPTunnel() bool { return false }
func (c *topSAPClient) DialTCP(context.Context, *net.TCPAddr) (net.Conn, error) {
	return nil, errors.New("TopSAP 使用 IPv4 数据通道")
}
func (c *topSAPClient) NewL3Conn() (io.ReadWriteCloser, error) {
	if s := c.session.Load(); s != nil {
		return s, nil
	}
	return nil, net.ErrClosed
}
