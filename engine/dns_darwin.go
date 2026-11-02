package engine

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"EasyConnectSuite/vpncore/log"
)

func (r *darwinRoutes) setDNS(names []string) (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), r.ref.Profile, r.ref.Session, r.ref.Attachment), "darwin_dns", nil)
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	server, parseErr := netip.ParseAddr(r.dns)
	if parseErr != nil || !server.Is4() {
		outcome = "failed"
		return errors.New("invalid TUN DNS address")
	}
	wanted := make(map[string]bool, len(names))
	for _, value := range names {
		name := strings.TrimPrefix(strings.ToLower(value), ".")
		if !validDomain(name) || name == "." || name == ".." {
			outcome = "failed"
			return fmt.Errorf("invalid TUN DNS namespace %q", value)
		}
		wanted[name] = true
	}
	dir := r.resolverDir
	if dir == "" {
		dir = "/etc/resolver"
	}
	if r.resolverFiles == nil {
		r.resolverFiles = make(map[string]os.FileInfo)
	}
	contents := []byte("nameserver " + server.String() + "\n")
	var remove []string
	for name := range r.resolverFiles {
		if !wanted[name] {
			remove = append(remove, name)
		}
	}
	sort.Strings(remove)
	for _, name := range remove {
		op.Step("remove_resolver")
		path := filepath.Join(dir, name)
		info, e := os.Lstat(path)
		if e != nil {
			outcome = "failed"
			return fmt.Errorf("check DNS resolver %s: %w", name, e)
		}
		if !os.SameFile(r.resolverFiles[name], info) || !info.Mode().IsRegular() {
			outcome = "failed"
			return fmt.Errorf("DNS resolver %s changed; cleanup refused", name)
		}
		data, e := os.ReadFile(path)
		if e != nil || string(data) != string(contents) {
			outcome = "failed"
			return fmt.Errorf("DNS resolver %s changed; cleanup refused", name)
		}
		if e = os.Remove(path); e != nil {
			outcome = "failed"
			return fmt.Errorf("remove DNS resolver %s: %w", name, e)
		}
		delete(r.resolverFiles, name)
	}
	var add []string
	for name := range wanted {
		if _, ok := r.resolverFiles[name]; !ok {
			add = append(add, name)
		}
	}
	sort.Strings(add)
	if len(add) != 0 {
		op.Step("create_resolver_directory")
		if e := os.MkdirAll(dir, 0755); e != nil {
			outcome = "failed"
			return fmt.Errorf("create DNS resolver directory: %w", e)
		}
	}
	for _, name := range add {
		op.Step("create_resolver")
		path := filepath.Join(dir, name)
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			outcome = "failed"
			return fmt.Errorf("create DNS resolver %s: %w", name, e)
		}
		info, statErr := file.Stat()
		if statErr == nil {
			r.resolverFiles[name] = info
		}
		_, writeErr := file.Write(contents)
		closeErr := file.Close()
		if e = errors.Join(statErr, writeErr, closeErr); e != nil {
			outcome = "failed"
			return fmt.Errorf("write DNS resolver %s: %w", name, e)
		}
	}
	outcome = "completed"
	return nil
}
