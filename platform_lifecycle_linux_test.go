package main

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
)

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
	go config.OnShutdown(context.Background())
	select {
	case <-stopped:
		t.Fatal("shutdown ran before startup completed")
	case <-time.After(100 * time.Millisecond):
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
	go config.OnShutdown(context.Background())
	select {
	case <-stopped:
		t.Fatal("shutdown ran before startup was invoked")
	case <-time.After(100 * time.Millisecond):
	}
	config.OnStartup(context.Background())
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not resume after startup")
	}
}
