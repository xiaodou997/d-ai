package routing

import (
	"sync"
	"testing"
	"time"
)

// newFastTracker returns a tracker with a low threshold and tiny base duration
// so tests don't need real-time waits.
func newFastTracker(threshold int, base time.Duration) *InMemoryTracker {
	return NewInMemoryTracker(threshold, base)
}

// ── basic state machine ──────────────────────────────────────────────────────

func TestHealthTracker_OpenAfterThreshold(t *testing.T) {
	tr := newFastTracker(3, time.Second)
	const id = "dep-1"

	for i := 1; i <= 2; i++ {
		tr.RecordFailure(id, TargetDeployment)
		if tr.IsBlocked(id) {
			t.Fatalf("blocked too early after %d failures", i)
		}
	}
	tr.RecordFailure(id, TargetDeployment)
	if !tr.IsBlocked(id) {
		t.Fatal("should be blocked after threshold failures")
	}
	if tr.State(id) != StateOpen {
		t.Fatalf("expected StateOpen, got %s", tr.State(id))
	}
}

func TestHealthTracker_RecordSuccessResets(t *testing.T) {
	tr := newFastTracker(2, time.Second)
	tr.RecordFailure("d", TargetDeployment)
	tr.RecordSuccess("d", TargetDeployment)
	if tr.IsBlocked("d") {
		t.Fatal("should not be blocked after success reset")
	}
	// Counter should be zero — a single new failure must not re-open.
	tr.RecordFailure("d", TargetDeployment)
	if tr.IsBlocked("d") {
		t.Fatal("one failure after reset should not open the circuit")
	}
}

func TestHealthTracker_HalfOpenAfterCooldown(t *testing.T) {
	tr := newFastTracker(2, 10*time.Millisecond)
	tr.RecordFailure("d", TargetDeployment)
	tr.RecordFailure("d", TargetDeployment) // → OPEN
	if !tr.IsBlocked("d") {
		t.Fatal("expected blocked right after opening")
	}
	time.Sleep(20 * time.Millisecond)

	// First caller after cooldown should get the probe (not blocked).
	if tr.IsBlocked("d") {
		t.Fatal("probe window elapsed; first caller should not be blocked")
	}
	if tr.State("d") != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %s", tr.State("d"))
	}
}

func TestHealthTracker_ProbeSuccessCloses(t *testing.T) {
	tr := newFastTracker(2, 5*time.Millisecond)
	tr.RecordFailure("d", TargetDeployment)
	tr.RecordFailure("d", TargetDeployment) // OPEN
	time.Sleep(10 * time.Millisecond)
	tr.IsBlocked("d") // claim probe → HALF_OPEN

	tr.RecordSuccess("d", TargetDeployment)
	if tr.State("d") != StateClosed {
		t.Fatalf("probe success should close circuit; got %s", tr.State("d"))
	}
	if tr.IsBlocked("d") {
		t.Fatal("closed circuit must not be blocked")
	}
}

func TestHealthTracker_ProbeFailureReopens(t *testing.T) {
	tr := newFastTracker(2, 5*time.Millisecond)
	tr.RecordFailure("d", TargetDeployment)
	tr.RecordFailure("d", TargetDeployment) // OPEN (openCount=1)
	time.Sleep(10 * time.Millisecond)
	tr.IsBlocked("d") // claim probe → HALF_OPEN

	tr.RecordFailure("d", TargetDeployment) // probe failed → OPEN (openCount=2)
	if tr.State("d") != StateOpen {
		t.Fatalf("probe failure should reopen circuit; got %s", tr.State("d"))
	}
	// Backoff: next probe should be at baseDuration * 2^(openCount-1) = 5ms * 2 = 10ms
	if tr.IsBlocked("d") == false {
		t.Fatal("should be blocked immediately after re-open")
	}
}

// ── exponential backoff ──────────────────────────────────────────────────────

func TestHealthTracker_ExponentialBackoff(t *testing.T) {
	base := 10 * time.Millisecond
	tr := newFastTracker(1, base)

	openAndProbe := func() {
		tr.RecordFailure("d", TargetDeployment)               // threshold=1 → OPEN
		for tr.State("d") == StateOpen && tr.IsBlocked("d") { // wait for probe window
			time.Sleep(base)
		}
		tr.IsBlocked("d")                       // claim probe
		tr.RecordFailure("d", TargetDeployment) // probe fails → OPEN with backoff
	}

	// First open: openCount=1, next wait=base (10ms)
	openAndProbe()
	snap1, _ := tr.entrySnapshot("d")
	wait1 := time.Until(snap1.nextProbeAt)

	// Second open: openCount=2, next wait=2*base (20ms)
	time.Sleep(base + 2*time.Millisecond)
	openAndProbe()
	snap2, _ := tr.entrySnapshot("d")
	wait2 := time.Until(snap2.nextProbeAt)

	if wait2 <= wait1 {
		t.Fatalf("exponential backoff not increasing: wait1=%v wait2=%v", wait1, wait2)
	}
}

func TestHealthTracker_BackoffCappedAtMax(t *testing.T) {
	tr := newFastTracker(1, 5*time.Millisecond)
	// Drive up openCount by simulating many probe failures.
	for i := 0; i < 30; i++ {
		tr.mu.Lock()
		e := tr.getOrCreate("d", TargetDeployment)
		e.openCount = i + 1
		tr.mu.Unlock()
	}
	dur := tr.openDuration(30)
	if dur != maxOpenDuration {
		t.Fatalf("expected maxOpenDuration=%v, got %v", maxOpenDuration, dur)
	}
}

// ── concurrent probe safety ──────────────────────────────────────────────────

func TestHealthTracker_OnlyOneProbeConcurrent(t *testing.T) {
	tr := newFastTracker(2, time.Millisecond)
	tr.RecordFailure("d", TargetDeployment)
	tr.RecordFailure("d", TargetDeployment) // OPEN
	time.Sleep(5 * time.Millisecond)        // let probe window elapse

	const goroutines = 100
	notBlocked := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if !tr.IsBlocked("d") {
				mu.Lock()
				notBlocked++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if notBlocked != 1 {
		t.Fatalf("exactly 1 goroutine should be the probe; got %d", notBlocked)
	}
}

// ── snapshot ─────────────────────────────────────────────────────────────────

func TestHealthTracker_Snapshot(t *testing.T) {
	tr := newFastTracker(3, time.Second)
	tr.RecordFailure("a", TargetDeployment)
	tr.RecordFailure("b", TargetCredential)

	snap := tr.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 records, got %d", len(snap))
	}
	for _, r := range snap {
		if r.State != StateClosed {
			t.Fatalf("expected StateClosed for %s, got %s", r.TargetID, r.StateStr)
		}
	}
}

// ── syncFromEvent (Redis simulation) ─────────────────────────────────────────

func TestHealthTracker_SyncFromEvent_Opened(t *testing.T) {
	tr := newFastTracker(5, time.Second)
	future := time.Now().Add(5 * time.Minute)
	tr.syncFromEvent(healthEvent{
		TargetID:      "remote-dep",
		Kind:          TargetDeployment,
		Event:         "opened",
		NextProbeAtNS: future.UnixNano(),
		OpenCount:     1,
	})
	if tr.State("remote-dep") != StateOpen {
		t.Fatalf("synced open event should set StateOpen, got %s", tr.State("remote-dep"))
	}
	if !tr.IsBlocked("remote-dep") {
		t.Fatal("opened target should be blocked")
	}
}

func TestHealthTracker_SyncFromEvent_Closed(t *testing.T) {
	tr := newFastTracker(2, time.Second)
	tr.RecordFailure("dep", TargetDeployment)
	tr.RecordFailure("dep", TargetDeployment) // OPEN

	tr.syncFromEvent(healthEvent{TargetID: "dep", Kind: TargetDeployment, Event: "closed"})
	if tr.State("dep") != StateClosed {
		t.Fatalf("closed sync should reset to StateClosed, got %s", tr.State("dep"))
	}
}
