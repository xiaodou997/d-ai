package gateway

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/core/catalog"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/formats/claude"
	"xiaodou/dai/internal/ai/observability"
	"xiaodou/dai/internal/ai/runtimecompat"
	"xiaodou/dai/internal/ai/serving"
)

// RuntimeOverride carries request metadata that must override what would
// otherwise be detected from the HTTP path/body. Gemini native routes use it
// for URL model/action metadata; the web runtime chat layer uses it after
// mapping the internal chat payload to a concrete upstream protocol.
type RuntimeOverride struct {
	Model          string // upstream model code (when provided by URL)
	Stream         bool   // true if this URL is a streaming variant
	Apply          bool   // false → ignore model/stream overrides and parse from body
	ClientProtocol domain.UpstreamProtocol
	ClientPath     string
	ExecutionMode  coreruntime.ExecutionMode
}

// handleRuntime is the unified entrypoint for OpenAI / Anthropic native client
// endpoints. The capability is fixed per route; model + stream-ness come from
// the request body.
func (s *Gateway) handleRuntime(capType domain.CapabilityType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.ExecuteRuntime(w, r, capType, RuntimeOverride{}, nil, false)
	}
}

// handleGeminiRuntime handles Google's native paths
// `POST /v1beta/models/{model}:generateContent|streamGenerateContent|embedContent`.
// Chi captures the entire last segment ("gemini-pro:generateContent"); we
// split on the colon to recover model and action.
func (s *Gateway) handleGeminiRuntime(w http.ResponseWriter, r *http.Request) {
	modelAction := chi.URLParam(r, "modelAction")
	colon := strings.IndexByte(modelAction, ':')
	if colon <= 0 || colon == len(modelAction)-1 {
		WriteRuntimeErrorByProtocol(w, domain.ProtocolGeminiGenerate, http.StatusBadRequest,
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
		WriteRuntimeErrorByProtocol(w, domain.ProtocolGeminiGenerate, http.StatusBadRequest,
			fmt.Sprintf("Unsupported Gemini action %q.", action), "invalid_request")
		return
	}

	s.ExecuteRuntime(w, r, capType, RuntimeOverride{
		Model:  model,
		Stream: stream,
		Apply:  true,
	}, nil, false)
}

func (s *Gateway) ExecuteRuntime(
	w http.ResponseWriter,
	r *http.Request,
	capType domain.CapabilityType,
	override RuntimeOverride,
	subjectOverride *coreidentity.Subject,
	forceStream bool,
) coreruntime.Result {
	var subject coreidentity.Subject
	if subjectOverride != nil {
		subject = *subjectOverride
	} else if auth, ok := runtimeAuthFromContext(r.Context()); ok {
		subject = auth.Subject
	}
	if subject.TenantID == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
		return coreruntime.Result{}
	}

	clientProto := formats.DetectClientProtocol(r)
	if override.ClientProtocol != "" {
		clientProto = override.ClientProtocol
	}
	contentType := r.Header.Get("Content-Type")

	bodyLimit := int64(32 * 1024 * 1024)
	if capType == domain.CapabilityImage {
		bodyLimit = maxImageRequestBodyBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Failed to read request body.", "body_read_error")
		return coreruntime.Result{}
	}
	if forceStream {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body: expected JSON object.", "invalid_body")
			return coreruntime.Result{}
		}
		payload["stream"] = true
		body, err = json.Marshal(payload)
		if err != nil {
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body.", "invalid_body")
			return coreruntime.Result{}
		}
	}
	if capType == domain.CapabilityImage && clientProto == domain.ProtocolOpenAIImages {
		if err := validateOpenAIImageInputLimits(body, contentType); err != nil {
			var apiErr *serving.APIError
			if errors.As(err, &apiErr) {
				WriteRuntimeErrorByProtocol(w, clientProto, apiErr.Status, apiErr.Message, apiErr.Code)
				return coreruntime.Result{}
			}
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid image input.", "invalid_request_error")
			return coreruntime.Result{}
		}
		var imageValidationErr error
		body, contentType, imageValidationErr = normalizeOpenAIImageRuntimeRequest(body, contentType, r.URL.Path)
		if imageValidationErr != nil {
			var apiErr *serving.APIError
			if errors.As(imageValidationErr, &apiErr) {
				WriteRuntimeErrorByProtocol(w, clientProto, apiErr.Status, apiErr.Message, apiErr.Code)
				return coreruntime.Result{}
			}
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, imageValidationErr.Error(), "invalid_request_error")
			return coreruntime.Result{}
		}
		r.Header.Set("Content-Type", contentType)
	} else if clientProto == domain.ProtocolGeminiGenerate {
		body, err = sanitizeGeminiImageCount(body)
		if err != nil {
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body.", "invalid_request_error")
			return coreruntime.Result{}
		}
	}
	var model string
	var stream bool
	if override.Apply {
		if err := validateServiceTierParseableBody(body, contentType); err != nil {
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body: "+err.Error(), "invalid_body")
			return coreruntime.Result{}
		}
		model = override.Model
		stream = override.Stream
	} else {
		meta, err := formats.ParseRequestMeta(body, contentType)
		if err != nil {
			WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Invalid request body: "+err.Error(), "invalid_body")
			return coreruntime.Result{}
		}
		model = meta.Model
		stream = meta.Stream
	}
	serviceTier, err := parseServiceTier(body, contentType)
	if err != nil {
		WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, err.Error(), "invalid_service_tier")
		return coreruntime.Result{}
	}
	if model == "" {
		WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Missing required parameter: model.", "missing_required_parameter")
		return coreruntime.Result{}
	}

	runtimeCapability := runtimeCapabilityForRequest(capType, clientProto, r.URL.Path, body)
	clientSurface, err := runtimecompat.ProtocolToSurfaceForCapability(clientProto, runtimeCapability)
	if err != nil {
		WriteRuntimeErrorByProtocol(w, clientProto, http.StatusBadRequest, "Unsupported client protocol.", "invalid_request")
		return coreruntime.Result{}
	}

	runtimeReq := coreruntime.Request{
		ExecutionMode:  normalizedExecutionMode(override.ExecutionMode),
		RequestID:      newRequestID(r),
		TraceID:        r.Header.Get("X-Trace-Id"),
		Capability:     runtimeCapability,
		ClientSurface:  clientSurface,
		RequestedModel: model,
		Body:           body,
		Stream:         stream,
		ServiceTier:    string(serviceTier),
		ReceivedAt:     time.Now(),
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	if override.ClientPath != "" && r.URL != nil {
		r.URL.Path = override.ClientPath
	}
	if s.runtimeEngine == nil {
		WriteRuntimeErrorByProtocol(w, clientProto, http.StatusServiceUnavailable, "Runtime engine is unavailable.", "runtime_unavailable")
		return coreruntime.Result{}
	}
	result, err := s.runtimeEngine.Execute(r.Context(), coreruntime.ExecutionInput{
		Subject: subject,
		Request: runtimeReq,
		Envelope: coreruntime.ExecutionEnvelope{
			ResponseWriter: w,
			HTTPRequest:    r,
			ClientBody:     body,
		},
	})
	if err != nil {
		if result.ResponseCommitted {
			s.logger.Warn("runtime pipeline error after response committed",
				gatewayLogFields(r.Context(), subject.TenantID, subject.UserID,
					zap.Error(err),
					zap.String("request_id", runtimeReq.RequestID),
					zap.Int("http_status", result.StatusCode),
				)...,
			)
			return result
		}
		writePipelineErrorByProtocol(w, clientProto, err)
		return result
	}
	return result
}

func runtimeCapabilityForRequest(capType domain.CapabilityType, clientProto domain.UpstreamProtocol, path string, body []byte) catalog.Capability {
	if capType != domain.CapabilityImage && clientProto != domain.ProtocolGeminiGenerate {
		return capType.ToCore()
	}
	if clientProto == domain.ProtocolGeminiGenerate {
		if isImage, isEdit := formats.GeminiRequestImageIntent(body); isImage {
			if isEdit {
				return catalog.CapabilityImageEdit
			}
			return catalog.CapabilityImageGeneration
		}
		if capType != domain.CapabilityImage {
			return capType.ToCore()
		}
	}
	if strings.Contains(path, "/images/edits") {
		return catalog.CapabilityImageEdit
	}
	return catalog.CapabilityImageGeneration
}

func parseImageBillingMeta(body []byte, contentType string) (count int, size string) {
	count = domain.DefaultImageOutputCount
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType == "multipart/form-data" {
		fields, err := formats.MultipartScalarFields(body, contentType, 1<<20)
		if err != nil {
			return count, ""
		}
		if parsed, err := strconv.Atoi(strings.TrimSpace(fields["n"])); err == nil && validOpenAIImageCount(parsed) {
			count = parsed
		}
		return count, fields["size"]
	}
	var meta struct {
		N                int    `json:"n"`
		Size             string `json:"size"`
		GenerationConfig struct {
			CandidateCount int    `json:"candidateCount"`
			ImageSize      string `json:"imageSize"`
		} `json:"generationConfig"`
	}
	_ = json.Unmarshal(body, &meta)
	if validOpenAIImageCount(meta.N) {
		count = meta.N
	} else if validOpenAIImageCount(meta.GenerationConfig.CandidateCount) {
		count = meta.GenerationConfig.CandidateCount
	}
	if meta.Size == "" {
		meta.Size = meta.GenerationConfig.ImageSize
	}
	return count, meta.Size
}

func writePipelineErrorByProtocol(w http.ResponseWriter, clientProto domain.UpstreamProtocol, err error) {
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		WriteRuntimeErrorByProtocol(w, clientProto, apiErr.Status, apiErr.Message, apiErr.Code)
		return
	}
	var pipeErr *serving.PipelineError
	if errors.As(err, &pipeErr) {
		var inner *serving.APIError
		if errors.As(pipeErr.Cause, &inner) {
			WriteRuntimeErrorByProtocol(w, clientProto, inner.Status, inner.Message, inner.Code)
			return
		}
	}
	WriteRuntimeErrorByProtocol(w, clientProto, http.StatusInternalServerError, "Internal server error.", "internal_error")
}

// WriteRuntimeErrorByProtocol writes an error response in the client's wire
// protocol. Anthropic clients get the `{"type":"error","error":{...}}` envelope;
// everything else gets the OpenAI `{"error":{...}}` envelope.
func WriteRuntimeErrorByProtocol(w http.ResponseWriter, clientProto domain.UpstreamProtocol, status int, message, code string) {
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
// conversationIDHeaders are the request headers that carry a client-side
// conversation identity, in priority order. Codex CLI sends session_id (and
// conversation_id); other clients may set X-Conversation-Id explicitly.
var conversationIDHeaders = []string{
	"X-Conversation-Id",
	"X-Session-Id",
	"session_id",
	"conversation_id",
}

// extractConversationID resolves the sticky-routing conversation key for a
// request. Real coding clients never send a dedicated conversation header, so
// the lookup walks every identity a client may carry — headers, then the
// protocol-specific body fields — and finally falls back to a fingerprint of
// the first message, which stays stable for the whole life of a multi-turn
// conversation. Without this fallback a Codex/Claude Code session would be
// re-scored (and re-scattered across upstream accounts) on every turn.
func extractConversationID(body []byte, r *http.Request) string {
	if r != nil {
		for _, name := range conversationIDHeaders {
			if v := strings.TrimSpace(r.Header.Get(name)); v != "" {
				return v
			}
		}
	}
	var envelope struct {
		ConversationID string          `json:"conversation_id"`
		Conversation   json.RawMessage `json:"conversation"`
		PromptCacheKey string          `json:"prompt_cache_key"`
		SessionID      string          `json:"session_id"`
		PreviousRespID string          `json:"previous_response_id"`
		User           string          `json:"user"`
		Metadata       struct {
			ConversationID string `json:"conversation_id"`
			SessionID      string `json:"session_id"`
			UserID         string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	candidates := []string{
		envelope.ConversationID,
		conversationRefID(envelope.Conversation),
		// Codex CLI reuses one prompt_cache_key for a whole session.
		envelope.PromptCacheKey,
		envelope.SessionID,
		envelope.Metadata.ConversationID,
		envelope.Metadata.SessionID,
		// Claude Code encodes its session UUID into metadata.user_id.
		envelope.Metadata.UserID,
		envelope.User,
	}
	for _, c := range candidates {
		if c = strings.TrimSpace(c); c != "" {
			return c
		}
	}
	// previous_response_id changes every turn, so it cannot key a conversation
	// on its own — but its presence proves this is a follow-up turn, and the
	// fingerprint below still anchors on the unchanged first message.
	return conversationFingerprint(body)
}

// conversationRefID reads the Responses API "conversation" field, which is
// either a bare id string or an object carrying one.
func conversationRefID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.ID
	}
	return ""
}

// conversationAnchorFields are the body fields holding the first turn of a
// conversation, across every client protocol we accept. The first message of a
// thread does not change as the thread grows, which makes it a usable identity
// for clients that send no conversation id at all.
var conversationAnchorFields = []string{"messages", "input", "contents"}

// conversationFingerprint derives a stable key from the first message in the
// body. Returns "" when no anchor can be found (single-shot payloads then keep
// the previous behaviour of no sticky binding).
func conversationFingerprint(body []byte) string {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	for _, field := range conversationAnchorFields {
		raw, ok := envelope[field]
		if !ok {
			continue
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
			continue
		}
		anchor := firstUserTurn(items)
		sum := sha256.Sum256(append([]byte(field+":"), anchor...))
		return "fp_" + hex.EncodeToString(sum[:12])
	}
	return ""
}

// firstUserTurn returns the first non-system item, falling back to items[0].
// System/developer preambles are skipped because they often embed volatile
// context (current date, working directory) that would break the fingerprint
// between turns of the same conversation.
func firstUserTurn(items []json.RawMessage) json.RawMessage {
	for _, item := range items {
		var probe struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(item, &probe); err != nil {
			continue
		}
		if probe.Role != "system" && probe.Role != "developer" {
			return item
		}
	}
	return items[0]
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
