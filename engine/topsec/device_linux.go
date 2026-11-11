//go:build linux && !android

package topsec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	vpnlog "EasyConnectSuite/vpncore/log"
	"golang.org/x/sys/unix"
)

const linuxDeviceCodeFlag = "--topsec-device-code"

func localDeviceCode(ctx context.Context, profileID string) (code string, err error) {
	op := vpnlog.BeginOperation(fmt.Sprintf("[TopSAP pid=%d profile=%q] ", os.Getpid(), profileID), "linux_device_identity", ctx.Done())
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		if errors.Is(err, context.Canceled) {
			outcome = "canceled"
		}
		op.Finish(outcome, err)
	}()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if os.Geteuid() == 0 {
		code, err = linuxHardwareDeviceCode(ctx, op, os.ReadFile, readLinuxDiskIdentity)
	} else {
		code, err = privilegedLinuxDeviceCode(ctx, op)
	}
	if err == nil {
		outcome = "completed"
	}
	return code, err
}

// RunDeviceCodeHelper is called before normal application/log initialization.
// Only this exact operation is accepted; hardware paths cannot be supplied.
func RunDeviceCodeHelper(args []string) (bool, error) {
	if len(args) == 0 || args[0] != linuxDeviceCodeFlag {
		return false, nil
	}
	if len(args) != 1 {
		return true, errors.New("TopSAP 设备标识辅助入口不接受额外参数")
	}
	if os.Geteuid() != 0 {
		return true, errors.New("TopSAP 设备标识辅助入口需要管理员权限")
	}
	log.SetOutput(os.Stderr)
	return true, serveLinuxDeviceCode(os.Stdin, os.Stdout, func(ctx context.Context) (string, error) {
		return localDeviceCode(ctx, "privileged-helper")
	})
}

func serveLinuxDeviceCode(input io.Reader, output io.Writer, read func(context.Context) (string, error)) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The parent holds this private pipe open. EOF also covers parent death.
	go func() { var b [1]byte; _, _ = input.Read(b[:]); cancel() }()
	type result struct {
		code string
		err  error
	}
	finished := make(chan result, 1)
	go func() { code, err := read(ctx); finished <- result{code, err} }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case r := <-finished:
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.err != nil {
			return r.err
		}
		_, err := fmt.Fprintln(output, r.code)
		return err
	}
}

func privilegedLinuxDeviceCode(ctx context.Context, op *vpnlog.Operation) (string, error) {
	op.Step("resolve_helper_executable")
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	input, cancelPipe, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer input.Close()
	defer cancelPipe.Close()
	cmd := exec.CommandContext(ctx, "pkexec", executable, linuxDeviceCodeFlag)
	cmd.Stdin = input
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = log.Writer()
	cmd.Cancel = func() error {
		// pkexec does not read stdin while waiting for authorization, and its
		// real uid still permits the caller to terminate it at that stage.
		_ = cancelPipe.Close()
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrPermission) {
			// Once all uids change, EOF terminates the privileged helper even
			// when its read-only hardware syscall has not returned.
			return nil
		}
		return err
	}
	op.Step("request_admin_authorization")
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("TopSAP Linux 管理员授权启动失败: %w", err)
	}
	input.Close()
	vpnlog.Printf("[TopSAP pid=%d] device helper_pid=%d", os.Getpid(), cmd.Process.Pid)
	op.Step("wait_privileged_device_identity")
	err = cmd.Wait()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("TopSAP Linux 管理员设备标识读取失败: %w", err)
	}
	op.Step("validate_helper_result")
	data := output.Bytes()
	if len(data) != 25 || data[24] != '\n' {
		return "", errors.New("TopSAP Linux 设备标识辅助进程返回格式无效")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(data[:24]))
	if err != nil || len(decoded) != md5.Size {
		return "", errors.New("TopSAP Linux 设备标识辅助进程返回摘要无效")
	}
	return string(data[:24]), nil
}

func linuxHardwareDeviceCode(ctx context.Context, op *vpnlog.Operation, readFile func(string) ([]byte, error), readDisk func(string) ([]byte, error)) (string, error) {
	op.Step("read_root_mount")
	mounts, diskErr := readFile("/etc/mtab")
	var identity string
	if diskErr == nil {
		var path string
		path, diskErr = linuxRootDisk(mounts)
		if diskErr == nil {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			op.Step("read_disk_identity")
			var data []byte
			data, diskErr = readDisk(path)
			if diskErr == nil {
				identity, diskErr = linuxDiskSerial(data)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if diskErr != nil {
		// This order is observed in the official Linux client, not an invented
		// alternate identity. Its random registration-file branch is omitted.
		vpnlog.Printf("[TopSAP pid=%d] disk identity unavailable error=%v", os.Getpid(), linuxHardwareError(diskErr))
		op.Step("read_dmi_product_uuid")
		data, err := readFile("/sys/class/dmi/id/product_uuid")
		if err != nil {
			return "", fmt.Errorf("TopSAP Linux 无可读取的已验证硬件标识: %w", errors.Join(linuxHardwareError(diskErr), linuxHardwareError(err)))
		}
		identity = strings.TrimSuffix(string(data), "\n")
		if identity == "" || strings.ContainsAny(identity, "\x00\r\n") {
			return "", errors.New("TopSAP Linux DMI 硬件标识为空或格式无效")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	op.Step("derive_device_code")
	digest := md5.Sum([]byte(identity + "lUoYuAncHeNtIaNkAijWn"))
	return base64.StdEncoding.EncodeToString(digest[:]), nil
}

func linuxHardwareError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

func linuxRootDisk(data []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[1] != "/" {
			continue
		}
		// Match the native trailing-digit removal, including its limits for
		// NVMe/LVM. Do not invent another device mapping before the DMI path.
		path := strings.TrimRight(fields[0], "0123456789")
		if !strings.HasPrefix(path, "/dev/") {
			return "", errors.New("根挂载不对应已验证的磁盘设备")
		}
		return path, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("未找到根挂载设备")
}

func readLinuxDiskIdentity(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var data [512]byte
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), unix.HDIO_GET_IDENTITY, uintptr(unsafe.Pointer(&data[0])))
	if errno != 0 {
		return nil, errno
	}
	return data[:], nil
}

func linuxDiskSerial(data []byte) (string, error) {
	if len(data) != 512 {
		return "", errors.New("磁盘硬件标识长度无效")
	}
	end := bytes.IndexByte(data[20:], 0)
	if end < 0 {
		return "", errors.New("磁盘硬件标识缺少结束标记")
	}
	serial := strings.ReplaceAll(string(data[20:20+end]), " ", "")
	if serial == "" {
		return "", errors.New("磁盘硬件标识为空")
	}
	return serial, nil
}
