package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats"
	"uni-ai-api/backend/internal/formats/canonical"
	"uni-ai-api/backend/internal/serving"
)

// handleRuntime is the unified entrypoint for all AI inference endpoints:
//
//	POST /v1/chat/completions
//	POST /v1/responses
//	POST /v1/embeddings
//	POST /v1/images/generations
//	POST /v1/messages  (native Anthropic)
//
// It builds a serving.Request, runs the pipeline, and maps errors to OpenAI-style
// JSON error responses.
func (s *Server) handleRuntime(capType domain.CapabilityType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024*1024))
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "Failed to read request body.", "invalid_request_error", "body_read_error")
			return
		}

		clientProto := formats.DetectClientProtocol(r)
		req := &serving.Request{
			W:              w,
			R:              r,
			CapabilityType: capType,
			ClientProtocol: clientProto,
			StartedAt:      time.Now(),
			RequestID:      newRequestID(r),
			TraceID:        r.Header.Get("X-Trace-Id"),
		}

		switch capType {
		case domain.CapabilityChat:
			chatReq, err := formats.ParseClientChatRequest(body, clientProto)
			if err != nil {
				writeOpenAIError(w, http.StatusBadRequest, "Invalid request body: "+err.Error(), "invalid_request_error", "invalid_body")
				return
			}
			req.ModelCode = chatReq.Model
			req.IsStream = chatReq.Stream
			req.ChatReq = chatReq
			req.ConversationID = extractConversationID(body)

		case domain.CapabilityEmbedding:
			var embedReq canonical.EmbeddingRequest
			if err := json.Unmarshal(body, &embedReq); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_body")
				return
			}
			req.ModelCode = embedReq.Model
			req.EmbedReq = &embedReq

		case domain.CapabilityImage:
			var imgReq canonical.ImageRequest
			if err := json.Unmarshal(body, &imgReq); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_body")
				return
			}
			req.ModelCode = imgReq.Model
			req.ImageReq = &imgReq

		default:
			writeOpenAIError(w, http.StatusBadRequest, "Unsupported capability.", "invalid_request_error", "unsupported_capability")
			return
		}

		if req.ModelCode == "" {
			writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: model.", "invalid_request_error", "missing_required_parameter")
			return
		}

		if err := s.pipeline.Run(r.Context(), req); err != nil {
			// If Execute step already wrote headers/body (HTTPStatus != 0), the
			// response is committed (especially for SSE streams). Writing another
			// JSON error here would produce a "superfluous WriteHeader" warning
			// and corrupt the stream. Just log and return.
			if req.HTTPStatus != 0 {
				s.logger.WarnContext(r.Context(), "pipeline error after response committed",
					"error", err,
					"request_id", req.RequestID,
					"http_status", req.HTTPStatus,
				)
				return
			}
			writeRuntimeError(w, err)
		}
	}
}

// writeRuntimeError converts a pipeline error to an OpenAI-style JSON response.
func writeRuntimeError(w http.ResponseWriter, err error) {
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		writeOpenAIError(w, apiErr.Status, apiErr.Message, "invalid_request_error", apiErr.Code)
		return
	}
	var pipeErr *serving.PipelineError
	if errors.As(err, &pipeErr) {
		var inner *serving.APIError
		if errors.As(pipeErr.Cause, &inner) {
			writeOpenAIError(w, inner.Status, inner.Message, "invalid_request_error", inner.Code)
			return
		}
	}
	writeOpenAIError(w, http.StatusInternalServerError, "Internal server error.", "server_error", "internal_error")
}

// extractConversationID reads the top-level "conversation_id" string field
// from a raw JSON body without fully unmarshalling the payload.
func extractConversationID(body []byte) string {
	var envelope struct {
		ConversationID string `json:"conversation_id"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.ConversationID
}

// newRequestID returns the X-Request-Id header value, or generates a random hex ID.
func newRequestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
