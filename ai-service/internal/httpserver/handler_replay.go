package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	pgadapter "xiaodou/uni-ai-api/internal/adapters/postgres"
)

// handleReplay handles POST /api/v1/usage-logs/{id}/replay.
// {id} is treated as a request_id (same value stored in ai_usage_logs.request_id).
// Returns the stored request messages, params, and response message for
// inspection — the caller must re-submit manually with valid credentials.
func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "request_id is required")
		return
	}

	store := pgadapter.NewAuditStore(s.postgres)
	record, err := store.GetByRequestID(r.Context(), requestID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrDatabase, "failed to fetch audit record")
		return
	}
	if record == nil {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "no audit record found for this request_id")
		return
	}

	writeOK(w, map[string]any{
		"request_id":       record.RequestID,
		"client_protocol":  record.ClientProtocol,
		"request_model":    record.RequestModel,
		"request_messages": json.RawMessage(record.RequestMessages),
		"request_params":   json.RawMessage(record.RequestParams),
		"response_message": json.RawMessage(record.ResponseMessage),
		"request_status":   record.RequestStatus,
		"http_status":      record.HTTPStatus,
		"error_code":       record.ErrorCode,
		"latency_ms":       record.LatencyMs,
	})
}
