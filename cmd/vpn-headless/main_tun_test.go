package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"EasyConnectSuite/engine"
)

type headlessTestTUN struct {
	engine.TUNController
	close func() error
}

func (c headlessTestTUN) Close() error { return c.close() }

type forbiddenCredentialReader struct{ t *testing.T }

func (r forbiddenCredentialReader) Read([]byte) (int, error) {
	r.t.Error("credentials read before privilege or option check")
	return 0, io.EOF
}

func TestRunTUNChecksPrivilegesBeforeCredentials(t *testing.T) {
	expected := errors.New("synthetic privilege denied")
	err := runWithTUN(context.Background(), []string{"-server", "x", "-username", "u", "-tun"}, forbiddenCredentialReader{t}, io.Discard, io.Discard,
		func(engine.Options) (runningEngine, error) {
			t.Fatal("privilege failure started engine")
			return nil, nil
		},
		func() (headlessTUN, error) { return nil, expected })
	if !errors.Is(err, expected) {
		t.Fatalf("privilege error: %v", err)
	}
}

func TestRunRejectsNoProxyWithoutTUNBeforeCredentials(t *testing.T) {
	err := runWithTUN(context.Background(), []string{"-server", "x", "-username", "u", "-no-proxy"}, forbiddenCredentialReader{t}, io.Discard, io.Discard,
		func(engine.Options) (runningEngine, error) {
			t.Fatal("invalid options started engine")
			return nil, nil
		},
		func() (headlessTUN, error) { t.Fatal("invalid options created controller"); return nil, nil })
	if err == nil {
		t.Fatal("accepted disabled TUN and proxies")
	}
}

func TestRunTUNReportsFailureAndPreservesProxySession(t *testing.T) {
	for _, noProxy := range []bool{false, true} {
		t.Run(map[bool]string{false: "with_proxy", true: "without_proxy"}[noProxy], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session := &fakeEngine{}
			cleanupFailure := errors.New("synthetic cleanup failure")
			closed := false
			controller := headlessTestTUN{close: func() error {
				if !session.stopped.Load() {
					t.Error("controller closed before engine stopped")
				}
				closed = true
				return cleanupFailure
			}}
			args := []string{"-server", "x", "-username", "u", "-tun"}
			if noProxy {
				args = append(args, "-no-proxy")
			}
			var output bytes.Buffer
			err := runWithTUN(ctx, args, strings.NewReader("tun-private-canary\n"), &output, io.Discard,
				func(opts engine.Options) (runningEngine, error) {
					if !opts.TUNEnabled || opts.TUNController == nil || opts.Config.TUNMode {
						t.Error("independent TUN controller not configured")
					}
					if noProxy != (opts.Config.SocksBind == "" && opts.Config.HTTPBind == "") {
						t.Error("proxy mode not applied")
					}
					opts.OnStatus(engine.Status{Phase: engine.PhaseConnected})
					opts.OnTUNStatus(engine.TUNStatus{State: "error", Error: "synthetic TUN failure"})
					if !noProxy {
						cancel()
					}
					return session, nil
				}, func() (headlessTUN, error) { return controller, nil })
			if !closed || !errors.Is(err, cleanupFailure) {
				t.Fatalf("cleanup result: closed=%t err=%v", closed, err)
			}
			if noProxy && !strings.Contains(err.Error(), "TUN 模式失败") {
				t.Fatalf("no-proxy failure not returned: %v", err)
			}
			if !noProxy && !strings.Contains(output.String(), "VPN 与本地代理继续运行") {
				t.Fatal("proxy continuity not reported")
			}
			if strings.Contains(output.String(), "canary") || strings.Contains(err.Error(), "VPN 连接终止") {
				t.Fatal("TUN failure leaked a credential or misreported VPN failure")
			}
		})
	}
}

func TestRunTUNClosesControllerAfterStartupFailure(t *testing.T) {
	startFailure, cleanupFailure := errors.New("synthetic startup failure"), errors.New("synthetic cleanup failure")
	closed := false
	err := runWithTUN(context.Background(), []string{"-server", "x", "-username", "u", "-tun"}, strings.NewReader("secret\n"), io.Discard, io.Discard,
		func(engine.Options) (runningEngine, error) { return nil, startFailure },
		func() (headlessTUN, error) {
			return headlessTestTUN{close: func() error { closed = true; return cleanupFailure }}, nil
		})
	if !closed || !errors.Is(err, startFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("startup cleanup: closed=%t err=%v", closed, err)
	}
}
