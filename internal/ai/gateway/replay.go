package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"strings"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

// ReplayInput is a synthesized runtime call: what an API client would have sent,
// minus the socket.
//
// This is how work runs when nobody is holding a connection — an async worker,
// a console task. The async surface is a first-class client of the runtime
// surface, not a second implementation of it, so a replayed call goes through
// the exact same pipeline, billing and logging as a real one.
//
// The console already worked this way (it built an in-process http.Request
// against /v1/images/generations); this type just makes it shared instead of
// private, so every async capability inherits it rather than reinventing it.
type ReplayInput struct {
	// Subject is the authenticated caller. Replay does not modify it — in
	// particular RequestSource is the caller's to set, because it is what
	// distinguishes a console task from an API-key task in the usage log.
	Subject coreidentity.Subject
	// ExecutionMode must be async for queued work. Zero defaults to sync so
	// live console previews that also use Replay retain interactive semantics.
	ExecutionMode coreruntime.ExecutionMode

	Capability domain.CapabilityType
	Protocol   domain.UpstreamProtocol

	// ClientPath is semantic, not cosmetic: image request normalization keys off
	// it to tell generation from edit (see normalizeOpenAIImageRuntimeRequest).
	ClientPath string

	Body        []byte
	ContentType string

	// Header carries extra headers the synthesized request should have.
	// ContentType and RequestID are applied after these and win.
	Header http.Header

	// RequestID, when set, is injected as X-Request-Id, which newRequestID
	// prefers over generating one. That makes the ai_usage_logs row this call
	// produces join back to whatever queued it.
	RequestID string

	// StreamExpected tells the aggregator to fold an SSE response back into a
	// single JSON body. A replayed call has no client to stream to.
	StreamExpected bool
}

// ReplayResult is the buffered outcome of a replayed call.
type ReplayResult struct {
	// Request is nil when the pipeline never started, which means the failure is
	// in Body rather than in the request's error fields.
	Request    *coreruntime.Result
	Body       []byte
	StatusCode int
	Header     http.Header
}

// buildReplayRequest synthesizes the http.Request for a replay.
//
// Precedence: in.Header first, then ContentType and RequestID overwrite it. A
// forwarding caller must not be able to leak the inbound client's Content-Type
// or X-Request-Id into a call whose body and identity we chose.
func buildReplayRequest(ctx context.Context, in ReplayInput) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, in.ClientPath, bytes.NewReader(in.Body))
	if err != nil {
		return nil, err
	}
	for key, values := range in.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	if in.RequestID != "" {
		req.Header.Set("X-Request-Id", in.RequestID)
	}
	req.ContentLength = int64(len(in.Body))
	return req, nil
}

// Replay runs one synthesized request through the runtime pipeline and buffers
// the whole response.
func (s *Gateway) Replay(ctx context.Context, in ReplayInput) ReplayResult {
	req, err := buildReplayRequest(ctx, in)
	if err != nil {
		return ReplayResult{
			Body:       ReplayErrorBody(err.Error()),
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{},
		}
	}

	buffered := newBufferedResponseWriter()
	subject := in.Subject
	runtimeResult := s.ExecuteRuntime(buffered, req, in.Capability, RuntimeOverride{
		ClientProtocol: in.Protocol,
		ClientPath:     in.ClientPath,
		ExecutionMode:  in.ExecutionMode,
	}, &subject, false)
	var resultPtr *coreruntime.Result
	if runtimeResult.RequestID != "" {
		resultPtr = &runtimeResult
	}

	statusCode := buffered.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	body := buffered.buf.Bytes()
	// Both of these read the images response shape, so they are gated on the
	// capability rather than applied to every replay — folding an SSE chat
	// stream with the image aggregator would corrupt it.
	if in.Capability == domain.CapabilityImage {
		if aggregated, ok, err := AggregateImageStreamResponse(body, buffered.Header().Get("Content-Type"), in.StreamExpected); err != nil {
			body = ReplayErrorBody(err.Error())
			statusCode = http.StatusBadGateway
		} else if ok {
			body = aggregated
			buffered.Header().Set("Content-Type", "application/json")
		}
	}
	return ReplayResult{
		Request:    resultPtr,
		Body:       body,
		StatusCode: statusCode,
		Header:     buffered.Header().Clone(),
	}
}

// bufferedResponseWriter captures a runtime response in memory. It belongs to
// the gateway because the thing being captured is a gateway response.
type bufferedResponseWriter struct {
	header     http.Header
	buf        bytes.Buffer
	statusCode int
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header)}
}

func (w *bufferedResponseWriter) Header() http.Header { return w.header }

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.buf.Write(p)
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) { w.statusCode = statusCode }

// Flush is a no-op: there is no client to flush to, which is the whole point.
func (w *bufferedResponseWriter) Flush() {}

// AggregateImageStreamResponse folds an SSE image response into a single JSON
// body. Whether the upstream streams is a routing decision independent of
// whether the caller wants a stream, so a replayed call may get SSE even though
// it never asked for one.
//
// Reports ok=false when the body is not a stream and should be used as-is.
func AggregateImageStreamResponse(body []byte, contentType string, streamExpected bool) ([]byte, bool, error) {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return body, false, nil
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if !streamExpected && !strings.EqualFold(mediaType, "text/event-stream") {
		return body, false, nil
	}
	if !strings.Contains(text, "data:") && !strings.Contains(text, "event:") {
		return body, false, nil
	}
	aggregated, err := formats.AggregateOpenAIImageSSE(body)
	if err != nil {
		return nil, true, err
	}
	return aggregated, true, nil
}

// ReplayErrorBody renders an error in the client-facing envelope, so a replay
// failure looks like any other error body to whatever stores it.
func ReplayErrorBody(message string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
		},
	})
	return raw
}

// ExtractResponseErrorMessage pulls a human-readable message out of an error
// body, tolerating both the OpenAI `{"error":{"message"}}` shape and a bare
// `{"message"}`.
func ExtractResponseErrorMessage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.Error.Message) != "" {
		return strings.TrimSpace(payload.Error.Message)
	}
	return strings.TrimSpace(payload.Message)
}

// StripImageResponseRevisedPrompt removes the upstream's rewritten prompt from
// an images response. Apps are sealed products and must not leak how the
// platform rewrote the prompt.
func StripImageResponseRevisedPrompt(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return raw
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		delete(obj, "revised_prompt")
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return out
}

// ReplayCallerChargeMicro returns the settled charge for the task owner. The
// runtime already chooses user debit for user owners and tenant charge for
// tenant owners; API key quota is a separate meter and never substitutes here.
func ReplayCallerChargeMicro(result *coreruntime.Result) int64 {
	if result == nil {
		return 0
	}
	return result.CallerChargeMicro
}
