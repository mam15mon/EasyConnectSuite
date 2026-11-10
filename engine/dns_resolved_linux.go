package engine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
	"time"

	"EasyConnectSuite/vpncore/log"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// The resolved API applies settings to a single interface index. Callers must
// keep that interface alive and verify its identity before applying or reverting.
type resolvedAddress struct {
	Family  int32
	Address []byte
}
type resolvedDomain struct {
	Name      string
	RouteOnly bool
}
type resolvedCall func(context.Context, string, ...any) error

func resolvedDNSArguments(server string, names []string) ([]resolvedAddress, []resolvedDomain, error) {
	ip, err := netip.ParseAddr(server)
	if err != nil || !ip.Is4() {
		return nil, nil, fmt.Errorf("invalid TUN DNS server")
	}
	seen := make(map[string]bool)
	domains := make([]resolvedDomain, 0, len(names))
	for _, raw := range names {
		name := strings.TrimPrefix(strings.ToLower(raw), ".")
		if !validDomain(name) {
			return nil, nil, fmt.Errorf("invalid TUN DNS namespace")
		}
		if !seen[name] {
			seen[name] = true
			domains = append(domains, resolvedDomain{Name: name, RouteOnly: true})
		}
	}
	sort.Slice(domains, func(i, j int) bool { return domains[i].Name < domains[j].Name })
	return []resolvedAddress{{Family: unix.AF_INET, Address: ip.AsSlice()}}, domains, nil
}

// resolvedLinkDNS encapsulates D-Bus encoding. Its caller owns rollback and
// must not call it for an interface whose identity is no longer confirmed.
func resolvedLinkDNS(ctx context.Context, call resolvedCall, index int, server string, names []string) error {
	if index <= 0 {
		return fmt.Errorf("invalid TUN interface index")
	}
	addresses, domains, err := resolvedDNSArguments(server, names)
	if err != nil {
		return err
	}
	if err = call(ctx, "SetLinkDNS", int32(index), addresses); err != nil {
		return fmt.Errorf("set link DNS: %w", err)
	}
	if err = call(ctx, "SetLinkDomains", int32(index), domains); err != nil {
		return fmt.Errorf("set link domains: %w", err)
	}
	return nil
}

func resolvedSystemCall(ctx context.Context, method string, args ...any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	obj := conn.Object("org.freedesktop.resolve1", dbus.ObjectPath("/org/freedesktop/resolve1"))
	return obj.CallWithContext(ctx, "org.freedesktop.resolve1.Manager."+method, dbus.FlagNoAutoStart, args...).Err
}

// resolvedDNSOwner tracks attempted changes until RevertLink succeeds. The
// caller must retain the TUN device and provide an identity check each time.
type resolvedDNSOwner struct {
	index   int
	server  string
	ref     TUNSpec
	call    resolvedCall
	verify  func() error
	touched bool
}

func (d *resolvedDNSOwner) Apply(ctx context.Context, names []string) (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), d.ref.Profile, d.ref.Session, d.ref.Attachment), "linux_dns_apply", ctx.Done())
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		op.Finish(outcome, err)
	}()
	if d.index <= 0 || d.call == nil || d.verify == nil {
		return fmt.Errorf("invalid DNS owner")
	}
	if _, _, err = resolvedDNSArguments(d.server, names); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	op.Step("verify_interface")
	if err = d.verify(); err != nil {
		return err
	}
	op.Step("configure_link_dns")
	// A timed-out D-Bus call may still have changed the service's state.
	d.touched = true
	if err = resolvedLinkDNS(ctx, d.call, d.index, d.server, names); err != nil {
		op.Step("rollback_link_dns")
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = errors.Join(err, d.revert(rollbackCtx))
		return err
	}
	outcome = "configured"
	return nil
}

func (d *resolvedDNSOwner) revert(ctx context.Context) error {
	if !d.touched {
		return nil
	}
	if err := d.verify(); err != nil {
		return fmt.Errorf("verify DNS interface: %w", err)
	}
	if err := d.call(ctx, "RevertLink", int32(d.index)); err != nil {
		return fmt.Errorf("revert link DNS: %w", err)
	}
	d.touched = false
	return nil
}

func (d *resolvedDNSOwner) Close(ctx context.Context) (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), d.ref.Profile, d.ref.Session, d.ref.Attachment), "linux_dns_release", ctx.Done())
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		op.Finish(outcome, err)
	}()
	op.Step("revert_link_dns")
	if err = d.revert(ctx); err != nil {
		return err
	}
	outcome = "released"
	return nil
}
