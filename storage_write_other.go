//go:build !linux

package main

import "os"

func writeProfileConfig(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
