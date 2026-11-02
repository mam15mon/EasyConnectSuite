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
	name          string
	ref           TUNSpec
	dns           string
	resolverDir   string
	resolverFiles map[string]os.FileInfo
	run           func(context.Context, ...string) ([]byte, error)
}

func (r *darwinRoutes) Apply(add, remove []string) (result routeResult, err error) {
	operation := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), r.ref.Profile, r.ref.Session, r.ref.Attachment), "darwin_routes", nil)
	outcome := "interrupted"
	defer func() { operation.Finish(outcome, err) }()
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
	for _, action := range []struct {
		name     string
		prefixes []string
	}{{"delete", remove}, {"add", add}} {
		for _, value := range action.prefixes {
			prefix, parseErr := netip.ParsePrefix(value)
			if parseErr != nil || !prefix.Addr().Is4() || prefix.Bits() == 0 || prefix != prefix.Masked() {
				err = fmt.Errorf("invalid TUN route %q", value)
				outcome = "failed"
				return
			}
			operation.Step("route_" + action.name)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			run := r.run
			if run == nil {
				run = func(ctx context.Context, args ...string) ([]byte, error) {
					return exec.CommandContext(ctx, "/sbin/route", args...).CombinedOutput()
				}
			}
			if action.name == "delete" {
				operation.Step("verify_route_interface")
				output, checkErr := run(ctx, "-n", "get", "-net", value)
				if checkErr != nil || routeInterface(output) != r.name {
					cancel()
					err = fmt.Errorf("macOS TUN route %s no longer belongs to %s; cleanup refused", value, r.name)
					outcome = "failed"
					return
				}
				operation.Step("route_delete")
			}
			output, runErr := run(ctx, "-n", action.name, "-net", value, "-interface", r.name)
			cancel()
			if runErr != nil {
				err = fmt.Errorf("macOS TUN route %s %s: %w: %s", action.name, value, runErr, strings.TrimSpace(string(output)))
				outcome = "failed"
				return
			}
			if action.name == "add" {
				result.Added = append(result.Added, value)
			} else {
				result.Removed = append(result.Removed, value)
			}
		}
	}
	outcome = "completed"
	return
}

func routeInterface(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if found && key == "interface" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
