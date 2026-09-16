package serving

import (
	"maps"
	"time"
)

// RequestTiming captures the request-level lifecycle milestones needed to
// derive user-facing timing metrics. StartedAt still lives on Request as the
// canonical gateway ingress time; this struct tracks the remaining points.
type RequestTiming struct {
	FirstAttemptStartedAt time.Time
	FirstResponseByteAt   time.Time
	CompletedAt           time.Time
}

func (r *Request) MarkFirstAttemptStarted(at time.Time) {
	if r == nil || at.IsZero() || !r.Timing.FirstAttemptStartedAt.IsZero() {
		return
	}
	r.Timing.FirstAttemptStartedAt = at
}

func (r *Request) MarkFirstResponseByte(at time.Time) {
	if r == nil || at.IsZero() {
		return
	}
	if r.Timing.FirstResponseByteAt.IsZero() {
		r.Timing.FirstResponseByteAt = at
	}
	if n := len(r.Attempts); n > 0 && r.Attempts[n-1].FirstByteMs == 0 && !r.Attempts[n-1].TransportStartedAt.IsZero() {
		r.Attempts[n-1].FirstByteMs = durationMs(r.Attempts[n-1].TransportStartedAt, at)
	}
}

func (r *Request) MarkCompleted(at time.Time) {
	if r == nil || at.IsZero() || !r.Timing.CompletedAt.IsZero() {
		return
	}
	r.Timing.CompletedAt = at
	if n := len(r.Attempts); n > 0 {
		r.CompleteAttempt(n-1, at)
	}
}

func (r *Request) CompleteAttempt(index int, at time.Time) {
	if r == nil || at.IsZero() || index < 0 || index >= len(r.Attempts) {
		return
	}
	attempt := &r.Attempts[index]
	if !attempt.CompletedAt.IsZero() {
		return
	}
	attempt.UsageEvidence = r.UsageEvidence
	attempt.UsageEvidence.Fields = maps.Clone(r.UsageEvidence.Fields)
	attempt.ProviderTerminalState = r.ProviderTerminalState
	attempt.CompletedAt = at
	if !attempt.StartedAt.IsZero() {
		attempt.TotalMs = durationMs(attempt.StartedAt, at)
	}
}

func (r *Request) RequestTotalMs() (int, bool) {
	if r == nil || r.StartedAt.IsZero() || r.Timing.CompletedAt.IsZero() {
		return 0, false
	}
	return durationMs(r.StartedAt, r.Timing.CompletedAt), true
}

func (r *Request) RequestSetupMs() (int, bool) {
	if r == nil || r.StartedAt.IsZero() || r.Timing.FirstAttemptStartedAt.IsZero() {
		return 0, false
	}
	return durationMs(r.StartedAt, r.Timing.FirstAttemptStartedAt), true
}

func (r *Request) FirstResponseByteDurationMs() (int, bool) {
	if r == nil || r.StartedAt.IsZero() || r.Timing.FirstResponseByteAt.IsZero() {
		return 0, false
	}
	return durationMs(r.StartedAt, r.Timing.FirstResponseByteAt), true
}

func (r *Request) ResponseTailMs() (int, bool) {
	if r == nil || r.Timing.FirstResponseByteAt.IsZero() || r.Timing.CompletedAt.IsZero() {
		return 0, false
	}
	return durationMs(r.Timing.FirstResponseByteAt, r.Timing.CompletedAt), true
}

func (r *Request) FinalAttemptHeaderMs() (int, bool) {
	if r == nil || len(r.Attempts) == 0 {
		return 0, false
	}
	last := r.Attempts[len(r.Attempts)-1]
	if last.LatencyMs == 0 && last.TransportStartedAt.IsZero() {
		return 0, false
	}
	return last.LatencyMs, true
}

func (r *Request) FinalAttemptTotalMs() (int, bool) {
	if r == nil || len(r.Attempts) == 0 {
		return 0, false
	}
	last := r.Attempts[len(r.Attempts)-1]
	if last.TotalMs == 0 && last.CompletedAt.IsZero() {
		return 0, false
	}
	return last.TotalMs, true
}

func durationMs(start, end time.Time) int {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return int(end.Sub(start).Milliseconds())
}

// MarkFirstOutput keeps request latency separate from the final attempt's
// latency used by upstream scoring. Failed attempts and setup count for users.
func (r *Request) MarkFirstOutput(at, attemptStart time.Time) {
	if r == nil || r.FirstTokenMs != 0 {
		return
	}
	start := r.StartedAt
	if start.IsZero() {
		start = attemptStart
	}
	r.FirstTokenMs = max(1, durationMs(start, at))
	if n := len(r.Attempts); n > 0 && r.Attempts[n-1].FirstOutputMs == 0 {
		r.Attempts[n-1].FirstOutputMs = max(1, durationMs(attemptStart, at))
	}
}
