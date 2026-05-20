package routing

import (
	"sync"
	"time"
)

const (
	defaultFailThreshold    = 5
	defaultBaseOpenDuration = 60 * time.Second
	maxOpenDuration         = 30 * time.Minute
)

// targetEntry is the mutable per-target FSM state. Access is serialised by
// InMemoryTracker.mu.
type targetEntry struct {
	kind        TargetKind
	state       HealthState
	consecFail  int
	openCount   int       // monotone: incremented on each CLOSED/HALF_OPEN→OPEN transition for backoff
	probing     bool      // true = a HALF_OPEN probe request is in flight
	openedAt    time.Time
	nextProbeAt time.Time
}

// InMemoryTracker is a single-process HealthTracker. State resets on restart.
// Use NewRedisHealthTracker to add multi-node synchronisation.
type InMemoryTracker struct {
	mu            sync.Mutex
	entries       map[string]*targetEntry
	failThreshold int
	baseDuration  time.Duration
}

// NewInMemoryTracker creates a tracker with the given thresholds.
func NewInMemoryTracker(failThreshold int, baseDuration time.Duration) *InMemoryTracker {
	if failThreshold <= 0 {
		failThreshold = defaultFailThreshold
	}
	if baseDuration <= 0 {
		baseDuration = defaultBaseOpenDuration
	}
	return &InMemoryTracker{
		entries:       make(map[string]*targetEntry),
		failThreshold: failThreshold,
		baseDuration:  baseDuration,
	}
}

// DefaultInMemoryTracker returns a tracker with production defaults:
// 5 consecutive failures → OPEN; 60 s base probe interval; max 30 min.
func DefaultInMemoryTracker() *InMemoryTracker {
	return NewInMemoryTracker(defaultFailThreshold, defaultBaseOpenDuration)
}

// RecordSuccess resets state to CLOSED and clears all counters.
func (t *InMemoryTracker) RecordSuccess(targetID string, kind TargetKind) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.getOrCreate(targetID, kind)
	e.consecFail = 0
	e.state = StateClosed
	e.probing = false
	e.openCount = 0
	e.openedAt = time.Time{}
	e.nextProbeAt = time.Time{}
}

// RecordFailure applies the failure transition to the FSM.
func (t *InMemoryTracker) RecordFailure(targetID string, kind TargetKind) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.applyFailure(t.getOrCreate(targetID, kind))
}

// applyFailure drives the FSM forward on a failure event. Caller must hold mu.
func (t *InMemoryTracker) applyFailure(e *targetEntry) {
	switch e.state {
	case StateClosed:
		e.consecFail++
		if e.consecFail >= t.failThreshold {
			now := time.Now()
			e.state = StateOpen
			e.openedAt = now
			e.openCount++
			e.nextProbeAt = now.Add(t.openDuration(e.openCount))
		}
	case StateHalfOpen:
		// Probe failed → re-open with exponential backoff.
		now := time.Now()
		e.state = StateOpen
		e.openedAt = now
		e.openCount++
		e.nextProbeAt = now.Add(t.openDuration(e.openCount))
		e.probing = false
	case StateOpen:
		// Already open; the new failure doesn't change anything.
	}
}

// IsBlocked reports whether the target should be skipped.
// OPEN with an elapsed probe window atomically transitions to HALF_OPEN and
// returns false for the first concurrent caller so it can act as the probe.
func (t *InMemoryTracker) IsBlocked(targetID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[targetID]
	if !ok {
		return false
	}
	switch e.state {
	case StateClosed:
		return false
	case StateOpen:
		if time.Now().Before(e.nextProbeAt) {
			return true
		}
		// Probe window elapsed → claim the probe slot.
		e.state = StateHalfOpen
		e.probing = true
		return false
	case StateHalfOpen:
		if e.probing {
			return true // another request is already probing
		}
		e.probing = true
		return false
	}
	return false
}

// State returns the current FSM state for a target. Returns StateClosed for
// unknown targets. Used by RedisHealthTracker to detect state changes.
func (t *InMemoryTracker) State(targetID string) HealthState {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.entries[targetID]; ok {
		return e.state
	}
	return StateClosed
}

// StateOf satisfies HealthTracker.StateOf. Unlike IsBlocked it is a pure read
// that never claims the HALF_OPEN probe slot.
func (t *InMemoryTracker) StateOf(targetID string) HealthState {
	return t.State(targetID)
}

// entrySnapshot returns a copy of the entry for targetID, or (zero, false) if
// not tracked.
func (t *InMemoryTracker) entrySnapshot(targetID string) (targetEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.entries[targetID]; ok {
		return *e, true
	}
	return targetEntry{}, false
}

// syncFromEvent applies a remote state-change event received via Redis Pub/Sub.
func (t *InMemoryTracker) syncFromEvent(e healthEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.getOrCreate(e.TargetID, e.Kind)
	switch e.Event {
	case "opened":
		// Accept the remote signal if the remote openCount is higher (fresher trip).
		if entry.state != StateOpen || e.OpenCount > entry.openCount {
			now := time.Now()
			entry.state = StateOpen
			entry.openedAt = now
			entry.openCount = e.OpenCount
			if e.NextProbeAtNS > 0 {
				entry.nextProbeAt = time.Unix(0, e.NextProbeAtNS)
			}
			entry.probing = false
		}
	case "closed":
		entry.state = StateClosed
		entry.consecFail = 0
		entry.openCount = 0
		entry.probing = false
		entry.openedAt = time.Time{}
		entry.nextProbeAt = time.Time{}
	}
}

// Snapshot returns a read-only view of all tracked targets.
func (t *InMemoryTracker) Snapshot() []HealthRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]HealthRecord, 0, len(t.entries))
	for id, e := range t.entries {
		rec := HealthRecord{
			TargetID:   id,
			Kind:       e.kind,
			State:      e.state,
			StateStr:   e.state.String(),
			ConsecFail: e.consecFail,
		}
		if !e.openedAt.IsZero() {
			ts := e.openedAt
			rec.OpenedAt = &ts
		}
		if !e.nextProbeAt.IsZero() {
			ts := e.nextProbeAt
			rec.NextProbeAt = &ts
		}
		out = append(out, rec)
	}
	return out
}

// openDuration returns the OPEN wait time for the nth trip (1-based),
// applying binary exponential backoff capped at maxOpenDuration.
func (t *InMemoryTracker) openDuration(openCount int) time.Duration {
	if openCount <= 0 {
		openCount = 1
	}
	d := t.baseDuration
	for i := 1; i < openCount; i++ {
		d *= 2
		if d >= maxOpenDuration {
			return maxOpenDuration
		}
	}
	if d > maxOpenDuration {
		return maxOpenDuration
	}
	return d
}

func (t *InMemoryTracker) getOrCreate(targetID string, kind TargetKind) *targetEntry {
	e, ok := t.entries[targetID]
	if !ok {
		e = &targetEntry{kind: kind}
		t.entries[targetID] = e
	}
	return e
}
