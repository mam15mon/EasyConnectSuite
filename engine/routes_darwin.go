package engine

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"

	"EasyConnectSuite/vpncore/log"
)

// darwinRoutes changes only routes assigned to its own utun interface.
type darwinRoutes struct {
	name           string
	ref            TUNSpec
	dns            string
	resolverDir    string
	resolverFiles  map[string]os.FileInfo
	resolverMarker string
	run            func(context.Context, ...string) ([]byte, error)
}

func (r *darwinRoutes) Apply(add, remove []string) (result routeResult, err error) {
	operation := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), r.ref.Profile, r.ref.Session, r.ref.Attachment), "darwin_routes", nil)
	outcome := "interrupted"
	defer func() { operation.Finish(outcome, err) }()
	if len(remove) != 0 {
		outcome = "failed"
		return result, fmt.Errorf("macOS TUN routes are removed only by closing their interface")
	}
	if !strings.HasPrefix(r.name, "utun") || len(r.name) <= 4 {
		err = fmt.Errorf("invalid utun interface")
		outcome = "failed"
		return
	}
	for _, c := range r.name[4:] {
		if c < '0' || c > '9' {
			err = fmt.Errorf("invalid utun interface")
			outcome = "failed"
			return
		}
	}
	for _, value := range add {
		prefix, parseErr := netip.ParsePrefix(value)
		if parseErr != nil || !prefix.Addr().Is4() || prefix.Bits() == 0 || prefix != prefix.Masked() {
			err = fmt.Errorf("invalid TUN route %q", value)
			outcome = "failed"
			return
		}
		operation.Step("route_add")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		run := r.run
		if run == nil {
			run = func(ctx context.Context, args ...string) ([]byte, error) {
				return exec.CommandContext(ctx, "/sbin/route", args...).CombinedOutput()
			}
		}
		output, runErr := run(ctx, "-n", "add", "-net", value, "-interface", r.name)
		cancel()
		if runErr != nil {
			err = fmt.Errorf("macOS TUN route add %s: %w: %s", value, runErr, strings.TrimSpace(string(output)))
			outcome = "failed"
			return
		}
		result.Added = append(result.Added, value)
	}
	outcome = "completed"
	return
}
