package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"EasyConnectSuite/engine"
)

type fakeEngine struct{ stopped atomic.Bool }

func (f *fakeEngine) Stop() { f.stopped.Store(true) }

func TestRunCancelsAndStopsWithLocalProxyAndPrivateSecrets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &fakeEngine{}
	var out, diagnostic bytes.Buffer
	err := run(ctx, []string{"-server", "vpn.invalid", "-username", "test", "-totp-stdin"}, strings.NewReader("password-canary\ntotp-canary\n"), &out, &diagnostic, func(o engine.Options) (runningEngine, error) {
		if o.Config.Password != "password-canary" || o.Config.TOTPSecret != "totp-canary" {
			t.Fatal("secrets were not passed to engine")
		}
		if o.Config.SocksBind != "127.0.0.1:1080" || o.Config.HTTPBind != "127.0.0.1:1081" {
			t.Fatal("proxy is not loopback-only")
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
	if err != nil || !strings.Contains(help.String(), "-server") {
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
