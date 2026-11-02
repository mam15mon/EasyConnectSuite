package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDarwinWorkerAuthorizationScriptCompiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/osacompile", "-o", filepath.Join(t.TempDir(), "worker.scpt"), "-e", darwinWorkerScript).CombinedOutput()
	if err != nil {
		t.Fatalf("compile worker authorization AppleScript: %s: %v", out, err)
	}
}
