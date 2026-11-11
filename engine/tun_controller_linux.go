package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"EasyConnectSuite/vpncore/log"
)

type linuxControllerIngress interface{ Close() error }

// LinuxTUNController owns the single headless connection's system ingress.
// Its caller supplies privileges; it never starts an authorization dialog.
type LinuxTUNController struct {
	op         sync.Mutex
	attachment *linuxTUNAttachment
	cleanupErr error
	closed     bool
	start      func(context.Context, TUNSpec, TUNBackend) (linuxControllerIngress, error)
}

func NewLinuxTUNController() *LinuxTUNController {
	return &LinuxTUNController{start: func(ctx context.Context, spec TUNSpec, backend TUNBackend) (linuxControllerIngress, error) {
		return StartLinuxTUNIngress(ctx, spec, backend)
	}}
}

func linuxControllerPrefix(spec TUNSpec) string {
	return fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment, spec.Policy)
}

func (c *LinuxTUNController) Attach(ctx context.Context, spec TUNSpec, backend TUNBackend) (_ TUNAttachment, err error) {
	op := log.BeginOperation(linuxControllerPrefix(spec), "headless_tun_attach", ctx.Done())
	defer func() { op.Finish("returned", err) }()
	op.Step("wait_controller_lock")
	c.op.Lock()
	defer c.op.Unlock()
	if c.closed {
		return nil, net.ErrClosed
	}
	if c.cleanupErr != nil {
		return nil, fmt.Errorf("Linux TUN 清理未确认: %w", c.cleanupErr)
	}
	if c.attachment != nil {
		return nil, errors.New("headless TUN 已有活动附件")
	}
	if err = spec.Validate(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	a := &linuxTUNAttachment{controller: c, ctx: ctx, spec: spec, backend: backend, done: make(chan struct{}),
		prefix: fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment)}
	a.generation = newLinuxTUNGeneration(ctx, spec, backend)
	op.Step("create_ingress")
	a.ingress, err = c.start(a.generation.ctx, spec, a.generation)
	if err != nil {
		a.generation.cancel()
		c.recordCleanupFailure(err)
		return nil, err
	}
	c.attachment = a
	if err = ctx.Err(); err != nil {
		op.Step("cleanup_cancelled_attach")
		return nil, errors.Join(err, a.closeLocked())
	}
	return a, nil
}

func (c *LinuxTUNController) recordCleanupFailure(err error) {
	var cleanup *TUNCleanupError
	if errors.As(err, &cleanup) {
		c.cleanupErr = errors.Join(c.cleanupErr, cleanup)
	}
}

// Close also returns failures observed by Engine.Stop, whose API has no error result.
func (c *LinuxTUNController) Close() (err error) {
	op := log.BeginOperation(fmt.Sprintf("[TUN pid=%d] ", os.Getpid()), "headless_tun_shutdown", nil)
	defer func() { op.Finish("returned", err) }()
	op.Step("wait_controller_lock")
	c.op.Lock()
	defer c.op.Unlock()
	c.closed = true
	if c.attachment != nil {
		op.Step("close_attachment")
		_ = c.attachment.closeLocked()
	}
	return c.cleanupErr
}

type linuxTUNAttachment struct {
	controller *LinuxTUNController
	ctx        context.Context
	prefix     string
	spec       TUNSpec
	backend    TUNBackend
	generation *linuxTUNGeneration
	ingress    linuxControllerIngress
	done       chan struct{}
	closed     bool
	closeErr   error
}

func (a *linuxTUNAttachment) Done() <-chan struct{} { return a.done }

func (a *linuxTUNAttachment) Update(ctx context.Context, spec TUNSpec) (err error) {
	op := log.BeginOperation(linuxControllerPrefix(spec), "headless_tun_update", ctx.Done())
	defer func() { op.Finish("returned", err) }()
	op.Step("wait_controller_lock")
	c := a.controller
	c.op.Lock()
	defer c.op.Unlock()
	if a.closed || c.closed {
		return net.ErrClosed
	}
	if c.cleanupErr != nil {
		return fmt.Errorf("Linux TUN 清理未确认: %w", c.cleanupErr)
	}
	if err = spec.Validate(); err != nil {
		return err
	}
	if spec.Profile != a.spec.Profile || spec.Session != a.spec.Session || spec.Attachment != a.spec.Attachment || spec.Policy <= a.spec.Policy || spec.Slot != a.spec.Slot {
		return errors.New("obsolete headless TUN update")
	}
	op.Step("close_previous_ingress")
	a.generation.cancel()
	if a.ingress != nil {
		err = a.ingress.Close()
		a.ingress = nil
		if err != nil {
			a.closeErr = errors.Join(a.closeErr, err)
			c.cleanupErr = errors.Join(c.cleanupErr, err)
			return errors.Join(ctx.Err(), err)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	a.generation = newLinuxTUNGeneration(a.ctx, spec, a.backend)
	stop := context.AfterFunc(ctx, a.generation.cancel)
	defer stop()
	op.Step("create_updated_ingress")
	a.ingress, err = c.start(a.generation.ctx, spec, a.generation)
	if err != nil {
		a.generation.cancel()
		c.recordCleanupFailure(err)
		return err
	}
	if err = errors.Join(ctx.Err(), a.generation.ctx.Err()); err != nil {
		op.Step("cleanup_cancelled_update")
		return errors.Join(err, a.closeLocked())
	}
	a.spec = spec
	return nil
}

func (a *linuxTUNAttachment) Close(ctx context.Context) (err error) {
	op := log.BeginOperation(a.prefix, "headless_tun_detach", ctx.Done())
	defer func() { op.Finish("returned", err) }()
	op.Step("wait_controller_lock")
	a.controller.op.Lock()
	defer a.controller.op.Unlock()
	op.Step("close_ingress")
	return a.closeLocked()
}

func (a *linuxTUNAttachment) closeLocked() error {
	if a.closed {
		return a.closeErr
	}
	a.closed = true
	log.Printf("%sclosing policy=%d", a.prefix, a.spec.Policy)
	a.generation.cancel()
	if a.ingress != nil {
		err := a.ingress.Close()
		a.ingress = nil
		a.closeErr = errors.Join(a.closeErr, err)
		a.controller.cleanupErr = errors.Join(a.controller.cleanupErr, err)
	}
	if a.controller.attachment == a {
		a.controller.attachment = nil
	}
	close(a.done)
	return a.closeErr
}

// Each ingress receives a separate generation. Calls never hold controller locks:
// backend Dial or Write may synchronously publish a replacement VPN policy.
type linuxTUNGeneration struct {
	ctx     context.Context
	cancel  context.CancelFunc
	spec    TUNSpec
	backend TUNBackend
}

func newLinuxTUNGeneration(ctx context.Context, spec TUNSpec, backend TUNBackend) *linuxTUNGeneration {
	ctx, cancel := context.WithCancel(ctx)
	return &linuxTUNGeneration{ctx: ctx, cancel: cancel, spec: spec, backend: backend}
}

func (g *linuxTUNGeneration) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	if err := g.ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	conn, err := g.backend.Dial(ctx, network, address)
	if err != nil {
		return nil, err
	}
	retiring := &linuxRetiringConn{Conn: conn, generation: g}
	if err = errors.Join(ctx.Err(), g.ctx.Err()); err != nil {
		_ = retiring.Close()
		return nil, err
	}
	return retiring, nil
}

func (g *linuxTUNGeneration) DNS(ctx context.Context, data []byte) ([]byte, []string, error) {
	if err := g.ctx.Err(); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	answer, routes, err := g.backend.DNS(ctx, data)
	if cancelled := errors.Join(ctx.Err(), g.ctx.Err()); cancelled != nil {
		return nil, nil, cancelled
	}
	return answer, routes, err
}

type linuxRetiringConn struct {
	net.Conn
	generation *linuxTUNGeneration
	closed     atomic.Bool
}

func (c *linuxRetiringConn) Read(p []byte) (int, error) {
	if c.closed.Load() || c.generation.ctx.Err() != nil {
		return 0, net.ErrClosed
	}
	return c.Conn.Read(p)
}

func (c *linuxRetiringConn) Write(p []byte) (int, error) {
	if c.closed.Load() || c.generation.ctx.Err() != nil {
		return 0, net.ErrClosed
	}
	return c.Conn.Write(p)
}

func (c *linuxRetiringConn) CloseWrite() error {
	if c.closed.Load() || c.generation.ctx.Err() != nil {
		return net.ErrClosed
	}
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return c.Close()
}

func (c *linuxRetiringConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		go func() {
			op := log.BeginOperation(linuxControllerPrefix(c.generation.spec), "headless_retired_stream_close", c.generation.ctx.Done())
			op.Step("stream_close")
			err := c.Conn.Close()
			outcome := "closed"
			if err != nil {
				outcome = "failed"
			}
			op.Finish(outcome, err)
		}()
	}
	return nil
}
