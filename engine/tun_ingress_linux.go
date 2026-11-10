package engine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"

	"EasyConnectSuite/vpncore/log"
	tun "github.com/mythologyli/sing-tun"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
)

type linuxTunFile struct{ *os.File }

func (t *linuxTunFile) CreateVectorisedWriter() N.VectorisedWriter {
	return bufio.NewVectorisedWriter(t.File)
}
func (t *tunIngress) dnsAddress() string { return t.dns }

// Linux DNS ownership is not implemented; do not modify system DNS.
func (r *linuxRoutes) setDNS(names []string) error {
	if len(names) != 0 {
		return errors.New("Linux TUN DNS 分流尚未实现")
	}
	return nil
}
func validateIngressUpdate(t *tunIngress, spec TUNSpec) error {
	if t.spec.Ref.Attachment != "" {
		return errors.New("Linux TUN policy updates require a new interface")
	}
	return nil
}
func releaseIngressRoutes(t *tunIngress) error {
	if t.routes != nil {
		t.routes.mu.Lock()
		t.routes.closed = true
		t.routes.mu.Unlock()
	}
	if t.device == nil {
		return nil
	}
	if backend, ok := t.backend.(*linuxRoutes); ok && backend.closedOnFailure {
		return nil
	}
	op := log.BeginOperation(fmt.Sprintf("[TUN interface=%q] ", t.name), "linux_tun_release", nil)
	outcome := "interrupted"
	var err error
	defer func() { op.Finish(outcome, err) }()
	op.Step("close_interface")
	if err = t.device.Close(); err != nil {
		outcome = "failed"
		return err
	}
	outcome = "completed"
	return nil
}

// StartLinuxTUNIngress is currently exercised in an isolated namespace only.
func StartLinuxTUNIngress(ctx context.Context, spec TUNSpec, service TUNBackend) (ingress *tunIngress, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "linux_tun_attach", ctx.Done())
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	if err = spec.Validate(); err != nil {
		outcome = "invalid_spec"
		return nil, err
	}
	if len(spec.Names) != 0 {
		outcome = "dns_unsupported"
		return nil, errors.New("Linux TUN DNS 分流尚未实现")
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return nil, err
	}
	attachCtx, cancel := context.WithCancel(ctx)
	obj := &tunIngress{service: service, ctx: attachCtx, cancel: cancel}
	defer func() {
		if err != nil {
			op.Step("cleanup_failed_attach")
			err = errors.Join(err, obj.Close())
		}
	}()
	op.Step("create_device")
	var file *os.File
	file, obj.name, err = createLinuxTUN(attachCtx, spec)
	if err != nil {
		outcome = "failed"
		return
	}
	obj.device = &linuxTunFile{file}
	op.Step("configure_device")
	var index int
	index, err = configureLinuxTUN(attachCtx, file, obj.name, spec)
	if err != nil {
		obj.device = nil
		outcome = "failed"
		return
	}
	obj.prefix = fmt.Sprintf("198.19.128.%d/30", spec.Slot*4+1)
	obj.dns = fmt.Sprintf("198.19.128.%d", spec.Slot*4+2)
	backend := &linuxRoutes{device: file, name: obj.name, index: index, ref: spec}
	obj.backend = backend
	obj.routes = newRouteManager(backend)
	op.Step("create_stack")
	obj.stack, err = tun.NewStack("system", tun.StackOptions{Context: attachCtx, Tun: obj.device, Name: obj.name, MTU: 1400, Inet4Address: []netip.Prefix{netip.MustParsePrefix(obj.prefix)}, UDPTimeout: 60, Handler: obj, Logger: tunLogger{}})
	if err != nil {
		outcome = "failed"
		return
	}
	op.Step("start_stack")
	if err = obj.stack.Start(); err != nil {
		outcome = "failed"
		return
	}
	op.Step("install_routes")
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
