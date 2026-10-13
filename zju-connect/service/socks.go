package service

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"EasyConnectSuite/vpncore/internal/hook_func"
	"EasyConnectSuite/vpncore/log"
	"github.com/things-go/go-socks5"
)

func StartSocks5(bindAddr string, dialer ProxyDialer, resolver ProxyResolver, user string, password string) (io.Closer, error) {
	var authMethods []socks5.Authenticator
	if user != "" && password != "" {
		authMethods = append(authMethods, socks5.UserPassAuthenticator{
			Credentials: socks5.StaticCredentials{user: password},
		})

		log.Println("Neither traffic nor credentials are encrypted in the SOCKS5 protocol!")
		log.Println("DO NOT deploy it to the public network. All consequences and responsibilities have nothing to do with the developer")
	} else {
		authMethods = append(authMethods, socks5.NoAuthAuthenticator{})
	}

	server := socks5.NewServer(
		socks5.WithAuthMethods(authMethods),
		socks5.WithResolver(resolver),
		socks5.WithDial(dialer.DialIPPort),
		socks5.WithLogger(socks5.NewLogger(log.NewLogger("[SOCKS5] "))),
	)

	raw, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}
	listener := &ownedSocksListener{Listener: raw, conns: make(map[*ownedSocksConn]bool)}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("SOCKS5 server failed: %v", err)
		}
	}()
	return listener, nil
}

func ServeSocks5(bindAddr string, dialer ProxyDialer, resolver ProxyResolver, user string, password string) {
	listener, err := StartSocks5(bindAddr, dialer, resolver, user, password)
	if err != nil {
		log.Printf("SOCKS5 listen failed: %v", err)
		return
	}
	hook_func.RegisterTerminalFunc("CloseSocks5Listener", func(context.Context) error { return listener.Close() })
}

// Own accepted SOCKS connections as well as the listening socket.
type ownedSocksListener struct {
	net.Listener
	mu     sync.Mutex
	closed bool
	conns  map[*ownedSocksConn]bool
}
type ownedSocksConn struct {
	net.Conn
	owner *ownedSocksListener
}

func (l *ownedSocksListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		c.Close()
		return nil, net.ErrClosed
	}
	owned := &ownedSocksConn{Conn: c, owner: l}
	l.conns[owned] = true
	return owned, nil
}
func (l *ownedSocksListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	conns := make([]*ownedSocksConn, 0, len(l.conns))
	for c := range l.conns {
		conns = append(conns, c)
	}
	l.mu.Unlock()
	err := l.Listener.Close()
	for _, c := range conns {
		c.Close()
	}
	return err
}
func (c *ownedSocksConn) Close() error {
	c.owner.mu.Lock()
	delete(c.owner.conns, c)
	c.owner.mu.Unlock()
	return c.Conn.Close()
}
