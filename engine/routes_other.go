//go:build !windows

package engine

import "errors"

type unsupportedRoutes struct{}

func (unsupportedRoutes) Apply(_, _ []string) (routeResult, error) {
	return routeResult{}, errors.New("当前平台 TUN 路由尚未实现")
}
