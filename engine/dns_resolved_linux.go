package engine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"EasyConnectSuite/vpncore/log"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// The resolved API applies settings to a single interface index. Callers must
// keep that exclusive, non-persistent interface alive until each service reply.
type resolvedAddress struct {
	Family  int32
	Address []byte
}
type resolvedDomain struct {
	Name      string
	RouteOnly bool
}

// resolvedCall returns only after a service reply, unless it returns a tagged
// not-sent or outcome-unknown error. Plain errors represent confirmed rejection.
type resolvedCall func(context.Context, string, ...any) error

var errResolvedOutcomeUnknown = errors.New("Linux DNS 请求结果未确认，禁止自动重新配置或声称清理完成")
var errResolvedNotSent = errors.New("Linux DNS 请求未发送")

func resolvedDNSArguments(server string, names []string) ([]resolvedAddress, []resolvedDomain, error) {
	ip, err := netip.ParseAddr(server)
	if err != nil || !ip.Is4() {
		return nil, nil, fmt.Errorf("invalid TUN DNS server")
	}
	seen := make(map[string]bool)
	domains := make([]resolvedDomain, 0, len(names))
	for _, raw := range names {
		name := strings.TrimPrefix(strings.ToLower(raw), ".")
		if !validDomain(name) {
			return nil, nil, fmt.Errorf("invalid TUN DNS namespace")
		}
		if !seen[name] {
			seen[name] = true
			domains = append(domains, resolvedDomain{Name: name, RouteOnly: true})
		}
	}
	sort.Slice(domains, func(i, j int) bool { return domains[i].Name < domains[j].Name })
	return []resolvedAddress{{Family: unix.AF_INET, Address: ip.AsSlice()}}, domains, nil
}

// resolvedLinkDNS encapsulates D-Bus encoding. Its caller owns rollback and
// must not call it for an interface whose identity is no longer confirmed.
func resolvedLinkDNS(ctx context.Context, call resolvedCall, index int, server string, names []string) error {
	if index <= 0 {
		return fmt.Errorf("invalid TUN interface index")
	}
	addresses, domains, err := resolvedDNSArguments(server, names)
	if err != nil {
		return err
	}
	if err = call(ctx, "SetLinkDefaultRoute", int32(index), false); err != nil {
		return fmt.Errorf("disable link default DNS route: %w", err)
	}
	if err = call(ctx, "SetLinkDNS", int32(index), addresses); err != nil {
		return fmt.Errorf("set link DNS: %w", err)
	}
	if err = call(ctx, "SetLinkDomains", int32(index), domains); err != nil {
		return fmt.Errorf("set link domains: %w", err)
	}
	return nil
}

// Each attachment has a private bus connection and pins the service's unique
// owner. A canceled local waiter is not evidence that resolved finished writing.
type resolvedClient struct {
	conn      *dbus.Conn
	owner     string
	ref       TUNSpec
	mu        sync.Mutex
	pendingMu sync.Mutex
	pending   *resolvedPending
}

type resolvedPending struct {
	serial uint32
	reply  chan error
}

func newResolvedClient(ctx context.Context, ref TUNSpec) (client *resolvedClient, err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), ref.Profile, ref.Session, ref.Attachment, ref.Policy), "linux_dns_connect", ctx.Done())
	outcome := "failed"
	defer func() { op.Finish(outcome, err) }()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, errResolvedNotSent)
	}
	// Cancellation owns the connection only during authentication/owner lookup.
	// Stop this hook before returning it to a sender that must await real replies.
	connCtx, cancelConn := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx, cancelConn)
	defer stop()
	c := &resolvedClient{ref: ref}
	op.Step("connect_system_bus")
	c.conn, err = dbus.ConnectSystemBus(dbus.WithContext(connCtx), dbus.WithIncomingInterceptor(c.incoming), dbus.WithOutgoingInterceptor(c.outgoing))
	if err != nil {
		cancelConn()
		return nil, errors.Join(err, ctx.Err(), errResolvedNotSent)
	}
	defer func() {
		if err != nil {
			_ = c.conn.Close()
			cancelConn()
		}
	}()
	op.Step("resolve_unique_owner")
	var owner string
	err = c.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, "org.freedesktop.resolve1").Store(&owner)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("systemd-resolved unavailable: %w", safeResolvedError(err)), errResolvedNotSent)
	}
	if !stop() || ctx.Err() != nil {
		return nil, errors.Join(ctx.Err(), errResolvedNotSent)
	}
	if !strings.HasPrefix(owner, ":") {
		return nil, errors.Join(errors.New("invalid resolved unique owner"), errResolvedNotSent)
	}
	c.pendingMu.Lock()
	c.owner = owner
	c.pendingMu.Unlock()
	outcome = "connected"
	return c, nil
}

func safeResolvedError(err error) error {
	var serviceErr dbus.Error
	if errors.As(err, &serviceErr) {
		return fmt.Errorf("D-Bus error %s", serviceErr.Name)
	}
	var serviceErrPtr *dbus.Error
	if errors.As(err, &serviceErrPtr) {
		return fmt.Errorf("D-Bus error %s", serviceErrPtr.Name)
	}
	return err
}

func (c *resolvedClient) outgoing(msg *dbus.Message) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.pending != nil && msg.Type == dbus.TypeMethodCall && msg.Headers[dbus.FieldDestination].Value() == c.owner {
		c.pending.serial = msg.Serial()
	}
}

func (c *resolvedClient) incoming(msg *dbus.Message) {
	if msg.Type != dbus.TypeMethodReply && msg.Type != dbus.TypeError {
		return
	}
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	p := c.pending
	serial, _ := msg.Headers[dbus.FieldReplySerial].Value().(uint32)
	if p == nil || p.serial == 0 || serial != p.serial {
		return
	}
	sender, _ := msg.Headers[dbus.FieldSender].Value().(string)
	if sender != c.owner && sender != "org.freedesktop.DBus" {
		return
	}
	var err error
	if msg.Type == dbus.TypeError {
		name, _ := msg.Headers[dbus.FieldErrorName].Value().(string)
		err = fmt.Errorf("D-Bus error %s", name)
	}
	if sender != c.owner {
		err = errors.Join(err, errResolvedOutcomeUnknown)
	}
	select {
	case p.reply <- err:
	default:
	}
}

func (c *resolvedClient) Call(ctx context.Context, method string, args ...any) (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), c.ref.Profile, c.ref.Session, c.ref.Attachment, c.ref.Policy), "linux_dns_request", ctx.Done())
	outcome := "failed"
	defer func() {
		if err == nil {
			outcome = "confirmed"
		}
		op.Finish(outcome, err)
	}()
	op.Step("wait_request_lock")
	c.mu.Lock()
	defer c.mu.Unlock()
	if err = ctx.Err(); err != nil {
		return errors.Join(err, errResolvedNotSent)
	}
	switch method {
	case "SetLinkDefaultRoute", "SetLinkDNS", "SetLinkDomains":
	default:
		return errors.Join(errors.New("unsupported resolved mutation"), errResolvedNotSent)
	}
	p := &resolvedPending{reply: make(chan error, 1)}
	c.pendingMu.Lock()
	c.pending = p
	c.pendingMu.Unlock()
	defer func() { c.pendingMu.Lock(); c.pending = nil; c.pendingMu.Unlock() }()
	msg := &dbus.Message{Type: dbus.TypeMethodCall, Flags: dbus.FlagNoAutoStart, Headers: map[dbus.HeaderField]dbus.Variant{
		dbus.FieldDestination: dbus.MakeVariant(c.owner),
		dbus.FieldPath:        dbus.MakeVariant(dbus.ObjectPath("/org/freedesktop/resolve1")),
		dbus.FieldInterface:   dbus.MakeVariant("org.freedesktop.resolve1.Manager"),
		dbus.FieldMember:      dbus.MakeVariant(method),
		dbus.FieldSignature:   dbus.MakeVariant(dbus.SignatureOf(args...)),
	}, Body: args}
	op.Step("send_" + method)
	done := make(chan *dbus.Call, 1)
	c.conn.Send(msg, done)
	op.Step("wait_service_reply")
	for {
		select {
		case err = <-p.reply:
			if err == nil {
				outcome = "confirmed"
			}
			return err
		case call := <-done:
			// godbus accepts reply serials without checking the sender. Only the
			// interceptor can confirm completion; a local write failure is unknown.
			if call.Err != nil {
				var remote dbus.Error
				if !errors.As(call.Err, &remote) {
					select {
					case err = <-p.reply:
						return err
					default:
					}
					return errors.Join(safeResolvedError(call.Err), errResolvedOutcomeUnknown)
				}
			}
		case <-c.conn.Context().Done():
			select {
			case err = <-p.reply:
				return err
			default:
			}
			return errors.Join(dbus.ErrClosed, errResolvedOutcomeUnknown)
		}
	}
}

func (c *resolvedClient) Close() error { return c.conn.Close() }

// Kept as a one-request entry point for transport tests. Production attachments
// keep a single resolvedClient across all configuration requests.
func resolvedSystemCall(ctx context.Context, method string, args ...any) error {
	client, err := newResolvedClient(ctx, TUNSpec{})
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Call(ctx, method, args...)
}

// resolvedDNSOwner tracks completion on an exclusive TUN. DNS is removed by
// deleting that link, never by a whole-link RevertLink request.
type resolvedDNSOwner struct {
	index   int
	server  string
	ref     TUNSpec
	call    resolvedCall
	verify  func() error
	ctx     context.Context
	close   func() error
	touched bool
	unknown bool
}

func (d *resolvedDNSOwner) Apply(ctx context.Context, names []string) (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), d.ref.Profile, d.ref.Session, d.ref.Attachment, d.ref.Policy), "linux_dns_apply", ctx.Done())
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		op.Finish(outcome, err)
	}()
	if d.unknown {
		return errResolvedOutcomeUnknown
	}
	if d.index <= 0 || d.call == nil || d.verify == nil {
		return fmt.Errorf("invalid DNS owner")
	}
	if _, _, err = resolvedDNSArguments(d.server, names); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	op.Step("verify_interface")
	if err = d.verify(); err != nil {
		return err
	}
	op.Step("configure_link_dns")
	d.touched = true
	if err = resolvedLinkDNS(ctx, d.call, d.index, d.server, names); err != nil {
		d.unknown = errors.Is(err, errResolvedOutcomeUnknown)
		if d.unknown {
			op.Step("dns_outcome_unconfirmed")
		}
		return err
	}
	outcome = "configured"
	return nil
}

func (d *resolvedDNSOwner) Close(ctx context.Context) (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), d.ref.Profile, d.ref.Session, d.ref.Attachment, d.ref.Policy), "linux_dns_release", ctx.Done())
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		op.Finish(outcome, err)
	}()
	op.Step("close_private_bus")
	if d.close != nil {
		err = d.close()
	}
	if d.unknown {
		err = errors.Join(err, errResolvedOutcomeUnknown)
	}
	if err != nil {
		return err
	}
	// The caller closes and verifies removal of the owned non-persistent link.
	outcome = "ready_for_link_removal"
	return nil
}
