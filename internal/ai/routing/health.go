// Package routing implements route selection and health tracking.
package routing

import "time"

// TargetKind distinguishes upstream accounts from OAuth pools in
// health tracking. Both kinds share the same three-state FSM.
type TargetKind int

const (
	TargetAccount TargetKind = iota
	TargetPool
)

// HealthState is the three-state circuit-breaker FSM value.
type HealthState int

const (
	StateClosed   HealthState = iota // healthy; requests pass through
	StateOpen                        // tripped; requests are blocked
	StateHalfOpen                    // probing; exactly one request is let through
)

func (s HealthState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// HealthRecord is a read-only snapshot of one tracked target for admin APIs.
type HealthRecord struct {
	TargetID    string      `json:"target_id"`
	Kind        TargetKind  `json:"kind"`
	State       HealthState `json:"state"`
	StateStr    string      `json:"state_str"`
	ConsecFail  int         `json:"consecutive_failures"`
	OpenedAt    *time.Time  `json:"opened_at,omitempty"`
	NextProbeAt *time.Time  `json:"next_probe_at,omitempty"`
}

// HealthTracker tracks upstream target health with a three-state FSM.
// Implementations must be safe for concurrent use.
type HealthTracker interface {
	// RecordSuccess resets the failure counter. If the target was HALF_OPEN
	// (probe succeeded), it transitions back to CLOSED.
	RecordSuccess(targetID string, kind TargetKind)

	// RecordFailure records a health failure. Transitions CLOSED→OPEN after
	// the failure threshold, and HALF_OPEN→OPEN (with exponential backoff) on
	// a failed probe.
	RecordFailure(targetID string, kind TargetKind)

	// IsBlocked returns true when the target should not receive traffic:
	//   CLOSED    → false
	//   OPEN      → true, unless the probe window has elapsed; on first expiry
	//               the caller atomically claims the probe slot (→ HALF_OPEN)
	//               and receives false.
	//   HALF_OPEN → false for exactly one concurrent caller; true for the rest.
	// probeLease bounds the claim so a crashed worker cannot strand the target.
	IsBlocked(targetID string, probeLease time.Duration) bool

	// ReleaseProbe abandons a claimed HALF_OPEN probe without treating the
	// target as healthy or unhealthy. Call it when the request is canceled or
	// fails locally after claiming the probe but before an upstream verdict.
	ReleaseProbe(targetID string)

	// StateOf returns the current health state of a target. Unlike IsBlocked,
	// this is a pure read that does not claim the HALF_OPEN probe slot. Use it
	// for scoring rather than for gating admission.
	StateOf(targetID string) HealthState

	// StatesOf returns a read-only state snapshot for multiple targets without
	// claiming HALF_OPEN probes.
	StatesOf(targetIDs []string) map[string]HealthState

	// Snapshot returns a point-in-time view of all tracked targets.
	Snapshot() []HealthRecord
}
