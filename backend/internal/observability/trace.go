package observability

import (
	"encoding/base64"
	"encoding/json"
	"os"

	"uni-ai-api/backend/internal/serving"
)

// TraceAttempt is one upstream call in the X-Route-Trace payload.
type TraceAttempt struct {
	RouteID   string  `json:"route_id"`
	Score     float64 `json:"score,omitempty"`
	Outcome   string  `json:"outcome"`
	HTTP      int     `json:"http,omitempty"`
	LatencyMs int     `json:"latency_ms"`
}

// TracePayload is the full X-Route-Trace JSON body (base64-encoded in the header).
type TracePayload struct {
	Attempts      []TraceAttempt     `json:"attempts"`
	StickyHit     bool               `json:"sticky_hit"`
	ScorerWeights map[string]float64 `json:"scorer_weights,omitempty"`
}

// TraceHeaderEnabled reports whether X-Route-Trace should be included.
// Controlled by environment variable UNI_AI_TRACE_HEADER=1.
func TraceHeaderEnabled() bool {
	return os.Getenv("UNI_AI_TRACE_HEADER") == "1"
}

// BuildTrace constructs a TracePayload from the pipeline request.
func BuildTrace(req *serving.Request) *TracePayload {
	attempts := make([]TraceAttempt, 0, len(req.Attempts))
	for _, a := range req.Attempts {
		attempts = append(attempts, TraceAttempt{
			RouteID:   a.RouteID,
			Score:     a.Score,
			Outcome:   a.Outcome.String(),
			HTTP:      a.HTTPStatus,
			LatencyMs: a.LatencyMs,
		})
	}
	return &TracePayload{
		Attempts:  attempts,
		StickyHit: req.StickyHit,
	}
}

// EncodeTraceHeader serialises p to base64-encoded JSON suitable for the
// X-Route-Trace response header. Returns an error only on JSON marshal failure.
func EncodeTraceHeader(p *TracePayload) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
