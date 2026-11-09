package easyconnect

import (
	"bytes"
	"context"
	"errors"
	"io"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"EasyConnectSuite/vpncore/underlay"
)

func TestCloseCancelsReauthenticationAndWaitingKeepalive(t *testing.T) {
	loginStarted := make(chan struct{})
	var keepaliveRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/por/login_auth.csp":
			close(loginStarted)
			<-r.Context().Done()
		case "/por/update_session.csp":
			keepaliveRequests.Add(1)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := NewClient(Options{OwnedLifecycle: true, Server: strings.TrimPrefix(server.URL, "https://"),
		SessionID: "expired-session", UnderlayDialer: newTestUnderlay(t, underlay.Options{AutoDetect: false})})
	defer c.Close()
	c.httpClient = server.Client()
	c.setupStarted = true
	setupDone := make(chan error, 1)
	go func() { setupDone <- c.Setup() }()
	select {
	case <-loginStarted:
	case <-time.After(time.Second):
		t.Fatal("reauthentication did not start")
	}
	keepaliveDone := make(chan error, 1)
	go func() { keepaliveDone <- c.requestUpdateSession(c.lifecycleCtx) }()
	sendDone, recvDone := make(chan error, 1), make(chan error, 1)
	go func() { _, err := c.SendConn(); sendDone <- err }()
	go func() { _, err := c.RecvConn(); recvDone <- err }()
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close deadlocked with reauthentication or keepalive")
	}
	if err := <-setupDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("setup cancellation = %v", err)
	}
	select {
	case err := <-keepaliveDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("keepalive cancellation = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("keepalive did not stop after Close")
	}
	if keepaliveRequests.Load() != 0 {
		t.Fatal("keepalive sent a request during incomplete authentication")
	}
	for _, done := range []chan error{sendDone, recvDone} {
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("tunnel handshake cancellation = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("tunnel handshake did not stop after Close")
		}
	}
	if err := c.Setup(); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed client setup = %v", err)
	}
}

func TestRequestConfigRejectsHTTPError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("<html>session expired</html>"))
	}))
	defer server.Close()
	c := NewClient(Options{Server: strings.TrimPrefix(server.URL, "https://"), SessionID: "expired-session"})
	defer c.Close()
	c.httpClient = server.Client()
	config, err := c.requestConfig()
	if err == nil || !strings.Contains(err.Error(), "401") || config != "" {
		t.Fatalf("HTTP error returned as configuration: body=%q error=%v", config, err)
	}
}

// Signal the completed token request so cancellation occurs before the IP delay ends.
type tokenLogSignal struct {
	once  sync.Once
	ready chan struct{}
}

func (w *tokenLogSignal) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("Tunnel token received")) {
		w.once.Do(func() { close(w.ready) })
	}
	return len(p), nil
}

func TestCloseCancelsWaitBeforeIPRequest(t *testing.T) {
	c, _, _, _ := reloginGateway(t, "")
	signal := &tokenLogSignal{ready: make(chan struct{})}
	original := stdlog.Writer()
	stdlog.SetOutput(io.MultiWriter(original, signal))
	defer stdlog.SetOutput(original)
	done := make(chan error, 1)
	go func() { done <- c.Setup() }()
	select {
	case <-signal.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("token request did not complete")
	}
	c.lifecycleCancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("setup error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("setup did not cancel during the wait before IP request")
	}
}
