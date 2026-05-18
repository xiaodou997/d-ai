package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats"
	"uni-ai-api/backend/internal/formats/canonical"
	"uni-ai-api/backend/internal/formats/claude"
	"uni-ai-api/backend/internal/observability"
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
		clientProto := formats.DetectClientProtocol(r)

		body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024*1024))
		if err != nil {
			writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Failed to read request body.", "body_read_error")
			return
		}
		envelope := &serving.RequestEnvelope{
			W:              w,
			R:              r,
			ClientProtocol: clientProto,
			ClientBody:     body,
		}
		req := &serving.Request{
			Envelope:       envelope,
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
				writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body: "+err.Error(), "invalid_body")
				return
			}
			req.ModelCode = chatReq.Model
			req.IsStream = chatReq.Stream
			req.ChatReq = chatReq
			req.ConversationID = extractConversationID(body, r)
			envelope.IsStream = chatReq.Stream

		case domain.CapabilityEmbedding:
			var embedReq canonical.EmbeddingRequest
			if err := json.Unmarshal(body, &embedReq); err != nil {
				writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body.", "invalid_body")
				return
			}
			req.ModelCode = embedReq.Model
			req.EmbedReq = &embedReq

		case domain.CapabilityImage:
			var imgReq canonical.ImageRequest
			if err := json.Unmarshal(body, &imgReq); err != nil {
				writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body.", "invalid_body")
				return
			}
			req.ModelCode = imgReq.Model
			req.ImageReq = &imgReq

		default:
			writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Unsupported capability.", "unsupported_capability")
			return
		}

		if req.ModelCode == "" {
			writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Missing required parameter: model.", "missing_required_parameter")
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
			writeRuntimeError(w, clientProto, err)
			return
		}
		writeRouteHeaders(w, req)
	}
}

// writeRuntimeError converts a pipeline error to a protocol-appropriate JSON
// response (Anthropic clients get Anthropic-shaped errors, etc.).
func writeRuntimeError(w http.ResponseWriter, clientProto domain.UpstreamProtocol, err error) {
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		writeRuntimeErrorByProtocol(w, clientProto, apiErr.Status, apiErr.Message, apiErr.Code)
		return
	}
	var pipeErr *serving.PipelineError
	if errors.As(err, &pipeErr) {
		var inner *serving.APIError
		if errors.As(pipeErr.Cause, &inner) {
			writeRuntimeErrorByProtocol(w, clientProto, inner.Status, inner.Message, inner.Code)
			return
		}
	}
	writeRuntimeErrorByProtocol(w, clientProto, http.StatusInternalServerError, "Internal server error.", "internal_error")
}

// writeRuntimeErrorByProtocol writes an error response in the client's wire
// protocol. Anthropic clients get the `{"type":"error","error":{...}}` envelope;
// everything else gets the OpenAI `{"error":{...}}` envelope.
func writeRuntimeErrorByProtocol(w http.ResponseWriter, clientProto domain.UpstreamProtocol, status int, message, code string) {
	if clientProto == domain.ProtocolAnthropicMessages {
		body, err := claude.MarshalError(code, message)
		if err != nil {
			// Fall back to OpenAI shape so the client at least gets JSON.
			writeOpenAIError(w, status, message, "api_error", code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}
	writeOpenAIError(w, status, message, "invalid_request_error", code)
}

// extractConversationID picks the conversation/session identifier for sticky
// routing. Priority:
//  1. X-Conversation-Id header (gateway-defined opt-in).
//  2. Top-level "conversation_id" field in the request body.
//
// Returns the empty string when neither is present, which the pipeline treats
// as "new conversation — go through the scorer fresh".
func extractConversationID(body []byte, r *http.Request) string {
	if h := r.Header.Get("X-Conversation-Id"); h != "" {
		return h
	}
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

// writeRouteHeaders appends observability headers to the response. For SSE
// streams the headers have already been sent, so this is a no-op in practice
// (HTTP/1.1 trailers are not supported by most SSE clients). The headers are
// useful for non-streaming JSON responses.
func writeRouteHeaders(w http.ResponseWriter, req *serving.Request) {
	h := w.Header()
	if req.RequestID != "" {
		h.Set("X-Request-Id", req.RequestID)
	}
	if n := len(req.Attempts); n > 0 {
		h.Set("X-Route-Attempts", fmt.Sprintf("%d", n))
	}
	if req.Candidate != nil {
		h.Set("X-Route-Used", req.Candidate.RouteID)
	}
	if observability.TraceHeaderEnabled() {
		trace := observability.BuildTrace(req)
		if encoded, err := observability.EncodeTraceHeader(trace); err == nil {
			h.Set("X-Route-Trace", encoded)
		}
	}
}
