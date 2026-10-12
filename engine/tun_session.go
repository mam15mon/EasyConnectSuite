package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"EasyConnectSuite/internal/tunipc"
)

type tunSession struct {
	op         sync.Mutex
	mu         sync.Mutex
	status     TUNStatus
	attachment TUNAttachment
	backend    *tunBackend
	cancel     context.CancelFunc
	spec       TUNSpec
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
	defer t.op.Unlock()
	if !enabled {
		return e.detachTUN(ctx)
	}
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
	p, err := e.router.current.Load().tunView()
	if err != nil {
		e.tunStatus("error", err)
		return err
	}
	attachCtx, cancel := context.WithCancel(e.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	t.cancel = cancel
	t.backend = newTUNBackend(attachCtx, p)
	t.spec = TUNSpec{Ref: tunipc.Ref{Profile: e.opts.ConnectionID, Session: fmt.Sprint(e.id), Attachment: tunipc.NewName(), Policy: 1}, Slot: e.opts.TUNSlot}
	t.spec.Prefixes, err = resourcePrefixes(p.ips)
	if err == nil {
		t.spec.Names = discoverDNSNamespaces(attachCtx, p)
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
		err := errors.Join(errors.New("TUN Worker 连接已断开"), e.detachTUN(context.Background()))
		e.tunStatus("error", err)
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
	t := &e.tunSession
	t.op.Lock()
	defer t.op.Unlock()
	if t.attachment == nil {
		return
	}
	view, err := p.tunView()
	if err == nil {
		t.backend.mu.Lock()
		t.backend.policy = view
		t.backend.bindings = makeBindings()
		t.backend.mu.Unlock()
		t.spec.Policy++
		t.spec.Prefixes, err = resourcePrefixes(view.ips)
		t.spec.Names = dnsNamespaces(view)
		if discover {
			t.spec.Names = discoverDNSNamespaces(t.backend.ctx, view)
		}
		if err == nil {
			err = t.attachment.Update(e.ctx, t.spec)
		}
	}
	if err != nil {
		err = errors.Join(err, e.detachTUN(context.Background()))
		e.tunStatus("error", err)
		e.logf("TUN policy update failed: %v", err)
	}
}
