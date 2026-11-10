package engine

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	"EasyConnectSuite/vpncore/log"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

// createLinuxTUN creates an exclusive, non-persistent interface. The returned
// file owns the interface; closing it removes it without touching routes or DNS.
func createLinuxTUN(ctx context.Context, spec TUNSpec) (file *os.File, name string, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "linux_tun_create", ctx.Done())
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	if err = spec.Validate(); err != nil {
		outcome = "invalid_spec"
		return
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	op.Step("open_device")
	var fd int
	fd, err = unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		outcome = "failed"
		return
	}
	file = os.NewFile(uintptr(fd), "linux-tun")
	defer func() {
		if err != nil {
			op.Step("cleanup_failed_create")
			if closeErr := file.Close(); closeErr != nil {
				err = fmt.Errorf("%w; close TUN: %v", err, closeErr)
			}
			file = nil
		}
	}()
	var ifr *unix.Ifreq
	ifr, err = unix.NewIfreq("ecs%d")
	if err != nil {
		outcome = "failed"
		return
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI | unix.IFF_TUN_EXCL)
	op.Step("create_interface")
	if err = unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		outcome = "failed"
		return
	}
	name = ifr.Name()
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	outcome = "created"
	return
}

// configureLinuxTUN configures only the owned interface and leaves routing to
// the route manager. On failure it closes the non-persistent interface.
func configureLinuxTUN(ctx context.Context, file *os.File, name string, spec TUNSpec) (index int, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "linux_tun_configure", ctx.Done())
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	if file == nil {
		outcome = "failed"
		return 0, fmt.Errorf("TUN 文件未打开")
	}
	defer func() {
		if err != nil {
			op.Step("close_failed_interface")
			if closeErr := file.Close(); closeErr != nil {
				err = fmt.Errorf("%w; close TUN: %v", err, closeErr)
			}
			index = 0
		}
	}()
	if err = spec.Validate(); err != nil {
		outcome = "invalid_spec"
		return
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	op.Step("verify_interface")
	var ifr *unix.Ifreq
	ifr, err = unix.NewIfreq("")
	if err != nil {
		outcome = "failed"
		return
	}
	raw, rawErr := file.SyscallConn()
	if rawErr != nil {
		outcome = "failed"
		err = rawErr
		return
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
	if ifr.Name() != name {
		outcome = "failed"
		err = fmt.Errorf("TUN interface identity changed")
		return
	}
	var link netlink.Link
	link, err = netlink.LinkByName(name)
	if err != nil {
		outcome = "failed"
		return
	}
	index = link.Attrs().Index
	if index <= 0 {
		outcome = "failed"
		err = fmt.Errorf("invalid TUN interface index")
		return
	}
	op.Step("set_mtu")
	if err = netlink.LinkSetMTU(link, 1400); err != nil {
		outcome = "failed"
		return
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	op.Step("set_address")
	address := netip.MustParsePrefix(fmt.Sprintf("198.19.128.%d/30", spec.Slot*4+1))
	var addr *netlink.Addr
	addr, err = netlink.ParseAddr(address.String())
	if err != nil {
		outcome = "failed"
		return
	}
	addr.Flags = unix.IFA_F_NOPREFIXROUTE
	if err = netlink.AddrAdd(link, addr); err != nil {
		outcome = "failed"
		return
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	op.Step("activate_interface")
	if err = netlink.LinkSetUp(link); err != nil {
		outcome = "failed"
		return
	}
	if err = ctx.Err(); err != nil {
		outcome = "cancelled"
		return
	}
	outcome = "configured"
	return
}
