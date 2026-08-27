package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
)

// shutdownStack owns resources created during application assembly. Entries
// are closed in reverse construction order, so a dependent worker is stopped
// before the database and cache it uses. Close is idempotent and safe to call
// from both the normal shutdown path and a partial-startup error path.
type shutdownStack struct {
	mu      sync.Mutex
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
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// componentHealth is the lifecycle-only portion of the process health
// projection. Dependency reachability belongs to /ready; keeping this state
// separate makes /health useful during startup and shutdown without exposing
// connection strings, provider details, or other internal diagnostics.
type componentHealth struct {
	Started bool `json:"started"`
	Stopped bool `json:"stopped"`
}

const (
	healthPostgres          = "postgres"
	healthBillingPostgres   = "billing_postgres"
	healthRedis             = "redis"
	healthPlatformModules   = "platform_modules"
	healthBanReconciler     = "ban_reconciler"
	healthScheduler         = "scheduler"
	healthAIModules         = "ai_modules"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.entries = append(s.entries, shutdownEntry{name: name, close: closeFn})
}

func (s *shutdownStack) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	entries := append([]shutdownEntry(nil), s.entries...)
	s.mu.Unlock()

	var errs []error
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if err := entry.close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", entry.name, err))
		}
	}
	return errors.Join(errs...)
}
