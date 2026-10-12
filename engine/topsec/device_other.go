//go:build !windows

package topsec

import "errors"

func localDeviceCode(profileID string) (string, error) {
	return "", errors.New("TopSAP 设备标识目前仅实现 Windows")
}
