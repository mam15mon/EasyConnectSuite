package service

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/mythologyli/zju-connect/internal/hook_func"
	"github.com/mythologyli/zju-connect/log"
	"github.com/things-go/go-socks5"
)

func ServeSocks5(bindAddr string, dialer ProxyDialer, resolver ProxyResolver, user string, password string) {
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

	log.Printf("SOCKS5 server listening on %s", bindAddr)

	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		// Do not panic: when embedded in the EasyConnect Suite GUI a
		// panic here would take the whole app down. Just report and stop.
		log.Printf("SOCKS5 listen failed: %v", err)
		return
	}

	hook_func.RegisterTerminalFunc("CloseSocks5Listener", func(ctx context.Context) error {
		log.Println("Closing SOCKS5 listener...")
		if err := listener.Close(); err != nil {
			return fmt.Errorf("close SOCKS5 listener failed: %w", err)
		}
		return nil
	})

	if err = server.Serve(listener); err != nil {
		if errors.Is(err, net.ErrClosed) {
			log.Println("SOCKS5 server closed")
		} else {
			log.Println("SOCKS5 listen failed: " + err.Error())
		}
	}
}
