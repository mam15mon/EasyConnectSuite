package engine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"

	"EasyConnectSuite/vpncore/log"
	tun "github.com/mythologyli/sing-tun"
	"github.com/sagernet/netlink"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
	"golang.org/x/sys/unix"
)

type linuxTunFile struct{ *os.File }

func (t *linuxTunFile) CreateVectorisedWriter() N.VectorisedWriter {
	return bufio.NewVectorisedWriter(t.File)
}
func (t *tunIngress) dnsAddress() string { return t.dns }

// DNS settings belong to this exclusive link and disappear with its deletion.
func (r *linuxRoutes) setDNS(names []string) error {
	if r.dnsOwner == nil {
		if len(names) != 0 {
			return errors.New("Linux TUN DNS 服务未配置")
		}
		return nil
	}
	if len(names) == 0 {
		return r.dnsOwner.Close(context.Background())
	}
	return r.dnsOwner.Apply(r.dnsOwner.ctx, names)
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
	backend, _ := t.backend.(*linuxRoutes)
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q interface=%q] ", os.Getpid(), t.claimRef.Profile, t.claimRef.Session, t.claimRef.Attachment, t.name), "linux_tun_release", nil)
	outcome := "interrupted"
	var err error
	defer func() { op.Finish(outcome, err) }()
	if backend == nil || !backend.closedOnFailure {
		op.Step("close_interface")
		if err = t.device.Close(); err != nil {
			outcome = "failed"
			return err
		}
	}
	if backend != nil {
		op.Step("verify_interface_removed")
		_, lookupErr := netlink.LinkByIndex(backend.index)
		var missing netlink.LinkNotFoundError
		if lookupErr == nil {
			err = errors.New("owned TUN interface remains after close")
		} else if !errors.As(lookupErr, &missing) {
			err = fmt.Errorf("verify TUN removal: %w", lookupErr)
		}
		if err != nil {
			outcome = "failed"
			return err
		}
	}
	outcome = "completed"
	return nil
}

// StartLinuxTUNIngress owns a non-persistent TUN and, for resource namespaces,
// a private connection to the running systemd-resolved service.
func StartLinuxTUNIngress(ctx context.Context, spec TUNSpec, service TUNBackend) (*tunIngress, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if len(spec.Names) == 0 {
		return startLinuxTUNIngressWithDNS(ctx, spec, service, nil)
	}
	if os.Geteuid() != 0 {
		return nil, errors.New("Linux TUN DNS 配置需要 root Worker")
	}
	client, err := newResolvedClient(ctx, spec)
	if err != nil {
		return nil, err
	}
	ingress, err := startLinuxTUNIngressWithDNS(ctx, spec, service, client.Call)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	ingress.backend.(*linuxRoutes).dnsOwner.close = client.Close
	return ingress, nil
}

func startLinuxTUNIngressWithDNS(ctx context.Context, spec TUNSpec, service TUNBackend, call resolvedCall) (ingress *tunIngress, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "linux_tun_attach", ctx.Done())
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	if err = spec.Validate(); err != nil {
		outcome = "invalid_spec"
		return nil, err
	}
	if len(spec.Names) != 0 && call == nil {
		outcome = "dns_unsupported"
		return nil, errors.New("Linux TUN DNS 服务未配置")
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
			err = joinTUNCleanupError(err, obj.Close())
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
	if call != nil {
		backend.dnsOwner = &resolvedDNSOwner{index: index, server: obj.dns, ref: spec, call: call, ctx: attachCtx, verify: func() error {
			ifr, err := unix.NewIfreq("")
			if err != nil {
				return err
			}
			raw, err := file.SyscallConn()
			if err != nil {
				return err
			}
			var ioctlErr error
			if err = raw.Control(func(fd uintptr) { ioctlErr = unix.IoctlIfreq(int(fd), unix.TUNGETIFF, ifr) }); err != nil {
				return err
			}
			if ioctlErr != nil {
				return ioctlErr
			}
			if ifr.Name() != obj.name {
				return errors.New("TUN interface identity changed")
			}
			link, err := netlink.LinkByName(obj.name)
			if err != nil {
				return err
			}
			if link.Attrs().Index != index {
				return errors.New("TUN interface index changed")
			}
			return nil
		}}
	}
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
