package main

import (
	"context"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
)

type shutdownWaitLog struct {
	output io.Writer
	seen   chan struct{}
	once   sync.Once
}

func (w *shutdownWaitLog) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "name=linux_shutdown") && strings.Contains(string(p), "stage=wait_startup") {
		w.once.Do(func() { close(w.seen) })
	}
	return w.output.Write(p)
}

func observeShutdownWait(t *testing.T) <-chan struct{} {
	t.Helper()
	previous := log.Writer()
	writer := &shutdownWaitLog{output: previous, seen: make(chan struct{})}
	log.SetOutput(writer)
	t.Cleanup(func() { log.SetOutput(previous) })
	return writer.seen
}

func waitForShutdownStage(t *testing.T, seen <-chan struct{}) {
	t.Helper()
	select {
	case <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not reach startup wait")
	}
}

func TestLinuxShutdownWaitsForStartup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	config := &options.App{
		OnStartup:  func(context.Context) { close(started); <-release },
		OnShutdown: func(context.Context) { close(stopped) },
	}
	configurePlatformOptions(config, nil)
	go config.OnStartup(context.Background())
	<-started
	seen := observeShutdownWait(t)
	go config.OnShutdown(context.Background())
	waitForShutdownStage(t, seen)
	select {
	case <-stopped:
		t.Fatal("shutdown ran before startup completed")
	default:
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not resume after startup")
	}
}

func TestLinuxShutdownWaitsWhenStartupHasNotRunYet(t *testing.T) {
	stopped := make(chan struct{})
	config := &options.App{OnStartup: func(context.Context) {}, OnShutdown: func(context.Context) { close(stopped) }}
	configurePlatformOptions(config, nil)
	seen := observeShutdownWait(t)
	go config.OnShutdown(context.Background())
	waitForShutdownStage(t, seen)
	select {
	case <-stopped:
		t.Fatal("shutdown ran before startup was invoked")
	default:
	}
	config.OnStartup(context.Background())
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not resume after startup")
	}
}
