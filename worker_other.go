//go:build !windows && !darwin

package main

import "os"

func runTUNWorker([]string) error { return os.ErrInvalid }
