package engine

import (
	"EasyConnectSuite/vpncore/client"
	"EasyConnectSuite/vpncore/log"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"EasyConnectSuite/internal/tunipc"
)

type tunSession struct {
	op            sync.Mutex
	mu            sync.Mutex
	status        TUNStatus
	attachment    TUNAttachment
	backend       *tunBackend
	policy        *connectionPolicy
	cancel        context.CancelFunc
	spec          TUNSpec
	desired       bool
	recoveryAfter time.Time
}

func (e *Engine) TUNState() TUNStatus {
	e.tunSession.mu.Lock()
	defer e.tunSession.mu.Unlock()
	s := e.tunSession.status
	if s.State == "" {
		s.State = "off"
	}
	return s
}
func (e *Engine) tunStatus(state string, err error) {
	e.logf("TUN state=%s cause=%v", state, err)
	s := TUNStatus{State: state}
	if err != nil {
		s.Error = err.Error()
	}
	e.tunSession.mu.Lock()
	e.tunSession.status = s
	e.tunSession.mu.Unlock()
	if e.opts.OnTUNStatus != nil {
		e.opts.OnTUNStatus(s)
	}
}
func (e *Engine) SetTUN(ctx context.Context, enabled bool) error {
	t := &e.tunSession
	t.op.Lock()
	var refresh *connectionPolicy
	defer func() {
		t.op.Unlock()
		if refresh != nil {
			e.goWithRecovery("TUN DNS zone refresh", func() { e.updateTUN(refresh, true) })
		}
	}()
	if !enabled {
		t.desired = false
		t.recoveryAfter = time.Time{}
		return e.detachTUN(ctx)
	}
	newAttachment := t.attachment == nil
	err := e.attachTUN(ctx, true)
	if err == nil {
		t.desired = true
		t.recoveryAfter = time.Time{}
		if newAttachment && t.policy.useRemoteDNS && t.policy.dnsName != "" && len(t.spec.Names) != 0 {
			refresh = t.policy
		}
	}
	return err
}

// attachTUN requires tunSession.op. Automatic recovery is rate limited separately
// so a replacement Worker that immediately fails cannot trigger an elevation loop.
func (e *Engine) attachTUN(ctx context.Context, freshNames bool) error {
	t := &e.tunSession
	if !e.running.Load() || e.ctx.Err() != nil {
		return errors.New("VPN 会话不可附着 TUN")
	}
	if t.attachment != nil {
		return nil
	}
	if e.opts.TUNController == nil {
		return errors.New("TUN Worker 控制器未配置")
	}
	e.tunStatus("attaching", nil)
	policy := e.router.current.Load()
	p, err := policy.tunView()
	if err != nil {
		e.tunStatus("error", err)
		return err
	}
	attachCtx, cancel := context.WithCancel(e.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	t.cancel = cancel
	t.backend = newTUNBackend(attachCtx, p)
	t.policy = policy
	previousNames := t.spec.Names
	t.spec = TUNSpec{Ref: tunipc.Ref{Profile: e.opts.ConnectionID, Session: fmt.Sprint(e.id), Attachment: tunipc.NewName(), Policy: 1}, Slot: e.opts.TUNSlot}
	t.spec.Prefixes, err = resourcePrefixes(p.ips)
	if err == nil {
		t.spec.Names = previousNames
		if freshNames {
			// Network discovery runs after releasing op, once attachment is ready.
			t.spec.Names = dnsNamespaces(p)
		}
		err = t.spec.Validate()
	}
	if err == nil {
		t.attachment, err = e.opts.TUNController.Attach(attachCtx, t.spec, t.backend)
	}
	if err != nil {
		cancel()
		t.backend = nil
		e.tunStatus("error", err)
		return err
	}
	if attachCtx.Err() != nil {
		err = errors.Join(attachCtx.Err(), e.detachTUN(context.Background()))
		e.tunStatus("error", err)
		return err
	}
	attachment := t.attachment
	e.tunStatus("on", nil)
	go func() {
		<-attachment.Done()
		t.op.Lock()
		defer t.op.Unlock()
		if t.attachment != attachment {
			return
		}
		e.failTUN(errors.New("TUN Worker 连接已断开"), true)
	}()
	return nil
}
func (e *Engine) detachTUN(ctx context.Context) error {
	t := &e.tunSession
	if t.attachment == nil {
		return nil
	}
	e.tunStatus("detaching", nil)
	t.cancel()
	a := t.attachment
	t.attachment = nil
	t.backend = nil
	err := a.Close(ctx)
	if err != nil {
		e.tunStatus("error", err)
	} else {
		e.tunStatus("off", nil)
	}
	return err
}
func (e *Engine) updateTUN(p *connectionPolicy, discover bool) {
	diagnostic := log.BeginOperation(e.logPrefix(), "tun_update", e.ctx.Done())
	outcome := "interrupted"
	var diagnosticErr error
	defer func() { diagnostic.Finish(outcome, diagnosticErr) }()
	t := &e.tunSession
	diagnostic.Step("wait_tun_operation_lock")
	t.op.Lock()
	defer t.op.Unlock()
	if t.attachment == nil {
		outcome = "not_attached"
		return
	}
	var err error
	if discover {
		if t.policy != p || t.backend.ctx.Err() != nil {
			outcome = "discarded"
			return
		}
		backend, ref := t.backend, t.spec.Ref
		view := backend.policy // Policy writers hold op as well as backend.mu.
		e.logf("TUN discovery request session=%s attachment=%s policy=%d", ref.Session, ref.Attachment, ref.Policy)
		diagnostic.Step("dns_namespace_discovery")
		// DialTCP may synchronously trigger relogin while holding an endpoint lock.
		// The relogin callback must be able to update/detach TUN before Dial returns.
		names := func() []string {
			t.op.Unlock()
			defer func() {
				diagnostic.Step("wait_tun_operation_lock")
				t.op.Lock()
			}()
			return discoverDNSNamespaces(backend.ctx, view)
		}()
		diagnostic.Step("validate_discovery")
		if t.backend != backend || t.spec.Ref != ref || t.policy != p || backend.ctx.Err() != nil {
			diagnosticErr = backend.ctx.Err()
			outcome = "discarded"
			return
		}
		t.spec.Policy++
		t.spec.Names = names
	} else {
		diagnostic.Step("resource_view")
		var view *connectionPolicy
		view, err = p.tunView()
		if err == nil {
			diagnostic.Step("wait_backend_policy_lock")
			t.backend.mu.Lock()
			t.backend.policy = view
			t.backend.bindings = make(map[netip.Addr][]client.DomainResource)
			t.backend.mu.Unlock()
			t.policy = p
			diagnostic.Step("build_worker_spec")
			t.spec.Policy++
			t.spec.Prefixes, err = resourcePrefixes(view.ips)
			t.spec.Names = dnsNamespaces(view)
		}
	}
	if err == nil {
		diagnostic.Step("worker_update_reply")
		e.logf("TUN update request session=%s attachment=%s policy=%d prefixes=%d names=%d", t.spec.Session, t.spec.Attachment, t.spec.Policy, len(t.spec.Prefixes), len(t.spec.Names))
		err = t.attachment.Update(e.ctx, t.spec)
	}
	diagnosticErr = err
	if err != nil {
		outcome = "failed"
		diagnostic.Step("failed_update_cleanup")
		lost := false
		select {
		case <-t.attachment.Done():
			lost = true
		default:
		}
		e.failTUN(err, lost)
	} else {
		outcome = "updated"
	}
}

// Caller holds tunSession.op; cleanup failure must never be hidden by reattach.
func (e *Engine) failTUN(cause error, workerLost bool) {
	t := &e.tunSession
	cleanupErr := e.detachTUN(context.Background())
	e.tunStatus("error", errors.Join(cause, cleanupErr))
	if !workerLost || cleanupErr != nil || !t.desired || time.Now().Before(t.recoveryAfter) || e.ctx.Err() != nil {
		return
	}
	t.recoveryAfter = time.Now().Add(time.Minute)
	e.logf("TUN Worker lost; attempting one attachment recovery")
	// Recovery can run inside the VPN relogin callback. A DNS probe here
	// would depend on the very tunnel rebuild waiting for this callback.
	_ = e.attachTUN(e.ctx, false) // attachTUN publishes the error if recovery fails.
}
