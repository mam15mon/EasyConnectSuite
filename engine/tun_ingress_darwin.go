package engine

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	"EasyConnectSuite/vpncore/log"
	tun "github.com/mythologyli/sing-tun"
)

func StartDarwinTUNIngress(ctx context.Context, spec TUNSpec, service TUNBackend) (ingress *tunIngress, err error) {
	operation := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "darwin_tun_attach", ctx.Done())
	outcome := "interrupted"
	defer func() { operation.Finish(outcome, err) }()
	attachCtx, cancel := context.WithCancel(ctx)
	obj := &tunIngress{service: service, ctx: attachCtx, cancel: cancel}
	defer func() {
		if err != nil {
			operation.Step("cleanup_failed_attach")
			err = joinTUNCleanupError(err, obj.Close())
		}
	}()
	operation.Step("create_device")
	obj.device, obj.name, err = createDarwinTUN(attachCtx, spec)
	if err != nil {
		outcome = "failed"
		return
	}
	obj.prefix = fmt.Sprintf("198.19.128.%d/30", spec.Slot*4+1)
	obj.dns = fmt.Sprintf("198.19.128.%d", spec.Slot*4+2)
	backend := &darwinRoutes{name: obj.name, ref: spec, dns: obj.dns}
	obj.backend = backend
	obj.routes = newRouteManager(backend)
	operation.Step("create_stack")
	obj.stack, err = tun.NewStack("system", tun.StackOptions{Context: attachCtx, Tun: obj.device, Name: obj.name, MTU: 1400, Inet4Address: []netip.Prefix{netip.MustParsePrefix(obj.prefix)}, UDPTimeout: 60, Handler: obj, Logger: tunLogger{}})
	if err != nil {
		outcome = "failed"
		return
	}
	operation.Step("start_stack")
	if err = obj.stack.Start(); err != nil {
		outcome = "failed"
		return
	}
	operation.Step("install_routes")
	if err = obj.update(spec, nil); err != nil {
		outcome = "failed"
		return
	}
	if err = attachCtx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	outcome = "attached"
	return obj, nil
}
