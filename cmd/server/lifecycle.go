package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	lifecyclepkg "xiaodou/dai/internal/lifecycle"
)

// shutdownStack owns resources created during application assembly. Entries
// are closed in reverse construction order, so a dependent worker is stopped
// before the database and cache it uses. Close is idempotent and safe to call
// from both the normal shutdown path and a partial-startup error path.
type shutdownStack struct {
	mu      sync.Mutex
	closeMu sync.Mutex
	entries []shutdownEntry
	closed  bool
}

type shutdownEntry struct {
	name  string
	close func(context.Context) error
}

// periodicWorker owns a context-driven background loop. The cancellation is
// idempotent, while every Stop call still gets a chance to wait with its own
// deadline after an earlier short shutdown deadline expires.
type periodicWorker struct {
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
	stateMu sync.RWMutex
	started bool
	stopped bool
}

var _ lifecyclepkg.Component = (*periodicWorker)(nil)

// componentHealth aliases the shared worker contract. Dependency reachability
// belongs to /ready; keeping this state separate makes /health useful during
// startup and shutdown without exposing connection strings, provider details,
// or other internal diagnostics.
type componentHealth = lifecyclepkg.HealthSnapshot

const (
	healthPostgres          = "postgres"
	healthBillingPostgres   = "billing_postgres"
	healthRedis             = "redis"
	healthPlatformModules   = "platform_modules"
	healthBanReconciler     = "ban_reconciler"
	healthScheduler         = "scheduler"
	healthAIModules         = "ai_modules"
	healthRuntimeGateway    = "runtime_gateway"
	healthAsyncTasks        = "async_tasks"
	healthDataCleanup       = "data_cleanup"
	healthHourlyImage       = "hourly_image_cleanup"
	healthHourlyFile        = "hourly_file_cleanup"
	healthHourlyAuthSession = "hourly_auth_session_cleanup"
	healthHourlyActivation  = "hourly_activation_cleanup"
	healthHTTPPublic        = "http_public"
	healthHTTPManagement    = "http_management"
)

// lifecycleHealth records process-owned component state at the composition
// root. Components are marked only after successful construction/start and
// after their shutdown callback returns, so the projection also describes a
// partial-startup failure accurately.
type lifecycleHealth struct {
	mu         sync.RWMutex
	components map[string]componentHealth
}

func newLifecycleHealth() *lifecycleHealth {
	return &lifecycleHealth{components: make(map[string]componentHealth)}
}

func (h *lifecycleHealth) MarkStarted(name string) {
	if h == nil || name == "" {
		return
	}
	h.mu.Lock()
	if h.components == nil {
		h.components = make(map[string]componentHealth)
	}
	state := h.components[name]
	state.Started = true
	state.Stopped = false
	h.components[name] = state
	h.mu.Unlock()
}

func (h *lifecycleHealth) MarkStopped(name string) {
	if h == nil || name == "" {
		return
	}
	h.mu.Lock()
	if h.components == nil {
		h.components = make(map[string]componentHealth)
	}
	state := h.components[name]
	state.Stopped = true
	h.components[name] = state
	h.mu.Unlock()
}

func (h *lifecycleHealth) Snapshot() map[string]componentHealth {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	snapshot := make(map[string]componentHealth, len(h.components))
	for name, state := range h.components {
		snapshot[name] = state
	}
	return snapshot
}

func newHealthHandler(version string, scheduler func() any, lifecycle *lifecycleHealth) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		schedulerHealth := any(map[string]any{
			"started": false,
			"stopped": false,
			"tasks":   map[string]any{},
		})
		if scheduler != nil {
			if current := scheduler(); current != nil {
				schedulerHealth = current
			}
		}
		components := map[string]componentHealth{}
		if lifecycle != nil {
			if snapshot := lifecycle.Snapshot(); snapshot != nil {
				components = snapshot
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     "ok",
			"version":    version,
			"scheduler":  schedulerHealth,
			"components": components,
		})
	})
}

func (w *periodicWorker) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.once.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	if w.done == nil {
		return nil
	}
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Health returns a lock-safe lifecycle snapshot for the root health
// projection. A worker is stopped only after its loop has actually exited.
func (w *periodicWorker) Health() lifecyclepkg.HealthSnapshot {
	if w == nil {
		return lifecyclepkg.HealthSnapshot{}
	}
	w.stateMu.RLock()
	started, stopped := w.started, w.stopped
	w.stateMu.RUnlock()
	return lifecyclepkg.HealthSnapshot{Started: started, Stopped: stopped}
}

func (w *periodicWorker) markStopped() {
	if w == nil {
		return
	}
	w.stateMu.Lock()
	w.stopped = true
	w.stateMu.Unlock()
}

func registerPeriodicWorker(stack *shutdownStack, lifecycle *lifecycleHealth, name, healthName string, worker *periodicWorker) {
	if stack == nil || worker == nil {
		return
	}
	if lifecycle != nil {
		lifecycle.MarkStarted(healthName)
	}
	stack.Add(name, func(ctx context.Context) error {
		err := worker.Stop(ctx)
		if err == nil && lifecycle != nil {
			lifecycle.MarkStopped(healthName)
		}
		return err
	})
}

func (s *shutdownStack) Add(name string, closeFn func(context.Context) error) {
	if closeFn == nil {
		return
	}
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.entries = append(s.entries, shutdownEntry{name: name, close: closeFn})
}

func (s *shutdownStack) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	entries := append([]shutdownEntry(nil), s.entries...)
	s.mu.Unlock()

	var errs []error
	remaining := entries
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if err := entry.close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", entry.name, err))
			// A lower-level dependency must not be released while a dependent
			// component is still alive. Keep this entry and all older entries
			// registered so a later Close call can retry with a fresh deadline.
			remaining = entries[:i+1]
			break
		}
		remaining = entries[:i]
	}
	s.mu.Lock()
	s.entries = remaining
	if len(errs) == 0 {
		s.closed = true
	}
	s.mu.Unlock()
	return errors.Join(errs...)
}
