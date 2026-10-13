package easyconnect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/underlay"
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
