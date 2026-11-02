package engine

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	"EasyConnectSuite/vpncore/log"
	tun "github.com/mythologyli/sing-tun"
)

func createDarwinTUN(ctx context.Context, spec TUNSpec) (device tun.Tun, name string, err error) {
	operation := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "darwin_tun_create", ctx.Done())
	outcome := "interrupted"
	defer func() { operation.Finish(outcome, err) }()
	if err = spec.Validate(); err != nil {
		outcome = "invalid_spec"
		return
	}
	if os.Geteuid() != 0 {
		err = fmt.Errorf("创建 macOS TUN 需要管理员权限")
		outcome = "permission_denied"
		return
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	operation.Step("select_interface")
	name = tun.CalculateInterfaceName("")
	if name == "" {
		err = fmt.Errorf("无法选择 macOS TUN 网卡名称")
		outcome = "failed"
		return
	}
	address := netip.MustParsePrefix(fmt.Sprintf("198.19.128.%d/30", spec.Slot*4+1))
	operation.Step("create_utun")
	device, err = tun.New(tun.Options{Name: name, MTU: 1400, Inet4Address: []netip.Prefix{address}, AutoRoute: false})
	if err != nil {
		outcome = "failed"
		return
	}
	if err = ctx.Err(); err != nil {
		operation.Step("cancel_cleanup")
		if closeErr := device.Close(); closeErr != nil {
			err = fmt.Errorf("%w; close TUN: %v", err, closeErr)
		}
		device = nil
		outcome = "cancelled"
		return
	}
	outcome = "created"
	return
}
