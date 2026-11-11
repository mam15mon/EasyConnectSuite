package main

import (
	"errors"
	"os"

	"EasyConnectSuite/engine"
)

func newHeadlessTUN() (headlessTUN, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("headless -tun 需要 root 权限，请通过 sudo 启动")
	}
	return engine.NewLinuxTUNController(), nil
}

func checkHeadlessProtocol(protocol string) error {
	if protocol == "topsec" && os.Geteuid() != 0 {
		return errors.New("Linux TopSAP 设备身份读取需要 root 权限，请通过 sudo 启动 headless")
	}
	return nil
}
