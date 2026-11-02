//go:build !windows

package main

import (
	"EasyConnectSuite/engine"
	"context"
)

type fakeTUNControl struct{ prepared int }

func (f *fakeTUNControl) Attach(context.Context, engine.TUNSpec, engine.TUNBackend) (engine.TUNAttachment, error) {
	return nil, nil
}
func (f *fakeTUNControl) Prepare(context.Context, string) error { f.prepared++; return nil }
func (f *fakeTUNControl) Release(context.Context, string) error { return nil }
func (f *fakeTUNControl) Close()                                {}
