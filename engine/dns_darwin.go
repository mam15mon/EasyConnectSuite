package engine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
		dir = darwinResolverDir
	}
	if r.resolverFiles == nil {
		r.resolverFiles = make(map[string]os.FileInfo)
	}
	if r.resolverMarker == "" && len(wanted) != 0 {
		var nonce [24]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			outcome = "failed"
			return err
		}
		r.resolverMarker = fmt.Sprintf("# EasyConnectSuite TUN pid=%d token=%s\n", os.Getpid(), hex.EncodeToString(nonce[:]))
	}
	contents := []byte("nameserver " + server.String() + "\n" + r.resolverMarker)
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
		if e := validateDarwinResolverDir(dir, dir == darwinResolverDir); e != nil {
			outcome = "failed"
			return fmt.Errorf("check DNS resolver directory: %w", e)
		}
	}
	for _, name := range add {
		op.Step("publish_resolver")
		info, e := createDarwinResolver(dir, name, contents)
		if info != nil {
			r.resolverFiles[name] = info
		}
		if e != nil {
			outcome = "failed"
			return fmt.Errorf("publish DNS resolver %s: %w", name, e)
		}
	}
	outcome = "completed"
	return nil
}

// Publish only fully written resolver contents, without replacing another owner.
func createDarwinResolver(dir, name string, contents []byte) (info os.FileInfo, err error) {
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	pattern := fmt.Sprintf("%s%d-%s-*", darwinResolverTempPrefix, os.Getpid(), hex.EncodeToString(nonce[:]))
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	temp := file.Name()
	defer func() { err = errors.Join(err, os.Remove(temp)) }()
	defer func() { err = errors.Join(err, file.Close()) }()
	var n int
	if n, err = file.Write(contents); err != nil {
		return nil, err
	}
	if n != len(contents) {
		return nil, io.ErrShortWrite
	}
	if err = file.Chmod(0644); err != nil {
		return nil, err
	}
	if err = file.Sync(); err != nil {
		return nil, err
	}
	if info, err = file.Stat(); err != nil {
		return nil, err
	}
	if err = os.Link(temp, filepath.Join(dir, name)); err != nil {
		return nil, err
	}
	return info, nil
}
