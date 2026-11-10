//go:build !windows && !darwin && !linux

package main

import "os"

func runTUNWorker([]string) error { return os.ErrInvalid }
