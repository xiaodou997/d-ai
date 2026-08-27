package gateway

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/dai/internal/lifecycle"
)

// runtimeAuthToucher owns best-effort API-key telemetry writes. Touches keep
// the request context for normal cancellation, while Stop cancels every
// in-flight touch and waits before the runtime database dependency is closed.
type runtimeAuthToucher struct {
	touch func(context.Context, pgtype.UUID) error

	mu      sync.Mutex
	started bool
	fenced  bool
	stopped bool
	nextID  uint64
	pending map[uint64]context.CancelFunc
	wg      sync.WaitGroup
}

var _ lifecycle.Component = (*runtimeAuthToucher)(nil)

func newRuntimeAuthToucher(touch func(context.Context, pgtype.UUID) error) *runtimeAuthToucher {
	return &runtimeAuthToucher{touch: touch, pending: make(map[uint64]context.CancelFunc)}
}

func (t *runtimeAuthToucher) Start() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if !t.started && !t.fenced {
		t.started = true
	}
	t.mu.Unlock()
}

func (t *runtimeAuthToucher) Enqueue(ctx context.Context, keyID pgtype.UUID) {
	if t == nil || t.touch == nil || !keyID.Valid {
		return
	}
	if ctx == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	touchCtx, cancel := context.WithTimeout(ctx, runtimeAuthTouchTimeout)
	t.mu.Lock()
	if !t.started || t.fenced {
		t.mu.Unlock()
		cancel()
		return
	}
	t.nextID++
	id := t.nextID
	if t.pending == nil {
		t.pending = make(map[uint64]context.CancelFunc)
	}
	t.pending[id] = cancel
	t.wg.Add(1)
	t.mu.Unlock()

	go func() {
		defer t.wg.Done()
		defer cancel()
		_ = t.touch(touchCtx, keyID)
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
	}()
}

// Stop cancels and waits for all telemetry writes. A later call may provide a
// longer context after an earlier shutdown deadline expires.
func (t *runtimeAuthToucher) Stop(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	t.mu.Lock()
	t.fenced = true
	cancels := make([]context.CancelFunc, 0, len(t.pending))
	for _, cancel := range t.pending {
		cancels = append(cancels, cancel)
	}
	t.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		t.mu.Lock()
		t.stopped = true
		t.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Health returns a lock-safe lifecycle snapshot for runtime health projection.
func (t *runtimeAuthToucher) Health() lifecycle.HealthSnapshot {
	if t == nil {
		return lifecycle.HealthSnapshot{}
	}
	t.mu.Lock()
	started, stopped := t.started, t.stopped
	t.mu.Unlock()
	return lifecycle.HealthSnapshot{Started: started, Stopped: stopped}
}
