package tunipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const socketPrefix = "/tmp/easyconnectsuite-tun-"

func NewName() string {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic(err)
	}
	return socketPrefix + hex.EncodeToString(nonce[:]) + "/socket"
}

func validName(name string) bool {
	if !strings.HasPrefix(name, socketPrefix) || !strings.HasSuffix(name, "/socket") {
		return false
	}
	nonce := strings.TrimSuffix(strings.TrimPrefix(name, socketPrefix), "/socket")
	decoded, err := hex.DecodeString(nonce)
	return err == nil && len(decoded) == 24 && hex.EncodeToString(decoded) == nonce
}

type Listener struct {
	listener  *net.UnixListener
	dir       string
	info      os.FileInfo
	acceptMu  sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func Listen(name string) (*Listener, error) {
	if !validName(name) {
		return nil, errors.New("invalid TUN socket name")
	}
	dir := filepath.Dir(name)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		_ = os.Remove(dir)
		return nil, err
	}
	l.SetUnlinkOnClose(false)
	info, err := os.Lstat(name)
	if err != nil {
		l.Close()
		_ = os.Remove(dir)
		return nil, err
	}
	return &Listener{listener: l, dir: dir, info: info}, nil
}

func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		l.closeErr = l.listener.Close()
		name := filepath.Join(l.dir, "socket")
		info, err := os.Lstat(name)
		if err == nil && os.SameFile(l.info, info) {
			err = os.Remove(name)
		} else if err == nil {
			err = errors.New("TUN socket identity changed")
		}
		l.closeErr = errors.Join(l.closeErr, err, os.Remove(l.dir))
	})
	return l.closeErr
}

func peerPID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var pid int
	var socketErr error
	if err = raw.Control(func(fd uintptr) { pid, socketErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID) }); err != nil {
		return 0, err
	}
	return pid, socketErr
}

func checkPeer(conn *net.UnixConn, expected uint32) error {
	if expected == 0 {
		return errors.New("missing TUN peer PID")
	}
	pid, err := peerPID(conn)
	if err != nil {
		return fmt.Errorf("read TUN peer PID: %w", err)
	}
	if pid != int(expected) {
		return fmt.Errorf("unexpected TUN peer PID")
	}
	return nil
}

func (l *Listener) Accept(ctx context.Context, expected uint32) (net.Conn, error) {
	l.acceptMu.Lock()
	defer l.acceptMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = l.listener.SetDeadline(time.Now()); close(canceled) })
	defer func() {
		if !stop() {
			<-canceled
		}
		_ = l.listener.SetDeadline(time.Time{})
	}()
	conn, err := l.listener.AcceptUnix()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err = checkPeer(conn, expected); err != nil {
		conn.Close()
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func Dial(ctx context.Context, name string, expected uint32) (net.Conn, error) {
	if !validName(name) {
		return nil, errors.New("invalid TUN socket name")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", name)
	if err != nil {
		return nil, err
	}
	if err = checkPeer(conn.(*net.UnixConn), expected); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}
