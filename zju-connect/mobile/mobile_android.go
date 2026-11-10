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
var tunnelMu sync.Mutex
var activeTunnel *tun.Stack
var loginRejected atomic.Bool
var loginGeneration uint64
var logoutGeneration uint64
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
	op := log.BeginOperation(fmt.Sprintf("[android pid=%d] ", os.Getpid()), "android_logout", nil)
	defer op.Finish("success", nil)
	op.Step("acquire_client_lock")
	vpnClientMu.Lock()
	loginGeneration++
	logoutGeneration++
	if loginCancelled != nil {
		close(loginCancelled)
		loginCancelled = nil
	}
	oldClient, oldUnderlay := vpnClient, vpnUnderlay
	vpnClient, vpnUnderlay = nil, nil
	vpnClientMu.Unlock()
	op.Step("close_client")
	if oldClient != nil {
		oldClient.Close()
	}
	op.Step("close_underlay")
	if oldUnderlay != nil {
		_ = oldUnderlay.Close()
	}
}

func LoginRejected() bool {
	return loginRejected.Load()
}

func login(server string, username string, password string, totpSecret string) string {
	vpnClientMu.Lock()
	epoch := logoutGeneration
	vpnClientMu.Unlock()
	return loginAtEpoch(server, username, password, totpSecret, epoch)
}

func loginAtEpoch(server string, username string, password string, totpSecret string, epoch uint64) string {
	waitOp := log.BeginOperation(fmt.Sprintf("[android pid=%d login_epoch=%d] ", os.Getpid(), epoch), "android_login_wait", nil)
	waitOp.Step("acquire_login_lock")
	loginMu.Lock()
	defer loginMu.Unlock()
	vpnClientMu.Lock()
	if epoch != logoutGeneration {
		vpnClientMu.Unlock()
		waitOp.Finish("cancelled", context.Canceled)
		return ""
	}
	waitOp.Finish("success", nil)
	loginRejected.Store(false)
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
	if errors.Is(err, easyconnect.ErrLoginRejected) {
		loginRejected.Store(true)
	}
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

func StopStack() {
	op := log.BeginOperation(fmt.Sprintf("[android pid=%d] ", os.Getpid()), "android_tun_stop", nil)
	defer op.Finish("success", nil)
	op.Step("acquire_tunnel_lock")
	tunnelMu.Lock()
	stack := activeTunnel
	tunnelMu.Unlock()
	op.Step("close_tun")
	if stack != nil {
		if err := stack.Stop(); err != nil {
			log.Printf("[android pid=%d] android_tun_stop error_type=%T", os.Getpid(), err)
		}
	}
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

	if err := vpnTUNStack.SetupTun(fd); err != nil {
		log.Printf("tun descriptor setup failed: %v", err)
		return
	}

	vpnClientMu.Lock()
	if vpnClient != client {
		vpnClientMu.Unlock()
		_ = vpnTUNStack.Stop()
		return
	}
	tunnelMu.Lock()
	activeTunnel = vpnTUNStack
	tunnelRunning.Store(true)
	tunnelMu.Unlock()
	vpnClientMu.Unlock()
	defer func() {
		finishTunnel(vpnTUNStack)
	}()
	defer func() {
		// The read loops return on tunnel death; a residual panic must
		// not take the whole Android app down.
		if r := recover(); r != nil {
			log.Printf("tun stack stopped: %v", r)
		}
	}()
	vpnTUNStack.Run()
}

// Only the active tunnel may clear the shared running state.
func finishTunnel(stack *tun.Stack) {
	tunnelMu.Lock()
	defer tunnelMu.Unlock()
	if activeTunnel == stack {
		activeTunnel = nil
		tunnelRunning.Store(false)
	}
}
