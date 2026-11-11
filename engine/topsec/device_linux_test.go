//go:build linux && !android

package topsec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	vpnlog "EasyConnectSuite/vpncore/log"
	"golang.org/x/sys/unix"
)

func TestLinuxHardwareIdentityOrderAndRedaction(t *testing.T) {
	for _, diskOK := range []bool{true, false} {
		t.Run(fmt.Sprint(diskOK), func(t *testing.T) {
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(previous)
			op := vpnlog.BeginOperation("[synthetic] ", "linux_device_identity", nil)
			defer op.Finish("tested", nil)
			var calls []string
			readFile := func(path string) ([]byte, error) {
				calls = append(calls, path)
				switch path {
				case "/etc/mtab":
					return []byte("/dev/ignore1 /boot ext4 rw 0 0\n/dev/sda12 / ext4 rw 0 0\n"), nil
				case "/sys/class/dmi/id/product_uuid":
					return []byte("00000000-1111-2222-3333-444444444444\n"), nil
				default:
					t.Fatal("unexpected hardware source", path)
					return nil, os.ErrNotExist
				}
			}
			readDisk := func(path string) ([]byte, error) {
				calls = append(calls, path)
				if path != "/dev/sda" {
					t.Fatal("partition was not removed")
				}
				if !diskOK {
					return nil, &os.PathError{Op: "open", Path: "private-device-canary", Err: os.ErrPermission}
				}
				data := make([]byte, 512)
				copy(data[20:], " a b c ")
				return data, nil
			}
			code, err := linuxHardwareDeviceCode(context.Background(), op, readFile, readDisk)
			want := "9ZgAH2EHpqCdTiEgY662wA=="
			wantCalls := []string{"/etc/mtab", "/dev/sda"}
			if !diskOK {
				want = "0kHjlf0il4PC1hgCAOqddA=="
				wantCalls = append(wantCalls, "/sys/class/dmi/id/product_uuid")
			}
			if err != nil || code != want || strings.Join(calls, ",") != strings.Join(wantCalls, ",") {
				t.Fatalf("hardware source/order or independent vector mismatch: %v calls=%v", err, calls)
			}
			for _, secret := range []string{code, " a b c ", "private-device-canary", "00000000-1111-2222-3333-444444444444"} {
				if strings.Contains(output.String(), secret) {
					t.Fatal("identity leaked in diagnostics")
				}
			}
			if !strings.Contains(output.String(), "stage=derive_device_code") {
				t.Fatal("missing identity progress")
			}
		})
	}
}

func TestLinuxHardwareIdentityFailureAndCancellation(t *testing.T) {
	for _, cancelAfterDisk := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		op := vpnlog.BeginOperation("[synthetic] ", "linux_device_identity", ctx.Done())
		dmiReads := 0
		code, err := linuxHardwareDeviceCode(ctx, op, func(path string) ([]byte, error) {
			if path == "/etc/mtab" {
				return []byte("/dev/sda2 / ext4 rw 0 0\n"), nil
			}
			if path != "/sys/class/dmi/id/product_uuid" {
				t.Fatal("unexpected fallback source")
			}
			dmiReads++
			return nil, os.ErrNotExist
		}, func(string) ([]byte, error) {
			if cancelAfterDisk {
				cancel()
			}
			return nil, os.ErrPermission
		})
		op.Finish("tested", err)
		cancel()
		if code != "" || err == nil {
			t.Fatal("missing identity produced a device code")
		}
		if cancelAfterDisk {
			if !errors.Is(err, context.Canceled) || dmiReads != 0 {
				t.Fatal("canceled identity continued reading hardware")
			}
		} else if !errors.Is(err, os.ErrNotExist) || dmiReads != 1 {
			t.Fatal("hardware failure lost its cause")
		}
	}
}

func TestLinuxDeviceParsingKeepsObservedLimits(t *testing.T) {
	for _, test := range []struct{ mounts, disk string }{
		{"/dev/sda12 / ext4 rw 0 0\n", "/dev/sda"},
		{"/dev/nvme0n1p2 / ext4 rw 0 0\n", "/dev/nvme0n1p"},
		{"/dev/mapper/root / ext4 rw 0 0\n", "/dev/mapper/root"},
	} {
		got, err := linuxRootDisk([]byte(test.mounts))
		if err != nil || got != test.disk {
			t.Fatalf("native root device mapping: %q %v", got, err)
		}
	}
	for _, mounts := range []string{"", "overlay / overlay rw 0 0\n", "/dev/sda1 /boot ext4 rw 0 0\n"} {
		if _, err := linuxRootDisk([]byte(mounts)); err == nil {
			t.Fatal("unsupported root source accepted")
		}
	}
	for _, data := range [][]byte{nil, make([]byte, 511), make([]byte, 512), bytes.Repeat([]byte{'a'}, 512)} {
		if _, err := linuxDiskSerial(data); err == nil {
			t.Fatal("invalid disk identity accepted")
		}
	}
}

func installDevicePKExec(t *testing.T, body string) {
	t.Helper()
	directory := t.TempDir()
	program := "#!/bin/sh\n[ \"$#\" -eq 2 ] && [ \"$1\" = \"$ECS_TEST_DEVICE_EXE\" ] && [ \"$2\" = --topsec-device-code ] || exit 72\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(directory, "pkexec"), []byte(program), 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ECS_TEST_DEVICE_EXE", executable)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestLinuxPrivilegedDeviceResultValidation(t *testing.T) {
	for _, test := range []struct {
		body  string
		valid bool
	}{
		{"printf '%s\\n' '9ZgAH2EHpqCdTiEgY662wA=='", true},
		{"printf '%s\\n' 'private-output-canary'", false},
		{"printf '%s\\n' '9ZgAH2EHpqCdTiEgY662wA==' 'extra'", false},
		{"printf '%s' '9ZgAH2EHpqCdTiEgY662wA=='", false},
		{"printf '%s\\n' '!!!!!!!!!!!!!!!!!!!!!!!!'", false},
		{"exit 126", false},
	} {
		t.Run(test.body, func(t *testing.T) {
			installDevicePKExec(t, test.body)
			op := vpnlog.BeginOperation("[synthetic] ", "linux_device_identity", nil)
			code, err := privilegedLinuxDeviceCode(context.Background(), op)
			op.Finish("tested", err)
			if (err == nil) != test.valid {
				t.Fatalf("helper validation: %v", err)
			}
			if !test.valid && (code != "" || strings.Contains(err.Error(), "private-output-canary")) {
				t.Fatal("invalid helper result escaped")
			}
		})
	}
}

// The helper process intentionally simulates an uninterruptible hardware read.
// Its control listener must still return so main can terminate the process.
func TestLinuxDeviceHelperProcess(t *testing.T) {
	mode := os.Getenv("ECS_TEST_DEVICE_PROCESS")
	if mode != "helper" && mode != "authorization" {
		return
	}
	if mode == "authorization" {
		// Model pkexec's authorization wait: no reader is consuming stdin.
		if err := os.WriteFile(os.Getenv("ECS_TEST_DEVICE_PID"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(73)
		}
		time.Sleep(time.Hour)
		os.Exit(76)
	}
	err := serveLinuxDeviceCode(os.Stdin, os.Stdout, func(context.Context) (string, error) {
		if err := os.WriteFile(os.Getenv("ECS_TEST_DEVICE_PID"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(73)
		}
		select {}
	})
	if !errors.Is(err, context.Canceled) {
		os.Exit(74)
	}
	os.Exit(0)
}

func TestLinuxDeviceParentProcess(t *testing.T) {
	if os.Getenv("ECS_TEST_DEVICE_PROCESS") != "parent" {
		return
	}
	os.Setenv("ECS_TEST_DEVICE_PROCESS", "helper")
	op := vpnlog.BeginOperation("[synthetic-parent] ", "linux_device_identity", nil)
	_, _ = privilegedLinuxDeviceCode(context.Background(), op)
	os.Exit(75)
}

func waitDeviceHelperPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(string(data))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper did not start")
	return 0
}

func TestLinuxDeviceHelperCancellation(t *testing.T) {
	for _, mode := range []string{"helper", "authorization"} {
		t.Run(mode, func(t *testing.T) { testLinuxDeviceHelperCancellation(t, mode) })
	}
}

func testLinuxDeviceHelperCancellation(t *testing.T, mode string) {
	t.Helper()
	installDevicePKExec(t, "exec \"$1\" -test.run '^TestLinuxDeviceHelperProcess$'")
	pidPath := filepath.Join(t.TempDir(), "pid")
	t.Setenv("ECS_TEST_DEVICE_PROCESS", mode)
	t.Setenv("ECS_TEST_DEVICE_PID", pidPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	op := vpnlog.BeginOperation("[synthetic] ", "linux_device_identity", ctx.Done())
	done := make(chan error, 1)
	go func() { _, err := privilegedLinuxDeviceCode(ctx, op); done <- err }()
	pid := waitDeviceHelperPID(t, pidPath)
	cancel()
	select {
	case err := <-done:
		op.Finish("tested", err)
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost", err)
		}
	case <-time.After(5 * time.Second):
		unix.Kill(pid, unix.SIGKILL)
		t.Fatal("canceled helper remained alive")
	}
	if err := unix.Kill(pid, 0); !errors.Is(err, unix.ESRCH) {
		t.Fatal("helper was not reaped", err)
	}
}

func TestLinuxDeviceHelperStopsAfterParentDeath(t *testing.T) {
	// Adopt the orphan in this test process so it can be explicitly reaped.
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	defer unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 0, 0, 0, 0)
	installDevicePKExec(t, "exec \"$1\" -test.run '^TestLinuxDeviceHelperProcess$'")
	pidPath := filepath.Join(t.TempDir(), "pid")
	t.Setenv("ECS_TEST_DEVICE_PROCESS", "parent")
	t.Setenv("ECS_TEST_DEVICE_PID", pidPath)
	cmd := exec.Command(os.Args[0], "-test.run", "^TestLinuxDeviceParentProcess$")
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	pid := waitDeviceHelperPID(t, pidPath)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status unix.WaitStatus
		got, err := unix.Wait4(pid, &status, unix.WNOHANG, nil)
		if err != nil {
			t.Fatal("orphan wait failed", err)
		}
		if got == pid {
			if !status.Exited() || status.ExitStatus() != 0 {
				t.Fatal("orphan did not exit through cancellation")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	unix.Kill(pid, unix.SIGKILL)
	unix.Wait4(pid, nil, 0, nil)
	t.Fatal("helper did not stop after parent death")
}

func TestLinuxDeviceHelperArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--other"}} {
		if handled, err := RunDeviceCodeHelper(args); handled || err != nil {
			t.Fatal("unrelated entry point handled")
		}
	}
	if handled, err := RunDeviceCodeHelper([]string{linuxDeviceCodeFlag, "/arbitrary/path"}); !handled || err == nil {
		t.Fatal("helper accepted external identity input")
	}
	if os.Geteuid() != 0 {
		if handled, err := RunDeviceCodeHelper([]string{linuxDeviceCodeFlag}); !handled || err == nil {
			t.Fatal("unprivileged helper accepted")
		}
	}
}
