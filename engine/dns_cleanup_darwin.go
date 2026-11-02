package engine

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"EasyConnectSuite/vpncore/log"
	"golang.org/x/sys/unix"
)

const darwinResolverDir = "/etc/resolver"
const darwinResolverMarker = "# EasyConnectSuite TUN "

func validateDarwinResolverDir(dir string, requireRoot bool) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || (requireRoot && (stat.Uid != 0 || info.Mode().Perm()&0022 != 0)) {
		return errors.New("unsafe macOS resolver directory")
	}
	return nil
}

func darwinResolverOwner(data []byte) (int, bool, error) {
	text := string(data)
	if !strings.HasPrefix(text, darwinResolverMarker) {
		return 0, false, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) != 3 || lines[2] != "" {
		return 0, true, errors.New("invalid owned resolver content")
	}
	fields := strings.Fields(strings.TrimPrefix(lines[0], darwinResolverMarker))
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "pid=") || !strings.HasPrefix(fields[1], "token=") {
		return 0, true, errors.New("invalid owned resolver marker")
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(fields[0], "pid="))
	nonce := strings.TrimPrefix(fields[1], "token=")
	decoded, decodeErr := hex.DecodeString(nonce)
	if err != nil || pid <= 1 || decodeErr != nil || len(decoded) != 24 || hex.EncodeToString(decoded) != nonce {
		return 0, true, errors.New("invalid owned resolver identity")
	}
	if !strings.HasPrefix(lines[1], "nameserver ") {
		return 0, true, errors.New("invalid owned resolver server")
	}
	server, err := netip.ParseAddr(strings.TrimPrefix(lines[1], "nameserver "))
	if err != nil || !server.Is4() || !netip.MustParsePrefix("198.19.128.0/24").Contains(server) || server.As4()[3]%4 != 2 {
		return 0, true, errors.New("invalid owned resolver server")
	}
	return pid, true, nil
}

func darwinProcessAlive(pid int) (bool, error) {
	err := unix.Kill(pid, 0)
	if err == nil || errors.Is(err, unix.EPERM) {
		return true, nil
	}
	if errors.Is(err, unix.ESRCH) {
		return false, nil
	}
	return false, err
}

func cleanupDarwinResolvers(dir string, probe, requireRoot bool, alive func(int) (bool, error)) (pending bool, err error) {
	if err = validateDarwinResolverDir(dir, requireRoot); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !validDomain(name) {
			continue
		}
		path := filepath.Join(dir, name)
		info, e := os.Lstat(path)
		if e != nil {
			return pending, e
		}
		if !info.Mode().IsRegular() || info.Size() > 1024 {
			continue
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return pending, e
		}
		pid, owned, e := darwinResolverOwner(data)
		if e != nil {
			return pending, fmt.Errorf("resolver %s: %w", name, e)
		}
		if !owned {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (requireRoot && (stat.Uid != 0 || info.Mode().Perm()&0022 != 0)) {
			return pending, fmt.Errorf("unsafe owned resolver %s", name)
		}
		isAlive, e := alive(pid)
		if e != nil {
			return pending, fmt.Errorf("check resolver owner %d: %w", pid, e)
		}
		if isAlive {
			continue
		}
		pending = true
		if probe {
			continue
		}
		current, e := os.Lstat(path)
		if e != nil || !os.SameFile(info, current) {
			return pending, fmt.Errorf("resolver %s changed; cleanup refused", name)
		}
		contents, e := os.ReadFile(path)
		if e != nil || string(contents) != string(data) {
			return pending, fmt.Errorf("resolver %s changed; cleanup refused", name)
		}
		if e = os.Remove(path); e != nil {
			return pending, fmt.Errorf("remove stale resolver %s: %w", name, e)
		}
	}
	return pending, nil
}

func CleanupStaleDarwinTUN(probe bool) (pending bool, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d] ", os.Getpid()), "darwin_stale_cleanup", nil)
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		op.Finish(outcome, err)
	}()
	if !probe && os.Geteuid() != 0 {
		return false, errors.New("清理 macOS TUN 需要管理员权限")
	}
	op.Step("scan_owned_resolvers")
	pending, err = cleanupDarwinResolvers(darwinResolverDir, probe, true, darwinProcessAlive)
	if err == nil {
		outcome = "completed"
	}
	return
}
