package serving

import (
	"context"
	"errors"
	"sync"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Phase-timeout cause sentinels. They are attached via context.Cause so the
// classifier can tell connect / first-byte / idle / max-duration timeouts
// apart — each maps to a different retry decision.
var (
	ErrConnectTimeout   = errors.New("connect timeout: no response headers from upstream in time")
	ErrFirstByteTimeout = errors.New("first-byte timeout: no response body from upstream in time")
	ErrIdleTimeout      = errors.New("idle timeout: upstream stream stalled between chunks")
	ErrMaxDuration      = errors.New("max-duration timeout: response exceeded total time budget")
)

// deadlineController drives the 三段式 (connect → first-byte → idle) timeout
// state machine for one upstream attempt, plus an absolute max-duration cap.
//
// It owns the single cancelable context bound to the HTTP request, so any
// phase timeout cancels the in-flight header wait or body read. Crucially the
// context is NOT a fixed deadline: the phase timer is re-armed on every state
// transition (and on every streamed chunk), so a legitimately long stream is
// bounded by idle gaps + total duration rather than a single up-front budget.
//
// Lifecycle, driven by ExecuteStep.runAttempt + relay:
//
//	dc := newDeadlineController(parentCtx, cand.Timeouts)
//	defer dc.stop()
//	resp, err := transport.Do(dc.ctx, ...)   // connect phase armed
//	dc.headersReceived()                     // → first-byte phase
//	// streaming: dc.firstByte() on first chunk, dc.chunkReceived() after
//	// non-streaming: dc.syncBodyPhase() before reading the body
//
// All transition methods are safe to call from the relay goroutine while the
// AfterFunc timers fire on their own goroutines.
type deadlineController struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	t      domain.RouteTimeouts

	mu       sync.Mutex
	phase    *time.Timer // connect → first-byte → idle
	maxTimer *time.Timer // absolute max-duration
	stopped  bool
}

// newDeadlineController returns a controller already armed for the connect
// phase. The caller passes dc.ctx to transport.Do.
func newDeadlineController(parent context.Context, t domain.RouteTimeouts) *deadlineController {
	ctx, cancel := context.WithCancelCause(parent)
	dc := &deadlineController{ctx: ctx, cancel: cancel, t: t}
	dc.phase = time.AfterFunc(t.Connect, func() { dc.cancel(ErrConnectTimeout) })
	return dc
}

// armPhase stops the current phase timer and starts a fresh one.
func (dc *deadlineController) armPhase(d time.Duration, cause error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.stopped {
		return
	}
	if dc.phase != nil {
		dc.phase.Stop()
	}
	dc.phase = time.AfterFunc(d, func() { dc.cancel(cause) })
}

// headersReceived transitions connect → first-byte phase.
func (dc *deadlineController) headersReceived() {
	dc.armPhase(dc.t.FirstByte, ErrFirstByteTimeout)
}

// firstByte transitions first-byte → idle phase and starts the absolute
// max-duration cap. Call once, when the first streamed chunk arrives.
func (dc *deadlineController) firstByte() {
	dc.mu.Lock()
	if !dc.stopped && dc.maxTimer == nil {
		dc.maxTimer = time.AfterFunc(dc.t.MaxDuration, func() { dc.cancel(ErrMaxDuration) })
	}
	dc.mu.Unlock()
	dc.armPhase(dc.t.Idle, ErrIdleTimeout)
}

// chunkReceived resets the idle timer. Call on every streamed chunk after the
// first.
func (dc *deadlineController) chunkReceived() {
	dc.armPhase(dc.t.Idle, ErrIdleTimeout)
}

// syncBodyPhase switches from the first-byte timer to a single max-duration
// bound for the rest of a non-streaming body read (idle has no meaning without
// chunks). Call only after the first body bytes are observed.
func (dc *deadlineController) syncBodyPhase() {
	dc.armPhase(dc.t.MaxDuration, ErrMaxDuration)
}

// stop halts all timers and releases the context. Idempotent; call once via
// defer. Close resp.Body before stop() so a completed stream's connection can
// still be pooled.
func (dc *deadlineController) stop() {
	dc.mu.Lock()
	dc.stopped = true
	if dc.phase != nil {
		dc.phase.Stop()
	}
	if dc.maxTimer != nil {
		dc.maxTimer.Stop()
	}
	dc.mu.Unlock()
	dc.cancel(nil)
}

// cause returns the phase-timeout sentinel that aborted this attempt, or nil
// when the context was not cancelled by a phase timeout (plain success,
// client disconnect, or parent cancellation).
func (dc *deadlineController) cause() error {
	switch c := context.Cause(dc.ctx); c {
	case ErrConnectTimeout, ErrFirstByteTimeout, ErrIdleTimeout, ErrMaxDuration:
		return c
	default:
		return nil
	}
}
