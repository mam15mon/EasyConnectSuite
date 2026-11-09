package mobile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"EasyConnectSuite/vpncore/client/easyconnect"
	"EasyConnectSuite/vpncore/log"
	"EasyConnectSuite/vpncore/stack/tun"
	"EasyConnectSuite/vpncore/underlay"
)

var vpnClient *easyconnect.Client
var vpnUnderlay *underlay.Dialer
var vpnClientMu sync.Mutex
var loginMu sync.Mutex
var tunnelRunning atomic.Bool
var loginGeneration uint64
var loginCancelled chan struct{}

func Login(server string, username string, password string) string {
	log.Init()

	return login(server, username, password, "")
}

// LoginWithTOTP logs in with a TOTP second factor (EasyConnect Suite fork:
// servers that require 2FA reject plain Login).
func LoginWithTOTP(server string, username string, password string, totpSecret string) string {
	log.Init()

	return login(server, username, password, totpSecret)
}

func DebugLogin(server string, username string, password string) string {
	log.Init()
	log.EnableDebug()

	return login(server, username, password, "")
}

func Logout() {
	vpnClientMu.Lock()
	loginGeneration++
	if loginCancelled != nil {
		close(loginCancelled)
		loginCancelled = nil
	}
	oldClient, oldUnderlay := vpnClient, vpnUnderlay
	vpnClient, vpnUnderlay = nil, nil
	vpnClientMu.Unlock()
	if oldClient != nil {
		oldClient.Close()
	}
	if oldUnderlay != nil {
		_ = oldUnderlay.Close()
	}
}

func login(server string, username string, password string, totpSecret string) string {
	loginMu.Lock()
	defer loginMu.Unlock()

	vpnClientMu.Lock()
	loginGeneration++
	generation := loginGeneration
	cancelled := make(chan struct{})
	loginCancelled = cancelled
	oldClient, oldUnderlay := vpnClient, vpnUnderlay
	vpnClient, vpnUnderlay = nil, nil
	vpnClientMu.Unlock()
	op := log.BeginOperation(fmt.Sprintf("[android pid=%d session=%d] ", os.Getpid(), generation), "android_login", cancelled)
	var resultErr error
	defer func() { op.Finish("success", resultErr) }()
	op.Step("close_previous")
	if oldClient != nil {
		oldClient.Close()
	}
	if oldUnderlay != nil {
		_ = oldUnderlay.Close()
	}
	op.Step("create_underlay")
	newUnderlay, err := underlay.New(underlay.Options{AutoDetect: false})
	if err != nil {
		resultErr = err
		return ""
	}
	newClient := easyconnect.NewClient(easyconnect.Options{
		OwnedLifecycle: true,
		Server:         server,
		Auth:           easyconnect.AuthOptions{Username: username, Password: password, TOTPSecret: totpSecret},
		UnderlayDialer: newUnderlay,
	})
	vpnClientMu.Lock()
	if generation != loginGeneration {
		vpnClientMu.Unlock()
		newClient.Close()
		_ = newUnderlay.Close()
		resultErr = context.Canceled
		return ""
	}
	vpnClient, vpnUnderlay = newClient, newUnderlay
	vpnClientMu.Unlock()

	op.Step("setup")
	err = newClient.Setup()
	if err == nil {
		op.Step("read_ip")
		var clientIP string
		ip, ipErr := newClient.IP()
		if ipErr == nil {
			clientIP = ip.String()
		}
		vpnClientMu.Lock()
		current := generation == loginGeneration
		vpnClientMu.Unlock()
		if !current {
			resultErr = context.Canceled
			return ""
		}
		if ipErr == nil {
			return clientIP
		}
		err = ipErr
	}
	vpnClientMu.Lock()
	if generation == loginGeneration {
		vpnClient, vpnUnderlay = nil, nil
	}
	vpnClientMu.Unlock()
	newClient.Close()
	_ = newUnderlay.Close()
	if errors.Is(err, context.Canceled) {
		resultErr = context.Canceled
	} else {
		resultErr = err
	}
	return ""
}

// DNSServer returns the VPN-side DNS server of the current session, or ""
// when not logged in / not provided. The Android VpnService advertises it
// to the system.
func DNSServer() string {
	vpnClientMu.Lock()
	client := vpnClient
	vpnClientMu.Unlock()
	if client == nil {
		return ""
	}
	dns, err := client.DNSServer()
	if err != nil {
		return ""
	}
	return dns
}

// TunnelRunning reports whether the TUN forwarding loop is alive.
func TunnelRunning() bool {
	return tunnelRunning.Load()
}

func StartStack(fd int) {
	vpnClientMu.Lock()
	client := vpnClient
	vpnClientMu.Unlock()
	if client == nil {
		return
	}

	vpnTUNStack, err := tun.NewStack(client, false, false, nil)
	if err != nil {
		log.Printf("tun stack setup failed: %v", err)
		return
	}

	vpnTUNStack.SetupTun(fd)

	tunnelRunning.Store(true)
	defer tunnelRunning.Store(false)
	defer func() {
		// The read loops return on tunnel death; a residual panic must
		// not take the whole Android app down.
		if r := recover(); r != nil {
			log.Printf("tun stack stopped: %v", r)
		}
	}()
	vpnTUNStack.Run()
}
