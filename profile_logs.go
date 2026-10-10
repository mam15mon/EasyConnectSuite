package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetProfileLogs returns the latest 200 scoped entries from both log generations.
func (a *App) GetProfileLogs(id string) ([]string, error) {
	a.mu.Lock()
	known := false
	for _, p := range a.appConfig.Profiles {
		if p.ID == id {
			known = true
			break
		}
	}
	a.mu.Unlock()
	if !known {
		return nil, fmt.Errorf("服务器配置不存在")
	}
	marker := fmt.Sprintf("profile=%q", id)
	path := filepath.Join(os.Getenv("APPDATA"), "EasyConnectVPN", "logs", "engine.log")
	lines := make([]string, 0, 200)
	for _, filename := range []string{path + ".old", path} {
		file, err := os.Open(filename)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("读取连接日志失败: %w", err)
		}
		scan := bufio.NewScanner(file)
		scan.Buffer(make([]byte, 4096), 64<<10)
		for scan.Scan() {
			line := scan.Text()
			if !strings.Contains(line, marker) {
				continue
			}
			if len(lines) == 200 {
				copy(lines, lines[1:])
				lines[199] = line
			} else {
				lines = append(lines, line)
			}
		}
		err = scan.Err()
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("读取连接日志失败: %w", err)
		}
	}
	return lines, nil
}
