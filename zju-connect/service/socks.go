package service

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/mythologyli/zju-connect/internal/hook_func"
	"github.com/mythologyli/zju-connect/log"
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

	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}
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
