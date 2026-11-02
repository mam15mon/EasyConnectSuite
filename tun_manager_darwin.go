package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
	vpnlog "EasyConnectSuite/vpncore/log"
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
	launch       func(string) (*exec.Cmd, error)
}

func newTUNManager() *tunManager {
	return &tunManager{attachments: map[string]*remoteTUN{}, reservations: map[string]bool{}, flows: make(chan struct{}, tunipc.MaxFlows), launch: launchDarwinWorker}
}

const darwinWorkerScript = `on run argv
  do shell script (quoted form of (item 1 of argv) & " --tun-worker " & quoted form of (item 2 of argv) & " " & quoted form of (item 3 of argv)) with administrator privileges
end run`

func launchDarwinWorker(name string) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("/usr/bin/osascript", "-e", darwinWorkerScript, "--", executable, name, strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 macOS 管理员授权失败: %w", err)
	}
	return cmd, nil
}

func (m *tunManager) ensure(ctx context.Context) (err error) {
	trace := vpnlog.BeginOperation(fmt.Sprintf("[TUN pid=%d] ", os.Getpid()), "darwin_worker_launch", ctx.Done())
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		trace.Finish(outcome, err)
	}()
	m.mu.Lock()
	peer, recoveryErr := m.peer, m.recoveryErr
	m.mu.Unlock()
	if recoveryErr != nil {
		return recoveryErr
	}
	if peer != nil {
		select {
		case <-peer.Done():
			return errors.New("TUN Worker 已退出，请等待清理")
		default:
			outcome = "already_running"
			return nil
		}
	}
	trace.Step("create_control_socket")
	name := tunipc.NewName()
	listener, err := tunipc.Listen(name)
	if err != nil {
		return err
	}
	defer listener.Close()
	trace.Step("request_admin_authorization")
	cmd, err := m.launch(name)
	if err != nil {
		return err
	}
	processDone := make(chan struct{})
	var processErr error
	go func() { processErr = cmd.Wait(); close(processDone) }()
	handshake, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	go func() {
		select {
		case <-processDone:
			cancel()
		case <-handshake.Done():
		}
	}()
	trace.Step("accept_authenticated_worker")
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	conn, pid, err := listener.AcceptWorker(handshake, executable)
	if err != nil {
		select {
		case <-processDone:
			return fmt.Errorf("macOS TUN Worker 未启动或授权已取消: %w", errors.Join(err, processErr))
		default:
			unknown := errors.New("macOS TUN Worker 状态未知，无法确认清理；请退出程序后检查")
			m.mu.Lock()
			m.recoveryErr = unknown
			m.mu.Unlock()
			return errors.Join(err, unknown)
		}
	}
	peer = tunipc.NewPeer(conn, m.handle, func(msg tunipc.Message) {
		if msg.Op == "log" || msg.Op == "progress" {
			log.Printf("[TUN worker=%d profile=%q] %s", pid, msg.Ref.Profile, msg.Text)
		}
	})
	m.mu.Lock()
	m.peer, m.pid, m.workerDone = peer, pid, make(chan struct{})
	workerDone := m.workerDone
	m.mu.Unlock()
	peer.Heartbeat()
	go func() {
		defer close(workerDone)
		select {
		case <-processDone:
			peer.Close()
			m.lost(peer, processErr)
		case <-peer.Done():
			var cleanupErr error
			select {
			case <-processDone:
				cleanupErr = processErr
			case <-time.After(45 * time.Second):
				cleanupErr = errors.New("macOS TUN Worker 清理超时")
			}
			m.lost(peer, cleanupErr)
		}
	}()
	trace.Step("worker_hello")
	if _, err = peer.Call(handshake, tunipc.Message{Op: "hello"}); err != nil {
		peer.Close()
		return err
	}
	outcome = "ready"
	return nil
}

func (a *App) cleanupTUNOnStartup() (bool, error) { return engine.CleanupStaleDarwinTUN(true) }
