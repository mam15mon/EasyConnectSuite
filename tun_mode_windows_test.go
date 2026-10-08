package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
)

func tunModeTestApp(t *testing.T) *App {
	t.Helper()
	return &App{
		configPath: filepath.Join(t.TempDir(), "配置 with spaces.json"),
		appConfig: AppConfig{
			ActiveProfileID: "second",
			Profiles:        []Profile{{ID: "first"}, {ID: "second", Username: "user"}},
		},
	}
}

func readTUNTestConfig(t *testing.T, app *App) AppConfig {
	t.Helper()
	data, err := os.ReadFile(app.configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestTUNModeElevationSavesSelectedProfileBeforeLaunch(t *testing.T) {
	app := tunModeTestApp(t)
	launched := false
	restarting, err := app.setTUNMode(true, false, func(path string) error {
		launched = true
		if path != app.configPath {
			t.Fatal("lost the original configuration path")
		}
		config := readTUNTestConfig(t, app)
		if config.ActiveProfileID != "second" || !config.Profiles[1].TUNMode || config.Profiles[0].TUNMode {
			t.Fatal("selected profile was not persisted before elevation")
		}
		return nil
	})
	if err != nil || !restarting || !launched || !app.restarting {
		t.Fatalf("restarting=%v launched=%v err=%v", restarting, launched, err)
	}
	if got := app.StartVPN(); got != "正在以管理员身份重新启动" {
		t.Fatal("VPN startup was allowed during replacement")
	}
}

func TestTUNModeCancelledElevationRestoresMemoryAndDisk(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(strconv.FormatBool(previous), func(t *testing.T) {
			app := tunModeTestApp(t)
			app.appConfig.Profiles[1].TUNMode = previous
			cancelled := errors.New("cancelled UAC")
			restarting, err := app.setTUNMode(true, false, func(string) error { return cancelled })
			if !errors.Is(err, cancelled) || restarting || app.restarting {
				t.Fatalf("restarting=%v err=%v", restarting, err)
			}
			if app.appConfig.Profiles[1].TUNMode != previous || readTUNTestConfig(t, app).Profiles[1].TUNMode != previous {
				t.Fatal("cancelled authorization changed the saved mode")
			}
		})
	}
}

func TestTUNModeAlreadyElevatedOrDisabledDoesNotLaunch(t *testing.T) {
	for _, scenario := range []struct{ enabled, elevated bool }{{true, true}, {false, false}} {
		app := tunModeTestApp(t)
		restarting, err := app.setTUNMode(scenario.enabled, scenario.elevated, func(string) error {
			t.Fatal("unnecessary administrator relaunch")
			return nil
		})
		if err != nil || restarting || readTUNTestConfig(t, app).Profiles[1].TUNMode != scenario.enabled {
			t.Fatalf("restarting=%v err=%v", restarting, err)
		}
	}
}

func TestTUNModeCannotChangeDuringConnectionOrRestart(t *testing.T) {
	for _, state := range []string{"running", "starting", "restarting"} {
		t.Run(state, func(t *testing.T) {
			app := tunModeTestApp(t)
			app.status.Running = state == "running"
			app.starting = state == "starting"
			app.restarting = state == "restarting"
			_, err := app.setTUNMode(true, false, func(string) error { t.Fatal("launched while busy"); return nil })
			if err == nil || app.appConfig.Profiles[1].TUNMode {
				t.Fatal("mode changed while busy")
			}
			if _, err := os.Stat(app.configPath); !os.IsNotExist(err) {
				t.Fatal("wrote configuration while busy")
			}
		})
	}
}

func TestTUNModeSaveFailureDoesNotLaunchOrChangeMemory(t *testing.T) {
	app := tunModeTestApp(t)
	app.configPath = filepath.Join(t.TempDir(), "missing", "config.json")
	_, err := app.setTUNMode(true, false, func(string) error { t.Fatal("launched without saving"); return nil })
	if err == nil || app.appConfig.Profiles[1].TUNMode {
		t.Fatal("failed persistence still changed mode")
	}
}

func TestTUNRestartRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--tun-restart"},
		{"--tun-restart", "bad", `C:\config.json`},
		{"--tun-restart", "0", `C:\config.json`},
		{"--tun-restart", strconv.Itoa(os.Getpid()), `C:\config.json`},
		{"--tun-restart", "1", "relative.json"},
	} {
		if _, err := prepareTUNRestart(args); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
	if path, err := prepareTUNRestart(nil); path != "" || err != nil {
		t.Fatal("changed ordinary startup")
	}
}

func TestTUNRestartWaitsForOriginalProcess(t *testing.T) {
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()
	if err := waitForTUNParent(uint32(command.Process.Pid)); err != nil {
		t.Fatal(err)
	}
}

func TestTUNRestartArgumentsPreserveUnicodeAndSpaces(t *testing.T) {
	path := `C:\Users\admin\Desktop\脚本 项目\vpn_profiles.json`
	args := []string{"--tun-restart", "123", path}
	parsed, err := windows.DecomposeCommandLine(windows.ComposeCommandLine(args))
	if err != nil || len(parsed) != 3 || parsed[2] != path {
		t.Fatalf("argument roundtrip failed: %v %v", parsed, err)
	}
}
