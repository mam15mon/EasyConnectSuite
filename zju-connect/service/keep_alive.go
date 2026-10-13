package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"EasyConnectSuite/vpncore/log"
)

const (
	keepAliveRequestTimeout = 10 * time.Second
	keepAliveDrainLimit     = 32 << 10
)

func KeepAlive(ctx context.Context, resolver ProxyResolver, dialer ProxyDialer, keepAliveURL string) {
	if keepAliveURL != "" {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = dialer.Dial
		transport.ResponseHeaderTimeout = keepAliveRequestTimeout
		client := &http.Client{
			Transport: transport,
		}
		defer client.CloseIdleConnections()

		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		runHTTPKeepAlive(ctx, client, keepAliveURL, ticker.C)
		return
	} else {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			remoteUDPResolver, _ := resolver.RemoteUDPResolver()
			remoteTCPResolver, _ := resolver.RemoteTCPResolver()
			useTCP := remoteUDPResolver == nil
			requestCtx, cancel := context.WithTimeout(ctx, keepAliveRequestTimeout)

			if remoteUDPResolver != nil {
				_, err := remoteUDPResolver.LookupIP(requestCtx, "ip4", "www.baidu.com")
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						log.Printf("KeepAlive using UDP error: %s", err)
					}
					useTCP = true
				} else {
					log.Printf("KeepAlive using UDP: OK")
				}
			}

			if useTCP && remoteTCPResolver != nil {
				_, err := remoteTCPResolver.LookupIP(requestCtx, "ip4", "www.baidu.com")
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						log.Printf("KeepAlive using TCP error: %s", err)
					}
				} else {
					log.Printf("KeepAlive using TCP: OK")
				}
			}
			cancel()

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}
}

func runHTTPKeepAlive(ctx context.Context, client *http.Client, keepAliveURL string, ticks <-chan time.Time) {
	for {
		requestCtx, cancel := context.WithTimeout(ctx, keepAliveRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, keepAliveURL, nil)
		if err != nil {
			log.Printf("KeepAlive: %s", err)
		} else {
			resp, err := client.Do(req)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					if requestError, ok := err.(*url.Error); ok {
						err = requestError.Err
					}
					log.Printf("KeepAlive HTTP failed: %s", err)
				}
			} else {
				log.Printf("KeepAlive: OK, status code %d", resp.StatusCode)
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, keepAliveDrainLimit+1))
				_ = resp.Body.Close()
			}
		}
		cancel()

		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}
