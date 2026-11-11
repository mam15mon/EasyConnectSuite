//go:build windows || darwin || linux

package engine

import (
	"EasyConnectSuite/internal/tunipc"
	"EasyConnectSuite/vpncore/log"
	"context"
	"encoding/binary"
	"errors"
	"github.com/miekg/dns"
	stun "github.com/mythologyli/sing-tun"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"
)

const tunDNS = "198.19.128.2"

// TUNCleanupError identifies failed cleanup after an attachment operation.
type TUNCleanupError struct{ Err error }

func (e *TUNCleanupError) Error() string { return "TUN cleanup failed: " + e.Err.Error() }
func (e *TUNCleanupError) Unwrap() error { return e.Err }

func joinTUNCleanupError(operationErr, cleanupErr error) error {
	if cleanupErr == nil {
		return operationErr
	}
	return errors.Join(operationErr, &TUNCleanupError{Err: cleanupErr})
}

type tunNetworkBackend interface {
	routeBackend
	setDNS([]string) error
}

type tunIngress struct {
	name, dns, prefix string
	mu                sync.RWMutex
	device            stun.Tun
	stack             stun.Stack
	service           TUNBackend
	spec              TUNSpec
	routes            *routeManager
	backend           tunNetworkBackend
	ctx               context.Context
	cancel            context.CancelFunc
	closeOnce         sync.Once
	closeErr          error
	claimRef          tunipc.Ref
	closed            bool
	connections       sync.Map
}

func (t *tunIngress) UpdateSpec(spec TUNSpec) error { return t.update(spec, nil) }
func (t *tunIngress) update(spec TUNSpec, progress func(string, string)) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	prefixes := append(append([]string(nil), spec.Prefixes...), t.dnsAddress()+"/32")
	tunPolicies.Lock()
	defer tunPolicies.Unlock()
	if err := checkTUNConflicts(t, prefixes, spec.Names); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return net.ErrClosed
	}
	if err := validateIngressUpdate(t, spec); err != nil {
		return err
	}
	if progress != nil {
		progress("配置路由", "正在配置 VPN 资源路由…")
	}
	// Reserve both the old and intended policy before making changes. A
	// partially applied policy remains reserved until cleanup succeeds.
	claim := tunPolicies.claims[t]
	claim.prefixes = append(append([]string(nil), claim.prefixes...), prefixes...)
	claim.names = append(append([]string(nil), claim.names...), spec.Names...)
	tunPolicies.claims[t] = claim
	t.claimRef = spec.Ref
	if err := t.routes.replace(prefixes); err != nil {
		return err
	}
	if err := t.backend.setDNS(append([]string(nil), spec.Names...)); err != nil {
		return err
	}
	t.spec = spec
	tunPolicies.claims[t] = tunClaim{prefixes: prefixes, names: spec.Names}
	return nil
}

func (t *tunIngress) Close() error {
	t.closeOnce.Do(func() {
		tunPolicies.Lock()
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		tunPolicies.Unlock()
		// Keep resource claims until teardown completes, but let pending DNS
		// handlers observe closed before waiting for the stack to stop.
		cleanupCompleted := false
		defer func() {
			if cleanupCompleted && t.closeErr == nil {
				tunPolicies.Lock()
				delete(tunPolicies.claims, t)
				tunPolicies.Unlock()
			} else {
				log.Printf("[TUN pid=%d profile=%q session=%q attachment=%q] resource_claim_retained cleanup_completed=%t cause=%v", os.Getpid(), t.claimRef.Profile, t.claimRef.Session, t.claimRef.Attachment, cleanupCompleted, t.closeErr)
			}
		}()
		t.cancel()
		t.connections.Range(func(key, _ any) bool { _ = key.(io.Closer).Close(); return true })
		if t.backend != nil {
			t.closeErr = errors.Join(t.closeErr, t.backend.setDNS(nil))
		}
		t.closeErr = errors.Join(t.closeErr, releaseIngressRoutes(t))
		if t.stack != nil {
			t.closeErr = errors.Join(t.closeErr, t.stack.Close())
		}
		cleanupCompleted = true
	})
	return t.closeErr
}

func (t *tunIngress) track(conn io.Closer) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.closed {
		_ = conn.Close()
		return false
	}
	t.connections.Store(conn, true)
	return true
}

func (t *tunIngress) dial(ctx context.Context, network string, target M.Socksaddr) (net.Conn, error) {
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return nil, net.ErrClosed
	}
	return t.service.Dial(ctx, network, target.String())
}

func (t *tunIngress) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	defer conn.Close()
	if !t.track(conn) {
		return net.ErrClosed
	}
	defer t.connections.Delete(conn)
	if metadata.Destination.Addr.String() == t.dnsAddress() && metadata.Destination.Port == 53 {
		return t.serveTCPDNS(conn)
	}
	upstream, err := t.dial(ctx, "tcp", metadata.Destination)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if !t.track(upstream) {
		return net.ErrClosed
	}
	defer t.connections.Delete(upstream)
	tunipc.Relay(conn, upstream, false)
	return nil
}

func (t *tunIngress) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	defer conn.Close()
	if !t.track(conn) {
		return net.ErrClosed
	}
	defer t.connections.Delete(conn)
	if metadata.Destination.Addr.String() == t.dnsAddress() && metadata.Destination.Port == 53 {
		return t.serveUDPDNS(conn)
	}
	upstream, err := t.dial(ctx, "udp", metadata.Destination)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if !t.track(upstream) {
		return net.ErrClosed
	}
	defer t.connections.Delete(upstream)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer upstream.Close()
		for {
			packet := buf.NewPacket()
			_, err := conn.ReadPacket(packet)
			if err == nil {
				_, err = upstream.Write(packet.Bytes())
			}
			packet.Release()
			if err != nil {
				return
			}
		}
	}()
	for {
		_ = upstream.SetReadDeadline(time.Now().Add(60 * time.Second))
		data := make([]byte, 65535)
		n, err := upstream.Read(data)
		if err != nil {
			break
		}
		if err := conn.WritePacket(buf.As(data[:n]), metadata.Destination); err != nil {
			break
		}
	}
	_ = conn.Close()
	_ = upstream.Close()
	<-done
	return nil
}

func (t *tunIngress) serveTCPDNS(conn net.Conn) error {
	for {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		var length [2]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return err
		}
		request := make([]byte, int(binary.BigEndian.Uint16(length[:])))
		if _, err := io.ReadFull(conn, request); err != nil {
			return err
		}
		response, err := t.answerDNS(request)
		if err != nil {
			return err
		}
		binary.BigEndian.PutUint16(length[:], uint16(len(response)))
		if _, err := conn.Write(append(length[:], response...)); err != nil {
			return err
		}
	}
}

func (t *tunIngress) serveUDPDNS(conn N.PacketConn) error {
	for {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		packet := buf.NewPacket()
		target, err := conn.ReadPacket(packet)
		if err != nil {
			packet.Release()
			return err
		}
		response, err := t.answerDNS(packet.Bytes())
		packet.Release()
		if err != nil {
			return err
		}
		if err := conn.WritePacket(buf.As(response), target); err != nil {
			return err
		}
	}
}

func (t *tunIngress) answerDNS(data []byte) ([]byte, error) {
	t.mu.RLock()
	ref := t.spec.Ref
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return nil, net.ErrClosed
	}
	answer, routes, err := t.service.DNS(t.ctx, data)
	if err != nil {
		return nil, err
	}
	tunPolicies.Lock()
	defer tunPolicies.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.spec.Ref != ref {
		return nil, net.ErrClosed
	}
	for _, prefix := range routes {
		p, err := netip.ParsePrefix(prefix)
		if err != nil || !p.Addr().Is4() || p.Bits() != 32 {
			return nil, errors.New("invalid DNS route")
		}
		err = checkTUNConflicts(t, []string{prefix}, nil)
		if err == nil {
			err = t.routes.add(prefix)
		}
		if err != nil {
			request := new(dns.Msg)
			if e := request.Unpack(data); e != nil {
				return nil, e
			}
			reply := new(dns.Msg)
			reply.SetRcode(request, dns.RcodeServerFailure)
			return reply.Pack()
		}
		claim := tunPolicies.claims[t]
		if !slices.Contains(claim.prefixes, prefix) {
			claim.prefixes = append(claim.prefixes, prefix)
			tunPolicies.claims[t] = claim
		}
	}
	return answer, nil
}

func (t *tunIngress) NewError(_ context.Context, err error) { log.Printf("TUN: %v", err) }

type tunLogger struct{}

var _ logger.Logger = tunLogger{}

func (tunLogger) Trace(args ...any) { log.DebugPrintln(args...) }
func (tunLogger) Debug(args ...any) { log.DebugPrintln(args...) }
func (tunLogger) Info(args ...any)  { log.Println(args...) }
func (tunLogger) Warn(args ...any)  { log.Println(args...) }
func (tunLogger) Error(args ...any) { log.Println(args...) }
func (tunLogger) Fatal(args ...any) { log.Println(args...) }
func (tunLogger) Panic(args ...any) { log.Println(args...) }
