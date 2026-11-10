package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"EasyConnectSuite/internal/tunipc"
)

func runTUNWorker(args []string) (result error) {
	if len(args) != 3 || args[0] != "--tun-worker" {
		return errors.New("invalid TUN worker arguments")
	}
	pid, err := strconv.ParseUint(args[2], 10, 32)
	if err != nil || pid == 0 || pid == uint64(os.Getpid()) {
		return errors.New("invalid GUI PID")
	}
	if os.Geteuid() != 0 {
		return errors.New("TUN worker requires administrator permission")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := tunipc.Dial(ctx, args[1], uint32(pid))
	if err != nil {
		return err
	}
	if err = tunipc.VerifyParent(conn, args[1], uint32(pid), executable); err != nil {
		conn.Close()
		return err
	}
	w := &workerServer{pid: uint32(pid), entries: map[string]*workerEntry{}, reservations: map[string]bool{}, closing: make(chan struct{})}
	ready := make(chan struct{})
	w.peer = tunipc.NewPeer(conn, func(ctx context.Context, m tunipc.Message) (tunipc.Message, error) { <-ready; return w.handle(ctx, m) }, nil)
	close(ready)
	log.SetOutput(workerLog{w.peer})
	w.peer.Heartbeat()
	defer w.peer.Close()
	defer func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if err := w.cleanup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			result = errors.Join(result, err)
		}
	}()
	select {
	case <-w.peer.Done():
	case <-w.closing:
		select {
		case <-w.peer.Done():
		case <-time.After(time.Second):
		}
	}
	return nil
}
