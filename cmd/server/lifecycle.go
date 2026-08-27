package main

import (
	"context"
	"errors"
	"fmt"
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
