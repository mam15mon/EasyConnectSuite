package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestDarwinCanceledAuthorizationWaitsForProcessExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := newTUNManager()
	var stdin io.WriteCloser
	launches := 0
	manager.launch = func(string) (*exec.Cmd, error) {
		launches++
		cmd := exec.Command("/bin/cat")
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		if err = cmd.Start(); err != nil {
			stdin.Close()
			return nil, err
		}
		cancel()
		return cmd, nil
	}
	if err := manager.ensure(ctx); err == nil {
		t.Fatal("canceled authorization succeeded")
	}
	t.Cleanup(func() { _ = stdin.Close() })
	if err := manager.ensure(context.Background()); err == nil || launches != 1 {
		t.Fatalf("new authorization started before prior process exit: %v, launches=%d", err, launches)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		manager.mu.Lock()
		recoveryErr := manager.recoveryErr
		manager.mu.Unlock()
		if recoveryErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("authorization process exit did not clear wait: %v", recoveryErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDarwinTUNAvailabilityAndExistingPreferences(t *testing.T) {
	app := &App{configPath: filepath.Join(t.TempDir(), "profiles.json")}
	if !app.GetPlatformInfo().TUNSupported {
		t.Fatal("macOS TUN control is disabled")
	}
	app.initDefaultConfig()
	if profile := app.appConfig.Profiles[0]; !profile.TUNMode || !profile.DisableProxy {
		t.Fatalf("new macOS profile does not default to TUN: %+v", profile)
	}
	const saved = `{"activeProfileId":"existing","profiles":[{"id":"existing","tunMode":false,"disableProxy":false}]}`
	if err := os.WriteFile(app.configPath, []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	app.loadConfigFile()
	if profile := app.appConfig.Profiles[0]; profile.ID != "existing" || profile.TUNMode || profile.DisableProxy {
		t.Fatalf("existing profile preferences changed: %+v", profile)
	}
}

// Runs on disposable CI to exercise the AppleScript timeout without a GUI prompt.
func TestDarwinAuthorizationScriptLongRunning(t *testing.T) {
	if os.Getenv("ECS_DARWIN_LONG_SCRIPT_TEST") != "1" {
		t.Skip("requires the long-running macOS system test")
	}
	original := `do shell script (quoted form of (item 1 of argv) & " --tun-worker " & quoted form of (item 2 of argv) & " " & quoted form of (item 3 of argv)) with administrator privileges`
	if !strings.Contains(darwinWorkerScript, original) {
		t.Fatal("authorization script no longer matches system test")
	}
	script := strings.Replace(darwinWorkerScript, original, `do shell script "/bin/sleep 125"`, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("long-running authorization script failed: %s: %v", out, err)
	}
}
