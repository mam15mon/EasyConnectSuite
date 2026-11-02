package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"EasyConnectSuite/internal/tunipc"
	"golang.org/x/sys/windows"
)

type tunManager struct {
	op           sync.Mutex
	mu           sync.Mutex
	peer         *tunipc.Peer
	pid          uint32
	workerDone   chan struct{}
	recoveryErr  error
	attachments  map[string]*remoteTUN
	reservations map[string]bool
	flows        chan struct{}
	launch       func(string) (windows.Handle, error)
}

func newTUNManager() *tunManager {
	return &tunManager{attachments: map[string]*remoteTUN{}, reservations: map[string]bool{}, flows: make(chan struct{}, tunipc.MaxFlows), launch: launchTUNWorker}
}
func (m *tunManager) ensure(ctx context.Context) error {
	m.mu.Lock()
	p := m.peer
	recoveryErr := m.recoveryErr
	m.mu.Unlock()
	if recoveryErr != nil {
		return recoveryErr
	}
	if p != nil {
		select {
		case <-p.Done():
			return errors.New("TUN Worker 已退出，请重新开启 TUN")
		default:
			return nil
		}
	}
	name := tunipc.NewName()
	listener, err := tunipc.Listen(name)
	if err != nil {
		return err
	}
	defer listener.Close()
	handle, err := m.launch(name)
	if err != nil {
		return err
	}
	pid, err := windows.GetProcessId(handle)
	if err != nil {
		windows.CloseHandle(handle)
		return err
	}
	handshake, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	conn, err := listener.Accept(handshake, pid)
	if err != nil {
		windows.CloseHandle(handle)
		return err
	}
	if state, e := windows.WaitForSingleObject(handle, 0); e != nil || state == windows.WAIT_OBJECT_0 {
		conn.Close()
		windows.CloseHandle(handle)
		return errors.New("TUN Worker exited during handshake")
	}
	peer := tunipc.NewPeer(conn, m.handle, func(msg tunipc.Message) {
		if msg.Op == "log" || msg.Op == "progress" {
			log.Printf("[TUN worker=%d profile=%q] %s", pid, msg.Ref.Profile, msg.Text)
		}
	})
	m.mu.Lock()
	m.peer = peer
	m.pid = pid
	m.workerDone = make(chan struct{})
	workerDone := m.workerDone
	m.mu.Unlock()
	peer.Heartbeat()
	if _, err = peer.Call(handshake, tunipc.Message{Op: "hello"}); err != nil {
		peer.Close()
		windows.CloseHandle(handle)
		m.mu.Lock()
		m.peer = nil
		m.mu.Unlock()
		return err
	}
	go func() {
		defer windows.CloseHandle(handle)
		defer close(workerDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-peer.Done():
				m.lost(peer, waitTUNWorkerExit(handle))
				return
			case <-ticker.C:
				r, e := windows.WaitForSingleObject(handle, 0)
				if e != nil || r == windows.WAIT_OBJECT_0 {
					peer.Close()
				}
			}
		}
	}()
	return nil
}
func launchTUNWorker(name string) (windows.Handle, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	parameters := windows.ComposeCommandLine([]string{"--tun-worker", name, strconv.Itoa(os.Getpid())})
	info := shellExecuteInfo{Mask: 0x100 | 0x400 | 0x40, Verb: windows.StringToUTF16Ptr("runas"), File: windows.StringToUTF16Ptr(exe), Parameters: windows.StringToUTF16Ptr(parameters), Directory: windows.StringToUTF16Ptr(filepath.Dir(exe)), Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2|4)
	if int32(hr) < 0 {
		return 0, fmt.Errorf("初始化 TUN Worker 启动失败：0x%x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	result, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, errors.New("已取消管理员授权")
		}
		return 0, fmt.Errorf("启动 TUN Worker 失败：%w", err)
	}
	return info.Process, nil
}

func runTUNWorker(args []string) (result error) {
	if len(args) != 3 || args[0] != "--tun-worker" {
		return errors.New("invalid TUN Worker arguments")
	}
	pid, e := strconv.ParseUint(args[2], 10, 32)
	if e != nil || pid == 0 || pid == uint64(os.Getpid()) {
		return errors.New("invalid GUI PID")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("TUN Worker requires administrator permission")
	}
	parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return e
	}
	defer windows.CloseHandle(parent)
	var path [32768]uint16
	size := uint32(len(path))
	if e = windows.QueryFullProcessImageName(parent, 0, &path[0], &size); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if !strings.EqualFold(filepath.Clean(exe), filepath.Clean(windows.UTF16ToString(path[:size]))) {
		return errors.New("GUI executable identity mismatch")
	}
	ctx, c := context.WithTimeout(context.Background(), 20*time.Second)
	defer c()
	conn, e := tunipc.Dial(ctx, args[1], uint32(pid))
	if e != nil {
		return e
	}
	w := &workerServer{pid: uint32(pid), entries: map[string]*workerEntry{}, reservations: map[string]bool{}, closing: make(chan struct{})}
	ready := make(chan struct{})
	w.peer = tunipc.NewPeer(conn, func(ctx context.Context, m tunipc.Message) (tunipc.Message, error) { <-ready; return w.handle(ctx, m) }, nil)
	close(ready)
	log.SetOutput(workerLog{w.peer})
	w.peer.Heartbeat()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer w.peer.Close()
	defer func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if err := w.cleanup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			result = errors.Join(result, err)
		}
	}()
	for {
		select {
		case <-w.peer.Done():
			return nil
		case <-w.closing:
			select {
			case <-w.peer.Done():
			case <-time.After(time.Second):
			}
			return nil
		case <-ticker.C:
			r, e := windows.WaitForSingleObject(parent, 0)
			if e != nil || r == windows.WAIT_OBJECT_0 {
				return nil
			}
		}
	}
}

// A dead control pipe does not imply that the old process finished removing
// routes. Keep its handle until cleanup completes before permitting replacement.
func waitTUNWorkerExit(handle windows.Handle) error {
	state, err := windows.WaitForSingleObject(handle, 45000)
	if err != nil {
		return fmt.Errorf("wait TUN Worker cleanup: %w", err)
	}
	if state != windows.WAIT_OBJECT_0 {
		return errors.New("TUN Worker cleanup timed out")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("TUN Worker exited with code %d; cleanup is unconfirmed", code)
	}
	return nil
}
