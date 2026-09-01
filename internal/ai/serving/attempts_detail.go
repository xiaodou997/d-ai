package serving

import "encoding/json"

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
	RouteID         string  `json:"route_id,omitempty"`
	GroupID         string  `json:"group_id,omitempty"`
	RouteStrategy   string  `json:"route_strategy,omitempty"`
	RouteObjective  string  `json:"route_objective,omitempty"`
	GroupRank       int     `json:"group_rank"`
	Priority        int     `json:"priority"`
	RoutingWeight   float64 `json:"routing_weight"`
	SelectionReason string  `json:"selection_reason,omitempty"`
	ProviderCode    string  `json:"provider_code,omitempty"`
	UpstreamModel   string  `json:"upstream_model,omitempty"`
	EndpointID      string  `json:"endpoint_id,omitempty"`
	PoolID          string  `json:"pool_id,omitempty"`
	CredentialID    string  `json:"credential_id,omitempty"`
	ProfileRevision string  `json:"profile_revision,omitempty"`
	HTTPStatus      int     `json:"http_status,omitempty"`
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
			RouteID:         a.RouteID,
			GroupID:         a.GroupID,
			RouteStrategy:   a.RouteStrategy,
			RouteObjective:  a.RouteObjective,
			GroupRank:       a.GroupRank,
			Priority:        a.Priority,
			RoutingWeight:   a.RoutingWeight,
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
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}
