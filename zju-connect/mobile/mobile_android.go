package mobile

import (
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
	defer vpnClientMu.Unlock()

	if vpnClient != nil {
		vpnClient.Close()
		vpnClient = nil
	}
	if vpnUnderlay != nil {
		_ = vpnUnderlay.Close()
		vpnUnderlay = nil
	}
}

func login(server string, username string, password string, totpSecret string) string {
	loginMu.Lock()
	defer loginMu.Unlock()

	newUnderlay, err := underlay.New(underlay.Options{AutoDetect: false})
	if err != nil {
		return ""
	}
	newClient := easyconnect.NewClient(easyconnect.Options{
		Server: server,
		Auth: easyconnect.AuthOptions{
			Username:   username,
			Password:   password,
			TOTPSecret: totpSecret,
		},
		UnderlayDialer: newUnderlay,
	})

	// Close the old client and clear vpnClient to nil during setup so that
	// concurrent StartStack calls see nil and return early rather than
	// operating on an uninitialized client.
	vpnClientMu.Lock()
	old := vpnClient
	oldUnderlay := vpnUnderlay
	vpnClient = nil
	vpnUnderlay = nil
	vpnClientMu.Unlock()
	if old != nil {
		old.Close()
	}
	if oldUnderlay != nil {
		_ = oldUnderlay.Close()
	}

	err = newClient.Setup()
	if err != nil {
		newClient.Close()
		_ = newUnderlay.Close()
		return ""
	}

	log.Printf("EasyConnect client started")

	clientIP, err := newClient.IP()
	if err != nil {
		newClient.Close()
		_ = newUnderlay.Close()
		return ""
	}

	vpnClientMu.Lock()
	vpnClient = newClient
	vpnUnderlay = newUnderlay
	vpnClientMu.Unlock()

	return clientIP.String()
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
