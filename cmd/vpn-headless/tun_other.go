//go:build !linux

package main

import "errors"

func newHeadlessTUN() (headlessTUN, error) {
	return nil, errors.New("headless 系统 TUN 仅支持 Linux")
}

func checkHeadlessProtocol(string) error { return nil }
