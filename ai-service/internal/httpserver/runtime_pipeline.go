package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/formats"
	"xiaodou/uni-ai-api/internal/formats/claude"
	"xiaodou/uni-ai-api/internal/observability"
	"xiaodou/uni-ai-api/internal/serving"
)

// runtimeOverride carries values resolved from the URL path that must override
// what would otherwise be parsed from the request body. Used by the Gemini
// native endpoint where model and stream-ness live in the URL, not the body.
type runtimeOverride struct {
	model  string // upstream model code (when provided by URL)
	stream bool   // true if this URL is a streaming variant
	apply  bool   // false → ignore overrides and parse from body
}

// handleRuntime is the unified entrypoint for OpenAI / Anthropic native client
// endpoints. The capability is fixed per route; model + stream-ness come from
// the request body.
func (s *Server) handleRuntime(capType domain.CapabilityType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.serveRuntime(w, r, capType, runtimeOverride{})
	}
}

// handleGeminiRuntime handles Google's native paths
// `POST /v1beta/models/{model}:generateContent|streamGenerateContent|embedContent`.
// Chi captures the entire last segment ("gemini-pro:generateContent"); we
// split on the colon to recover model and action.
func (s *Server) handleGeminiRuntime(w http.ResponseWriter, r *http.Request) {
	modelAction := chi.URLParam(r, "modelAction")
	colon := strings.IndexByte(modelAction, ':')
	if colon <= 0 || colon == len(modelAction)-1 {
		writeRuntimeErrorByProtocol(w, domain.ProtocolGeminiGenerate, http.StatusBadRequest,
			"Invalid Gemini path: expected /v1beta/models/MODEL:ACTION.", "invalid_request")
		return
	}
	model, action := modelAction[:colon], modelAction[colon+1:]

	var (
		capType domain.CapabilityType
		stream  bool
	)
	switch action {
	case "generateContent":
		capType = domain.CapabilityChat
	case "streamGenerateContent":
		capType = domain.CapabilityChat
		stream = true
	case "embedContent":
		capType = domain.CapabilityEmbedding
	default:
		writeRuntimeErrorByProtocol(w, domain.ProtocolGeminiGenerate, http.StatusBadRequest,
			fmt.Sprintf("Unsupported Gemini action %q.", action), "invalid_request")
		return
	}

	s.serveRuntime(w, r, capType, runtimeOverride{
		model:  model,
		stream: stream,
		apply:  true,
	})
}

func (s *Server) serveRuntime(w http.ResponseWriter, r *http.Request, capType domain.CapabilityType, override runtimeOverride) {
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

	// Resolve model + stream. URL overrides (Gemini) take precedence over body
	// fields because the body of a Gemini request typically has no `model`.
	var model string
	var stream bool
	if override.apply {
		model = override.model
		stream = override.stream
	} else {
		meta, err := formats.ParseRequestMeta(body)
		if err != nil {
			writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest,
				"Invalid request body: "+err.Error(), "invalid_body")
			return
		}
		model = meta.Model
		stream = meta.Stream
	}

	req.ModelCode = model
	req.IsStream = stream
	envelope.IsStream = stream

	// Per-capability billing-only metadata (does not influence routing or
	// request body — strictly used for usage logging / freeze estimate).
	switch capType {
	case domain.CapabilityChat:
		req.ConversationID = extractConversationID(body, r)
	case domain.CapabilityImage:
		var meta struct {
			N    *int   `json:"n"`
			Size string `json:"size"`
		}
		_ = json.Unmarshal(body, &meta)
		n := 1
		if meta.N != nil && *meta.N > 0 {
			n = *meta.N
		}
		req.TokenUsage.ImageCount = n
		req.TokenUsage.ImageResolution = meta.Size
	case domain.CapabilityVideo:
		var meta struct {
			Resolution string  `json:"resolution"`
			Duration   float64 `json:"duration"`
		}
		_ = json.Unmarshal(body, &meta)
		req.TokenUsage.VideoSeconds = meta.Duration
		req.TokenUsage.VideoResolution = meta.Resolution
	}

	if req.ModelCode == "" {
		writeRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest,
			"Missing required parameter: model.", "missing_required_parameter")
		return
	}

	if err := s.pipeline.Run(r.Context(), req); err != nil {
		if req.HTTPStatus != 0 {
			s.logger.Warn( "pipeline error after response committed",
				zap.Error(err),
				zap.String("request_id", req.RequestID),
				zap.Int("http_status", req.HTTPStatus),
			)
			return
		}
		writeRuntimeError(w, clientProto, err)
		return
	}
	writeRouteHeaders(w, req)
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
