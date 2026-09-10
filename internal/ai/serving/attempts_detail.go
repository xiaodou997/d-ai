package serving

import (
	"encoding/json"
	"sort"
)

// attemptsDetailErrorMaxLen caps each attempt's error text. This is a compact
// per-attempt retry trail, not a full error dump — GiveUp's own error already
// lands in InternalErrorDetail (up to internalErrorDetailMaxLen) via
// normalizePipelineError/execute.go; this just needs enough to tell attempts
// apart (timeout vs 429 vs which upstream 5xx'd).
const attemptsDetailErrorMaxLen = 2048

// attemptDetailDTO is the admin-only JSON shape persisted for each retry
// attempt (ai_request_payloads.attempts_detail). Deliberately separate from
// AttemptRecord/TraceAttempt: observability.BuildTrace builds the client-
// visible X-Route-Trace header straight from AttemptRecord and only reads
// route-policy metadata, score, outcome and timings, so extending
// AttemptRecord cannot leak upstream identity or raw error text to callers —
// but this DTO is the one place that intentionally persists it, admin-only.
type attemptDetailDTO struct {
	Sequence        int     `json:"-"`
	RouteID         string  `json:"route_id,omitempty"`
	GroupID         string  `json:"group_id,omitempty"`
	RoutePolicy     string  `json:"route_policy,omitempty"`
	GroupRank       int     `json:"group_rank"`
	Priority        int     `json:"priority"`
	SelectionReason string  `json:"selection_reason,omitempty"`
	ProviderCode    string  `json:"provider_code,omitempty"`
	UpstreamModel   string  `json:"upstream_model,omitempty"`
	EndpointID      string  `json:"endpoint_id,omitempty"`
	PoolID          string  `json:"pool_id,omitempty"`
	CredentialID    string  `json:"credential_id,omitempty"`
	ProfileRevision string  `json:"profile_revision,omitempty"`
	HTTPStatus      int     `json:"http_status,omitempty"`
	Skipped         bool    `json:"skipped"`
	Outcome         string  `json:"outcome"`
	LatencyMs       int     `json:"latency_ms,omitempty"`
	FirstByteMs     int     `json:"first_byte_ms,omitempty"`
	TotalMs         int     `json:"total_ms,omitempty"`
	Error           string  `json:"error,omitempty"`
	Score           float64 `json:"score,omitempty"`
}

// BuildAttemptsDetail renders req.Attempts (every upstream candidate tried
// during Execute, in order) into the admin-only JSON persisted alongside
// InternalErrorDetail. Returns nil when there were no attempts (request
// failed before reaching Execute, e.g. auth/quota/routing).
func BuildAttemptsDetail(attempts []AttemptRecord) json.RawMessage {
	if len(attempts) == 0 {
		return nil
	}
	out := make([]attemptDetailDTO, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, attemptDetailDTO{
			Sequence:        a.Sequence,
			RouteID:         a.RouteID,
			GroupID:         a.GroupID,
			RoutePolicy:     a.RoutePolicy,
			GroupRank:       a.GroupRank,
			Priority:        a.TargetPriority,
			SelectionReason: a.SelectionReason,
			ProviderCode:    a.ProviderCode,
			UpstreamModel:   a.UpstreamModel,
			EndpointID:      a.EndpointID,
			PoolID:          a.PoolID,
			CredentialID:    a.CredentialID,
			ProfileRevision: a.ProfileRevision,
			HTTPStatus:      a.HTTPStatus,
			Outcome:         a.Outcome.String(),
			LatencyMs:       a.LatencyMs,
			FirstByteMs:     a.FirstByteMs,
			TotalMs:         a.TotalMs,
			Error:           RedactInternalErrorDetail(truncateValidUTF8(a.ErrorMsg, attemptsDetailErrorMaxLen)),
			Score:           a.Score,
		})
	}
	return marshalAttemptsDetail(out)
}

// BuildAttemptsDetailWithSkipped renders both real transport attempts and
// candidates that were filtered out before any upstream transport call (e.g.
// circuit breaker open). The admin-only trail distinguishes them with
// Skipped=true so operators can tell "never reached upstream" apart from
// "reached upstream and failed with outcome X". Skipped entries carry
// no HTTP status or latency. Entries retain their request-local event order.
// Returns nil when both slices are empty.
func BuildAttemptsDetailWithSkipped(attempts []AttemptRecord, skipped []AttemptRecord) json.RawMessage {
	if len(attempts) == 0 && len(skipped) == 0 {
		return nil
	}
	out := make([]attemptDetailDTO, 0, len(attempts)+len(skipped))
	for _, a := range skipped {
		out = append(out, attemptDetailDTO{
			Sequence:        a.Sequence,
			RouteID:         a.RouteID,
			GroupID:         a.GroupID,
			RoutePolicy:     a.RoutePolicy,
			GroupRank:       a.GroupRank,
			Priority:        a.TargetPriority,
			SelectionReason: a.SelectionReason,
			ProviderCode:    a.ProviderCode,
			UpstreamModel:   a.UpstreamModel,
			EndpointID:      a.EndpointID,
			PoolID:          a.PoolID,
			HTTPStatus:      0,
			Skipped:         true,
			Outcome:         a.Outcome.String(),
			Error:           RedactInternalErrorDetail(truncateValidUTF8(a.ErrorMsg, attemptsDetailErrorMaxLen)),
			Score:           0,
		})
	}
	for _, a := range attempts {
		out = append(out, attemptDetailDTO{
			Sequence:        a.Sequence,
			RouteID:         a.RouteID,
			GroupID:         a.GroupID,
			RoutePolicy:     a.RoutePolicy,
			GroupRank:       a.GroupRank,
			Priority:        a.TargetPriority,
			SelectionReason: a.SelectionReason,
			ProviderCode:    a.ProviderCode,
			UpstreamModel:   a.UpstreamModel,
			EndpointID:      a.EndpointID,
			PoolID:          a.PoolID,
			CredentialID:    a.CredentialID,
			ProfileRevision: a.ProfileRevision,
			HTTPStatus:      a.HTTPStatus,
			Outcome:         a.Outcome.String(),
			LatencyMs:       a.LatencyMs,
			FirstByteMs:     a.FirstByteMs,
			TotalMs:         a.TotalMs,
			Error:           RedactInternalErrorDetail(truncateValidUTF8(a.ErrorMsg, attemptsDetailErrorMaxLen)),
			Score:           a.Score,
		})
	}
	return marshalAttemptsDetail(out)
}

func marshalAttemptsDetail(out []attemptDetailDTO) json.RawMessage {
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

// BuildAttemptsDetailFull is the single entry point for the admin-only
// attempts_detail trail. It merges three sources in chronological-causal order:
//
//   - req.PlanningSkipped: planner verdicts (resolver rejected a target BEFORE
//     it became a candidate). Rendered as outcome=rejected + Skipped=true.
//   - req.SkippedAttempts: candidates whose breaker was OPEN when Execute tried
//     to use them. Rendered as outcome=circuit_open + Skipped=true.
//   - req.Attempts: real transport calls with their true outcome.
//
// PlanningSkipped has sequence zero and precedes execution. Calls and breaker
// skips share increasing sequence numbers, so interleaved events stay in order.
// Both skipped slices stay out of the retry budget and X-Route-Trace.
func BuildAttemptsDetailFull(req *Request) json.RawMessage {
	if req == nil {
		return nil
	}
	if len(req.Attempts) == 0 && len(req.PlanningSkipped) == 0 && len(req.SkippedAttempts) == 0 {
		return nil
	}
	skipped := make([]AttemptRecord, 0, len(req.PlanningSkipped)+len(req.SkippedAttempts))
	skipped = append(skipped, req.PlanningSkipped...)
	skipped = append(skipped, req.SkippedAttempts...)
	return BuildAttemptsDetailWithSkipped(req.Attempts, skipped)
}
