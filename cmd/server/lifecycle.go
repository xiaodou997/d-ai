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
