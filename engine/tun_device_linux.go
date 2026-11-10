package engine

import (
	"context"
	"fmt"
	"os"

	"EasyConnectSuite/vpncore/log"
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
