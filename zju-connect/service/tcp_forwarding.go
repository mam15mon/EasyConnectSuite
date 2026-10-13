package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"EasyConnectSuite/vpncore/client"
	"EasyConnectSuite/vpncore/internal/hook_func"
	"EasyConnectSuite/vpncore/log"
)

func handleTCPForwardingRequest(dialContext client.DialContextFunc, conn net.Conn, remoteAddress string) {
	log.Printf("Port forwarding (TCP): %s -> %s -> %s", conn.RemoteAddr(), conn.LocalAddr(), remoteAddress)

	proxy, err := dialContext(context.Background(), "tcp", remoteAddress)
	if err != nil {
		log.Printf("Port forwarding (TCP) dial %s failed: %v", remoteAddress, err)
		_ = conn.Close()
		return
	}

	go copyIO(conn, proxy)
	go copyIO(proxy, conn)
}

func copyIO(src, dest net.Conn) {
	defer func(src net.Conn) {
		_ = src.Close()
	}(src)
	defer func(dest net.Conn) {
		_ = dest.Close()
	}(dest)
	_, _ = io.Copy(src, dest)
}

func ServeTCPForwarding(dialContext client.DialContextFunc, bindAddress string, remoteAddress string) {
	ln, err := net.Listen("tcp", bindAddress)
	if err != nil {
		// Do not panic: when embedded in the EasyConnect Suite GUI a
		// panic here would take the whole app down.
		log.Printf("TCP forwarding listen failed: %v", err)
		return
	}

	log.Printf("TCP port forwarding: %s -> %s", bindAddress, remoteAddress)

	hook_func.RegisterTerminalFunc("CloseTCPForwardingPort", func(ctx context.Context) error {
		log.Println("Closing TCP forwarding port...")
		if err := ln.Close(); err != nil {
			return fmt.Errorf("close TCP forwarding listener failed: %w", err)
		}
		return nil
	})

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("TCP forwarding port closed")
				return
			}
			log.Printf("TCP forwarding accept failed: %v", err)
			return
		}

		go handleTCPForwardingRequest(dialContext, conn, remoteAddress)
	}
}
