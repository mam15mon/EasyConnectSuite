//go:build !windows && !darwin && (!linux || android)

package topsec

import (
	"context"
	"errors"
)

func localDeviceCode(ctx context.Context, profileID string) (string, error) {
	return "", errors.New("TopSAP 设备标识不支持当前平台")
}
