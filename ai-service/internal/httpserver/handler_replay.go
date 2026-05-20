package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handleReplay handles POST /api/v1/usage-logs/{id}/replay.
// It fetches the stored request payload for the given usage log, decrypts the
// original client body, and returns it together with the recorded route
// attempts so an admin can inspect the failure and manually re-submit.
//
// The handler intentionally does not re-execute the request server-side: the
// re-submit must be triggered by the caller with the appropriate API key and
// auth context (the original key may have expired or been revoked).
func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if s.payloadStore == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrBadRequest, "payload store not configured")
		return
	}

	usageLogID := chi.URLParam(r, "id")
	if usageLogID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "usage_log_id is required")
		return
	}

	payload, err := s.payloadStore.GetByUsageLogID(r.Context(), usageLogID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrDatabase, "failed to fetch payload")
		return
	}
	if payload == nil {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "no payload found for this usage log")
		return
	}

	// Return the decrypted client body as a JSON-encodable value for inspection.
	var clientBodyJSON json.RawMessage
	if len(payload.RawClientBody) > 0 {
		clientBodyJSON = json.RawMessage(payload.RawClientBody)
	}

	type attempt struct {
		RouteID   string  `json:"route_id"`
		Score     float64 `json:"score,omitempty"`
		Outcome   string  `json:"outcome"`
		HTTP      int     `json:"http,omitempty"`
		LatencyMs int     `json:"latency_ms"`
	}
	attempts := make([]attempt, 0, len(payload.RouteAttempts))
	for _, a := range payload.RouteAttempts {
		attempts = append(attempts, attempt{
			RouteID:   a.RouteID,
			Score:     a.Score,
			Outcome:   a.Outcome,
			HTTP:      a.HTTP,
			LatencyMs: a.LatencyMs,
		})
	}

	writeOK(w, map[string]any{
		"payload_id":      payload.ID,
		"usage_log_id":    payload.UsageLogID,
		"client_protocol": payload.ClientProtocol,
		"sampled":         payload.Sampled,
		"created_at":      payload.CreatedAt,
		"expires_at":      payload.ExpiresAt,
		"route_attempts":  attempts,
		"client_body":     clientBodyJSON,
	})
}
