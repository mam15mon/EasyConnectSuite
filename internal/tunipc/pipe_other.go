//go:build !windows

package tunipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
)

type Listener struct{}

func NewName() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "unix://easyconnectsuite-tun-" + hex.EncodeToString(b[:])
}

func Listen(string) (*Listener, error) { return nil, errors.New("当前平台 TUN IPC 尚未实现") }
func (*Listener) Close() error         { return nil }
func (*Listener) Accept(context.Context, uint32) (net.Conn, error) {
	return nil, errors.New("当前平台 TUN IPC 尚未实现")
}
func Dial(string, context.Context, uint32) (net.Conn, error) {
	return nil, errors.New("当前平台 TUN IPC 尚未实现")
}
