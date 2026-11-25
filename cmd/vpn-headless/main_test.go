package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"EasyConnectSuite/engine"
)

type fakeEngine struct{ stopped atomic.Bool }

func (f *fakeEngine) Stop() { f.stopped.Store(true) }

func TestRunCancelsAndStopsWithLocalProxyAndPrivateSecrets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &fakeEngine{}
	var out, diagnostic bytes.Buffer
	err := run(ctx, []string{"-server", "vpn.invalid", "-username", "test", "-totp-stdin", "-proxy-all"}, strings.NewReader("password-canary\ntotp-canary\n"), &out, &diagnostic, func(o engine.Options) (runningEngine, error) {
		if o.Config.Password != "password-canary" || o.Config.TOTPSecret != "totp-canary" {
			t.Fatal("secrets were not passed to engine")
		}
		if o.Config.SocksBind != "127.0.0.1:1080" || o.Config.HTTPBind != "127.0.0.1:1081" {
			t.Fatal("proxy is not loopback-only")
		}
		if !o.Config.ProxyAll {
			t.Fatal("proxy-all was not passed to engine")
		}
		if !o.Config.AutoDetectInterface || o.Config.BindInterface != "" || o.OnResources != nil {
			t.Fatal("manual interface or resource output enabled by default")
		}
		if o.Config.TUNMode || o.TUNEnabled {
			t.Fatal("TUN unexpectedly enabled")
		}
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		cancel()
		return fake, nil
	})
	if err != nil || !fake.stopped.Load() {
		t.Fatalf("cancel cleanup: %v stopped=%t", err, fake.stopped.Load())
	}
	if strings.Contains(out.String()+diagnostic.String(), "canary") {
		t.Fatal("secret appeared in output")
	}
}

func TestRunTerminalFailureStopsEngine(t *testing.T) {
	fake := &fakeEngine{}
	err := run(context.Background(), []string{"-server", "vpn.invalid", "-username", "test"}, strings.NewReader("password\n"), &bytes.Buffer{}, &bytes.Buffer{}, func(o engine.Options) (runningEngine, error) {
		o.OnStatus(engine.Status{Phase: engine.PhaseError})
		return fake, nil
	})
	if err == nil || !fake.stopped.Load() {
		t.Fatalf("terminal failure: %v stopped=%t", err, fake.stopped.Load())
	}
}

func TestRunRejectsInvalidArgumentsBeforeStarting(t *testing.T) {
	for _, args := range [][]string{
		{"-server", "x", "-username", "u", "-socks-port", "1081"},
		{"-server", "x", "-username", "u", "-protocol", "unknown"},
		{"-server", "x", "-username", "u", "-insecure-skip-verify"},
		{"-server", "x", "-username", "u", "-protocol", "univpn", "-keepalive-url", "https://probe.invalid"},
		{"-server", "x", "-username", "u", "-protocol", "topsec", "-disable-keepalive"},
		{"-server", "x", "-username", "u", "-protocol", "univpn", "-totp-stdin"},
		{"-server", "x", "-username", "u", "secret-argument"},
	} {
		err := run(context.Background(), args, strings.NewReader("secret\n"), &bytes.Buffer{}, &bytes.Buffer{}, func(engine.Options) (runningEngine, error) {
			t.Fatal("invalid request started engine")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
}

func TestRunCertificateVerification(t *testing.T) {
	for _, protocol := range []string{"univpn", "topsec"} {
		t.Run(protocol, func(t *testing.T) {
			if err := checkHeadlessProtocol(protocol); err != nil {
				t.Skip(err)
			}
			for _, tc := range []struct {
				name string
				flag string
				skip bool
			}{
				{name: "default"},
				{name: "explicit opt-in", flag: "-insecure-skip-verify", skip: true},
				{name: "explicit verification", flag: "-insecure-skip-verify=false"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					args := []string{"-protocol", protocol, "-server", "vpn.invalid", "-username", "test"}
					if tc.flag != "" {
						args = append(args, tc.flag)
					}
					started := errors.New("engine reached")
					err := run(context.Background(), args, strings.NewReader("password\n"), io.Discard, io.Discard, func(o engine.Options) (runningEngine, error) {
						if o.Config.Protocol != protocol || o.InsecureSkipVerify != tc.skip {
							t.Fatalf("protocol=%q skip verification=%t, want %q and %t", o.Config.Protocol, o.InsecureSkipVerify, protocol, tc.skip)
						}
						return nil, started
					})
					if !errors.Is(err, started) {
						t.Fatalf("engine was not reached: %v", err)
					}
				})
			}
		})
	}
}

func TestRunEasyConnectUnderlayAndKeepAlive(t *testing.T) {
	for _, disable := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		args := []string{"-server", "vpn.invalid", "-username", "test", "-bind-interface", "eth0", "-keepalive-url", " https://probe.invalid "}
		wantURL := "https://probe.invalid"
		if disable {
			args = append(args, "-disable-keepalive")
			wantURL = ""
		}
		err := run(ctx, args, strings.NewReader("password\n"), io.Discard, io.Discard, func(o engine.Options) (runningEngine, error) {
			if o.Config.BindInterface != "eth0" || o.Config.AutoDetectInterface {
				t.Fatal("explicit underlay interface was not passed to engine")
			}
			if o.Config.DisableKeepAlive != disable || o.Config.KeepAliveURL != wantURL {
				t.Fatalf("keepalive disabled=%t URL=%q, want %t and %q", o.Config.DisableKeepAlive, o.Config.KeepAliveURL, disable, wantURL)
			}
			cancel()
			return &fakeEngine{}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunShowsStatusAndResourceUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	args := []string{"-server", "vpn.invalid", "-username", "test", "-show-resources"}
	err := run(ctx, args, strings.NewReader("password\n"), &out, io.Discard, func(o engine.Options) (runningEngine, error) {
		if o.OnResources == nil {
			t.Fatal("resource callback was not wired")
		}
		o.OnResources([]engine.ResourceEntry{{Kind: "ip", Target: "192.0.2.1", Ports: "443", Protocol: "TCP"}})
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected, ClientIP: "198.51.100.2", DNS: "198.51.100.53"})
		o.OnStatus(engine.Status{Phase: engine.PhaseReconnecting, Stage: "认证中", Message: "正在重新登录"})
		o.OnResources(nil)
		cancel()
		return &fakeEngine{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`授权资源: 1`, `类型="ip" 目标="192.0.2.1" 端口="443" 协议="TCP"`, `VPN地址="198.51.100.2" DNS="198.51.100.53"`, `状态: reconnecting 阶段="认证中" 消息="正在重新登录"`, `授权资源: 0`} {
		if !strings.Contains(out.String(), part) {
			t.Errorf("output missing %q: %s", part, out.String())
		}
	}
}

func TestRunReturnsStartupError(t *testing.T) {
	expected := errors.New("startup failed")
	err := run(context.Background(), []string{"-server", "x", "-username", "u"}, strings.NewReader("secret\n"), &bytes.Buffer{}, &bytes.Buffer{}, func(engine.Options) (runningEngine, error) { return nil, expected })
	if !errors.Is(err, expected) {
		t.Fatalf("startup error lost: %v", err)
	}
}

func TestHelpDoesNotReadSecretsOrStartEngine(t *testing.T) {
	var help bytes.Buffer
	err := run(context.Background(), []string{"-h"}, strings.NewReader(""), &bytes.Buffer{}, &help, func(engine.Options) (runningEngine, error) {
		t.Fatal("help started engine")
		return nil, nil
	})
	if err != nil || !strings.Contains(help.String(), "-server") || !strings.Contains(help.String(), "-insecure-skip-verify") {
		t.Fatalf("help result: %v, %q", err, help.String())
	}
}

func TestEmptyAndOversizedSecretsAreRejected(t *testing.T) {
	for _, secret := range []string{"\n", strings.Repeat("x", 4097) + "\n"} {
		err := run(context.Background(), []string{"-server", "x", "-username", "u"}, strings.NewReader(secret), &bytes.Buffer{}, &bytes.Buffer{}, func(engine.Options) (runningEngine, error) {
			t.Fatal("invalid secret started engine")
			return nil, nil
		})
		if err == nil {
			t.Fatal("invalid secret accepted")
		}
	}
}

func TestRunPreservesMaximumLengthSecrets(t *testing.T) {
	password := strings.Repeat("p", 4096)
	secret := strings.Repeat("s", 4096)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &fakeEngine{}
	err := run(ctx, []string{"-server", "x", "-username", "u", "-totp-stdin"}, strings.NewReader(password+"\r\n"+secret+"\r\n"), &bytes.Buffer{}, &bytes.Buffer{}, func(o engine.Options) (runningEngine, error) {
		if o.Config.Password != password || o.Config.TOTPSecret != secret {
			t.Fatal("maximum-length credentials were truncated")
		}
		cancel()
		return session, nil
	})
	if err != nil || !session.stopped.Load() {
		t.Fatalf("run: %v", err)
	}
}

func TestRunRejectsOversizedSecondSecret(t *testing.T) {
	password := strings.Repeat("p", 4096)
	secret := strings.Repeat("s", 4097)
	err := run(context.Background(), []string{"-server", "x", "-username", "u", "-totp-stdin"}, strings.NewReader(password+"\r\n"+secret+"\n"), &bytes.Buffer{}, &bytes.Buffer{}, func(engine.Options) (runningEngine, error) {
		t.Fatal("oversized TOTP secret started engine")
		return nil, nil
	})
	if err == nil {
		t.Fatal("oversized TOTP secret accepted")
	}
}

func TestRunCancellationWhileReadingCredentials(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-server", "x", "-username", "u"}, reader, &bytes.Buffer{}, &bytes.Buffer{}, func(engine.Options) (runningEngine, error) {
			t.Error("canceled credential read started engine")
			return &fakeEngine{}, nil
		})
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("credential read did not respond to cancellation")
	}
}

type overlapWriter struct {
	active     atomic.Int32
	overlapped atomic.Bool
	writes     atomic.Int32
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) != 1 {
		w.overlapped.Store(true)
	}
	time.Sleep(time.Millisecond)
	w.writes.Add(1)
	w.active.Add(-1)
	return len(p), nil
}

func TestRunSerializesConcurrentStatusOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &overlapWriter{}
	session := &fakeEngine{}
	err := run(ctx, []string{"-server", "x", "-username", "u"}, strings.NewReader("secret\n"), w, &bytes.Buffer{}, func(o engine.Options) (runningEngine, error) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; o.OnStatus(engine.Status{Phase: engine.PhaseConnecting}) }()
		}
		close(start)
		wg.Wait()
		cancel()
		return session, nil
	})
	if err != nil || !session.stopped.Load() || w.overlapped.Load() || w.writes.Load() != 32 {
		t.Fatalf("status output: err=%v stopped=%t overlap=%t writes=%d", err, session.stopped.Load(), w.overlapped.Load(), w.writes.Load())
	}
}
