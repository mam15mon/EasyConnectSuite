package engine

import (
	"fmt"
	"net"
	"net/netip"
	"os"

	"EasyConnectSuite/vpncore/log"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

// linuxRoutes only creates routes on the open TUN. The kernel removes those
// routes when the owned non-persistent interface is closed. Explicit deletion
// is rejected until policy updates have a safe ownership mechanism.
type linuxRoutes struct {
	device          *os.File
	name            string
	index           int
	ref             TUNSpec
	closedOnFailure bool
	beforeAdd       func() // test seam for an external route arriving during installation
}

func (r *linuxRoutes) Apply(add, remove []string) (result routeResult, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), r.ref.Profile, r.ref.Session, r.ref.Attachment), "linux_routes", nil)
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	if len(remove) != 0 {
		outcome = "failed"
		return result, fmt.Errorf("Linux TUN 路由仅随网卡关闭移除")
	}
	if err = r.ref.Validate(); err != nil {
		outcome = "invalid_spec"
		return
	}
	if r.device == nil {
		outcome = "failed"
		return result, fmt.Errorf("TUN 文件未打开")
	}
	defer func() {
		if err != nil && len(result.Added) > 0 {
			op.Step("close_partial_interface")
			if closeErr := r.device.Close(); closeErr != nil {
				err = fmt.Errorf("%w; close TUN: %v", err, closeErr)
			} else {
				r.closedOnFailure = true
				result.Added = nil
			}
		}
	}()
	for _, value := range add {
		p, parseErr := netip.ParsePrefix(value)
		if parseErr != nil || !p.Addr().Is4() || p.Bits() == 0 || p != p.Masked() {
			outcome = "failed"
			return result, fmt.Errorf("invalid TUN route %q", value)
		}
	}
	op.Step("verify_interface")
	var ifr *unix.Ifreq
	ifr, err = unix.NewIfreq("")
	if err != nil {
		outcome = "failed"
		return
	}
	raw, rawErr := r.device.SyscallConn()
	if rawErr != nil {
		outcome = "failed"
		return result, rawErr
	}
	var ioctlErr error
	if err = raw.Control(func(fd uintptr) { ioctlErr = unix.IoctlIfreq(int(fd), unix.TUNGETIFF, ifr) }); err != nil {
		outcome = "failed"
		return
	}
	if err = ioctlErr; err != nil {
		outcome = "failed"
		return
	}
	if ifr.Name() != r.name {
		outcome = "failed"
		return result, fmt.Errorf("TUN interface identity changed")
	}
	var link netlink.Link
	link, err = netlink.LinkByName(r.name)
	if err != nil {
		outcome = "failed"
		return
	}
	if r.index <= 0 || link.Attrs().Index != r.index {
		outcome = "failed"
		return result, fmt.Errorf("TUN interface index changed")
	}
	for _, value := range add {
		_, dst, _ := net.ParseCIDR(value)
		op.Step("check_route_conflict")
		var existing []netlink.Route
		existing, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: dst, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
		if err != nil {
			outcome = "failed"
			return
		}
		if len(existing) != 0 {
			outcome = "failed"
			return result, fmt.Errorf("TUN route conflict: %s", value)
		}
		if r.beforeAdd != nil {
			r.beforeAdd()
		}
		op.Step("add_route")
		err = netlink.RouteAdd(&netlink.Route{LinkIndex: r.index, Dst: dst, Scope: netlink.SCOPE_LINK, Table: unix.RT_TABLE_MAIN})
		if err != nil {
			outcome = "failed"
			return
		}
		result.Added = append(result.Added, value)
		op.Step("verify_installed_route")
		existing, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: dst, Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
		if err != nil || len(existing) != 1 || existing[0].LinkIndex != r.index {
			if err == nil {
				err = fmt.Errorf("TUN route changed during installation: %s", value)
			}
			outcome = "failed"
			return
		}
	}
	outcome = "completed"
	return
}
