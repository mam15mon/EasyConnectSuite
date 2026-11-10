package engine

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"EasyConnectSuite/vpncore/client/easyconnect"
	"EasyConnectSuite/vpncore/configs"
)

func TestStartupTargetConnectionFailure(t *testing.T) {
	for _, protocol := range []string{"univpn", "easyconnect"} {
		t.Run(protocol, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := listener.Addr().(*net.TCPAddr)
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			conf := configs.Default()
			conf.Protocol, conf.ServerAddress, conf.ServerPort = protocol, "127.0.0.1", address.Port
			conf.Username, conf.Password = "test", "test"
			logPath := filepath.Join(t.TempDir(), "engine.log")
			eng, err := Start(Options{Config: conf, ConnectionID: "target-failure", LogFile: logPath})
			if eng != nil {
				eng.Stop()
				t.Fatal("unreachable target started an engine")
			}
			var dialErr *net.OpError
			if !errors.As(err, &dialErr) || dialErr.Op != "dial" || errors.Is(err, easyconnect.ErrLoginRejected) {
				t.Fatalf("lost target dial cause: %v", err)
			}
			if !strings.HasPrefix(err.Error(), "VPN 目标连接失败，") || !strings.Contains(err.Error(), address.String()) {
				t.Fatalf("missing target explanation or actual endpoint: %v", err)
			}
			logged, readErr := os.ReadFile(logPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, marker := range []string{`profile="target-failure"`, "name=startup", "outcome=failed", "stage=authentication", "elapsed=", "timeout=false", "canceled=false", "error_type=", "VPN 目标连接失败"} {
				if !strings.Contains(string(logged), marker) {
					t.Errorf("missing failure evidence %q", marker)
				}
			}
		})
	}
}

func TestStartupOtherFailuresAreNotTargetConnectionFailures(t *testing.T) {
	for _, mode := range []string{"authentication", "certificate", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			server, _ := integrationGateway(t, true)
			host, portText, err := net.SplitHostPort(server)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			conf := configs.Default()
			conf.Protocol, conf.ServerAddress, conf.ServerPort = "univpn", host, port
			conf.Username, conf.Password = "test", "test"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			eng, err := Start(Options{Config: conf, Context: ctx, InsecureSkipVerify: mode != "certificate"})
			if eng != nil {
				eng.Stop()
				t.Fatal("failed setup started an engine")
			}
			if err == nil || strings.Contains(err.Error(), "VPN 目标连接失败") {
				t.Fatalf("misclassified %s failure: %v", mode, err)
			}
			var certificateErr x509.UnknownAuthorityError
			if mode == "authentication" && !errors.Is(err, easyconnect.ErrLoginRejected) ||
				mode == "certificate" && !errors.As(err, &certificateErr) ||
				mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost original %s cause: %v", mode, err)
			}
		})
	}
}
