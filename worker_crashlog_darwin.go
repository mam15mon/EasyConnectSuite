package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"golang.org/x/sys/unix"
)

const darwinWorkerLogDir = "/private/var/log/EasyConnectSuite"

func setupWorkerCrashLog() error { return setupDarwinWorkerCrashLog(darwinWorkerLogDir) }

func setupDarwinWorkerCrashLog(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create macOS TUN worker log directory: %w", err)
	}
	dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open macOS TUN worker log directory: %w", err)
	}
	defer unix.Close(dirFD)
	var info unix.Stat_t
	if err = unix.Fstat(dirFD, &info); err != nil {
		return err
	}
	if info.Uid != 0 || info.Mode&0077 != 0 {
		return fmt.Errorf("unsafe macOS TUN worker log directory: %s", dir)
	}
	const name = "tun-worker-crash.log"
	fd, err := unix.Openat(dirFD, name, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return fmt.Errorf("open macOS TUN worker crash log: %w", err)
	}
	defer unix.Close(fd)
	if err = unix.Fstat(fd, &info); err != nil {
		return err
	}
	if info.Uid != 0 || info.Mode&unix.S_IFMT != unix.S_IFREG || info.Mode&0077 != 0 || info.Nlink != 1 {
		return fmt.Errorf("unsafe macOS TUN worker crash log: %s", filepath.Join(dir, name))
	}
	if info.Size > 8*1024*1024 {
		if err = unix.Renameat(dirFD, name, dirFD, name+".old"); err != nil {
			return fmt.Errorf("rotate macOS TUN worker crash log: %w", err)
		}
		unix.Close(fd)
		fd, err = unix.Openat(dirFD, name, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err != nil {
			return fmt.Errorf("create macOS TUN worker crash log: %w", err)
		}
		defer unix.Close(fd)
	}
	if err = unix.Dup2(fd, 2); err != nil {
		return fmt.Errorf("redirect macOS TUN worker stderr: %w", err)
	}
	debug.SetTraceback("all")
	log.SetOutput(os.Stderr)
	_, err = fmt.Fprintf(os.Stderr, "\n=== TUN worker pid=%d started ===\n", os.Getpid())
	return err
}
