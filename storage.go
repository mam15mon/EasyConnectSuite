package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func applicationDataDir() string {
	return filepath.Join(applicationStorageRoot(), "VPN")
}

func applicationLogDir() string {
	return filepath.Join(applicationDataDir(), "logs")
}

// Rename the whole directory on the same volume so profiles and logs move together.
func migrateLegacyStorage() error {
	legacy := filepath.Join(applicationStorageRoot(), "EasyConnectVPN")
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("检查旧 VPN 存储目录失败: %w", err)
	}
	if _, err := os.Stat(applicationDataDir()); err == nil {
		return fmt.Errorf("旧目录 %s 与新目录 %s 同时存在，请先合并配置和日志后移除旧目录；程序未覆盖任何文件", legacy, applicationDataDir())
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查新 VPN 存储目录失败: %w", err)
	}
	if err := os.Rename(legacy, applicationDataDir()); err != nil {
		return fmt.Errorf("迁移 VPN 配置和日志失败，请先退出旧版本 VPN 后重试: %w", err)
	}
	return nil
}
