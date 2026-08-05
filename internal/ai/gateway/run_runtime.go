package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/appkey"
	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/serving"
)

type runRequest struct {
	Input       string            `json:"input"`
	Variables   map[string]string `json:"variables"`
	Attachments []runAttachment   `json:"attachments,omitempty"`
	Stream      bool              `json:"stream"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
}

type runAttachment struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Name     string `json:"name,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type runImageGenerationRequest struct {
	Input          string            `json:"input"`
	Variables      map[string]string `json:"variables"`
	N              int               `json:"n,omitempty"`
	Stream         bool              `json:"stream"`
	Size           string            `json:"size,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	Background     string            `json:"background,omitempty"`
	OutputFormat   string            `json:"output_format,omitempty"`
}

var (
	errRunAppTypeUnsupported    = errors.New("app key target has no supported app type")
	errRunChatTargetUnsupported = errors.New("app does not support chat")
	errRunAppDisabled           = errors.New("app is disabled")
	errRunImageGenUnsupported   = errors.New("app does not support image generation")
	errRunImageEditUnsupported  = errors.New("app does not support image edit")
	errRunInputRequired         = errors.New("input is required")
	errRunKeyDisabled           = errors.New("app key disabled")
	errRunKeyExpired            = errors.New("app key expired")
	errRunAttachmentsNotAllowed = errors.New("attachments are not allowed for this app")
	errRunAttachmentInvalid     = errors.New("attachments must use http(s) direct URLs")
	errRunOptionOverrideDenied  = errors.New("runtime options cannot be overridden for this app")
)

// handleRunRuntime is the single entrypoint for app keys: POST /v1/run. The
// app bound to the key (chat / image_generation / image_edit) determines how
// the request body is parsed and dispatched — callers never choose a path.
func (s *Gateway) handleRunRuntime(w http.ResponseWriter, r *http.Request) {
	if !s.runRuntimeResolvedReady() {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Run runtime is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	expansion, err := s.expandRunRuntimeInvocation(r)
	if err != nil {
		writeRunRuntimeResolvedError(w, err)
		return
	}
	if s.rejectIfBanned(w, r.Context(), expansion.Subject.TenantID, expansion.Subject.UserID) {
		return
	}
	if expansion.App == nil {
		writeRunRuntimeResolvedError(w, errRunAppTypeUnsupported)
		return
	}
	switch expansion.App.App.AppType {
	case application.AppTypeChatAgent:
		s.handleRunChat(w, r, expansion)
	case application.AppTypeImageGenerationAgent:
		s.handleRunImageGeneration(w, r, expansion)
	case application.AppTypeImageEditAgent:
		s.handleRunImageEdit(w, r, expansion)
	default:
		writeRunRuntimeResolvedError(w, errRunAppTypeUnsupported)
	}
}

func (s *Gateway) runRuntimeResolvedReady() bool {
	return s.runtimeInvokeExpander != nil && s.runtimeEngine != nil
}

func (s *Gateway) handleRunChat(w http.ResponseWriter, r *http.Request, expansion coreruntime.InvokeExpansion) {
	req, err := decodeRunChatRequestBody(r.Body)
	if err != nil {
		var apiErr *serving.APIError
		if errors.As(err, &apiErr) {
			writeRunRuntimeResolvedError(w, err)
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_request_error")
		return
	}
	req.Input = strings.TrimSpace(req.Input)
	if req.Input == "" {
		writeOpenAIError(w, http.StatusBadRequest, "input is required.", "invalid_request_error", "invalid_request_error")
		return
	}

	body, err := buildRunChatBodyFromExpansion(expansion, req)
	if err != nil {
		writeRunRuntimeResolvedError(w, err)
		return
	}
	expansion.Request.Capability = catalog.CapabilityChat
	expansion.Request.ClientSurface = surface.OpenAIChat
	expansion.Request.Stream = req.Stream
	r.Header.Set("Content-Type", "application/json")
	if req.Stream {
		r.Header.Set("Accept", "text/event-stream")
	}
	s.executeRunRuntimeExpansion(w, r, expansion, body)
}

func decodeRunChatRequestBody(body io.Reader) (runRequest, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return runRequest{}, err
	}
	if err := rejectAppRunPromptJSON(raw); err != nil {
		return runRequest{}, err
	}
	var req runRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return runRequest{}, err
	}
	return req, nil
}

func (s *Gateway) handleRunImageGeneration(w http.ResponseWriter, r *http.Request, expansion coreruntime.InvokeExpansion) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageRequestBodyBytes)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_request_error")
		return
	}
	req, err := decodeRunImageGenerationRequestBody(rawBody)
	if err != nil {
		var apiErr *serving.APIError
		if errors.As(err, &apiErr) {
			writeRunRuntimeResolvedError(w, err)
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_request_error")
		return
	}

	body, err := buildRunImageGenerationBodyFromExpansion(expansion, req)
	if err != nil {
		writeRunRuntimeResolvedError(w, err)
		return
	}
	expansion.Request.Capability = catalog.CapabilityImageGeneration
	expansion.Request.ClientSurface = surface.OpenAIImages
	expansion.Request.Stream = req.Stream
	r.Header.Set("Content-Type", "application/json")
	if req.Stream {
		r.Header.Set("Accept", "text/event-stream")
	}
	s.executeRunRuntimeExpansion(w, r, expansion, body)
}

func (s *Gateway) handleRunImageEdit(w http.ResponseWriter, r *http.Request, expansion coreruntime.InvokeExpansion) {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, mediaErr := mime.ParseMediaType(contentType)
	if mediaErr == nil && mediaType == "application/json" {
		r.Body = http.MaxBytesReader(w, r.Body, maxImageRequestBodyBytes)
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_request_error")
			return
		}
		imageReq, variables, err := decodeAppRunImageEditRequestBody(rawBody, contentType)
		if err != nil {
			writeRunRuntimeResolvedError(w, err)
			return
		}
		body, err := buildRunImageEditBodyFromExpansion(expansion, imageReq, variables)
		if err != nil {
			writeRunRuntimeResolvedError(w, err)
			return
		}
		expansion.Request.Capability = catalog.CapabilityImageEdit
		expansion.Request.ClientSurface = surface.OpenAIImages
		expansion.Request.Stream = imageReq.Stream
		r.Header.Set("Content-Type", "application/json")
		if imageReq.Stream {
			r.Header.Set("Accept", "text/event-stream")
		}
		s.executeRunRuntimeExpansion(w, r, expansion, body)
		return
	}
	if mediaErr != nil || mediaType != "multipart/form-data" {
		writeOpenAIError(w, http.StatusBadRequest, "multipart/form-data is required.", "invalid_request_error", "invalid_request_error")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageRequestBodyBytes)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_request_error")
		return
	}
	imageReq, variables, err := decodeAppRunImageEditRequestBody(rawBody, contentType)
	if err != nil {
		writeRunRuntimeResolvedError(w, err)
		return
	}
	rewrittenBody, err := buildRunImageEditBodyFromExpansion(expansion, imageReq, variables)
	if err != nil {
		writeRunRuntimeResolvedError(w, err)
		return
	}
	expansion.Request.Capability = catalog.CapabilityImageEdit
	expansion.Request.ClientSurface = surface.OpenAIImages
	expansion.Request.Stream = imageReq.Stream
	r.Header.Set("Content-Type", "application/json")
	if imageReq.Stream {
		r.Header.Set("Accept", "text/event-stream")
	}
	s.executeRunRuntimeExpansion(w, r, expansion, rewrittenBody)
}

func writeRunRuntimeResolvedError(w http.ResponseWriter, err error) {
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		writeOpenAIError(w, apiErr.Status, apiErr.Message, apiErr.Code, apiErr.Code)
		return
	}
	switch {
	case errors.Is(err, application.ErrRuntimeInvocationNotFound):
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid app key.", "invalid_api_key", "invalid_api_key")
	case errors.Is(err, application.ErrRuntimeAppNotVisible):
		writeOpenAIError(w, http.StatusNotFound, "Run target is not visible.", "invalid_request_error", "invalid_request_error")
	case errors.Is(err, commercial.ErrClientSurfaceNotAllowed):
		writeOpenAIError(w, http.StatusForbidden, "This API endpoint is not enabled for the selected group.", "invalid_request_error", "endpoint_not_allowed")
	case errors.Is(err, coreruntime.ErrNoDispatchOption), errors.Is(err, coreruntime.ErrNoRouteCandidates):
		writeOpenAIError(w, http.StatusNotFound, "No available runtime target.", "invalid_request_error", "invalid_request_error")
	case errors.Is(err, errRunKeyDisabled):
		writeOpenAIError(w, http.StatusUnauthorized, "App key disabled.", "invalid_api_key", "invalid_api_key")
	case errors.Is(err, errRunKeyExpired):
		writeOpenAIError(w, http.StatusUnauthorized, "App key expired.", "invalid_api_key", "invalid_api_key")
	case errors.Is(err, errRunChatTargetUnsupported),
		errors.Is(err, errRunImageGenUnsupported),
		errors.Is(err, errRunImageEditUnsupported),
		errors.Is(err, errRunAppTypeUnsupported),
		errors.Is(err, errRunAppDisabled),
		errors.Is(err, errRunInputRequired),
		errors.Is(err, errRunAttachmentsNotAllowed),
		errors.Is(err, errRunAttachmentInvalid),
		errors.Is(err, errRunOptionOverrideDenied),
		errors.Is(err, application.ErrPromptInputTooLarge),
		errors.Is(err, application.ErrPromptPlaceholderInvalid),
		errors.Is(err, application.ErrPromptPlaceholderMissing),
		errors.Is(err, application.ErrPromptVariableMissing):
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request_error")
	default:
		writeOpenAIError(w, http.StatusInternalServerError, "Internal server error.", "internal_error", "internal_error")
	}
}

func writeRunRuntimePipelineError(w http.ResponseWriter, err error) {
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, apiErr.Status, apiErr.Message, apiErr.Code)
		return
	}
	var pipeErr *serving.PipelineError
	if errors.As(err, &pipeErr) {
		var inner *serving.APIError
		if errors.As(pipeErr.Cause, &inner) {
			WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, inner.Status, inner.Message, inner.Code)
			return
		}
	}
	WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusInternalServerError, "Internal server error.", "internal_error")
}

type runResponseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newRunResponseCapture() *runResponseCapture {
	return &runResponseCapture{header: http.Header{}}
}

func (c *runResponseCapture) Header() http.Header {
	return c.header
}

func (c *runResponseCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *runResponseCapture) Write(body []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(body)
}

func (c *runResponseCapture) writeTo(w http.ResponseWriter) {
	copyHeader(w.Header(), c.header)
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(c.body.Bytes())
}

// shouldWrapRunResponse reports whether the response should be normalized into
// the unified run payload shape. Every app key is bound to exactly one app, so
// the only thing that disqualifies wrapping is a streaming request (streamed
// bytes are forwarded as-is).
func shouldWrapRunResponse(expansion coreruntime.InvokeExpansion) bool {
	return !expansion.Request.Stream
}

func writeUnifiedRunResponse(w http.ResponseWriter, captured *runResponseCapture, result coreruntime.Result, expansion coreruntime.InvokeExpansion) {
	status := captured.status
	if status == 0 {
		status = result.StatusCode
	}
	if status == 0 {
		status = http.StatusOK
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		captured.writeTo(w)
		return
	}
	copyHeader(w.Header(), captured.header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := unifiedRunPayload(captured.body.Bytes(), result, expansion)
	_ = json.NewEncoder(w).Encode(payload)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func unifiedRunPayload(raw []byte, result coreruntime.Result, expansion coreruntime.InvokeExpansion) map[string]any {
	base := map[string]any{
		"request_id": firstNonEmpty(result.RequestID, expansion.Request.RequestID),
		"usage":      result.Usage,
	}
	switch expansion.App.App.AppType {
	case application.AppTypeImageGenerationAgent, application.AppTypeImageEditAgent:
		base["type"] = "image"
		base["images"] = extractRunImages(raw)
	default:
		base["type"] = "chat"
		base["text"] = extractRunText(raw)
	}
	return base
}

func extractRunText(raw []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return strings.TrimSpace(string(raw))
	}
	if len(payload.Choices) > 0 {
		if text, ok := payload.Choices[0].Message.Content.(string); ok {
			return text
		}
		if payload.Choices[0].Text != "" {
			return payload.Choices[0].Text
		}
	}
	return payload.Text
}

func extractRunImages(raw []byte) []map[string]any {
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &payload) != nil || len(payload.Data) == 0 {
		return []map[string]any{}
	}
	items := payload.Data
	for _, item := range items {
		delete(item, "revised_prompt")
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// expandRunRuntimeInvocation authenticates the app key and resolves its
// bound app. It is independent of the request body — capability and client
// surface are determined afterwards from the app's type, not the URL.
func (s *Gateway) expandRunRuntimeInvocation(r *http.Request) (coreruntime.InvokeExpansion, error) {
	key, err := appkey.ExtractBearer(r.Header.Get("Authorization"))
	if err != nil {
		return coreruntime.InvokeExpansion{}, application.ErrRuntimeInvocationNotFound
	}
	baseReq := coreruntime.Request{
		RequestID:  newRequestID(r),
		TraceID:    r.Header.Get("X-Trace-Id"),
		ReceivedAt: time.Now(),
	}
	expansion, err := s.runtimeInvokeExpander.ExpandByKeyHash(r.Context(), appkey.Hash(key), baseReq)
	if err != nil {
		return coreruntime.InvokeExpansion{}, err
	}
	if expansion.InvokeKey.Status != application.StatusActive {
		return coreruntime.InvokeExpansion{}, errRunKeyDisabled
	}
	if expansion.InvokeKey.ExpiresAt != nil && !time.Now().Before(*expansion.InvokeKey.ExpiresAt) {
		return coreruntime.InvokeExpansion{}, errRunKeyExpired
	}
	return expansion, nil
}

func (s *Gateway) executeRunRuntimeExpansion(w http.ResponseWriter, r *http.Request, expansion coreruntime.InvokeExpansion, body []byte) {
	expansion.Request.Body = body
	serviceTier, err := parseServiceTier(body, r.Header.Get("Content-Type"))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_service_tier")
		return
	}
	expansion.Request.ServiceTier = string(serviceTier)
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	writer := w
	var capture *runResponseCapture
	if shouldWrapRunResponse(expansion) {
		capture = newRunResponseCapture()
		writer = capture
	}
	result, err := s.runtimeEngine.Execute(r.Context(), coreruntime.ExecutionInput{
		Subject: expansion.Subject,
		Request: expansion.Request,
		Envelope: coreruntime.ExecutionEnvelope{
			ResponseWriter: writer,
			HTTPRequest:    r,
			ClientBody:     body,
		},
	})
	if err != nil {
		if result.ResponseCommitted {
			if capture != nil {
				capture.writeTo(w)
			}
			return
		}
		writeRunRuntimePipelineError(w, err)
		return
	}
	if capture != nil {
		writeUnifiedRunResponse(w, capture, result, expansion)
	}
}

func buildRunChatBodyFromExpansion(expansion coreruntime.InvokeExpansion, req runRequest) ([]byte, error) {
	if expansion.App.App.Status != application.StatusActive {
		return nil, errRunAppDisabled
	}
	cfg := application.ParseRuntimeConfig(expansion.App.App.AppType, expansion.App.App.DefaultOptions).Chat
	if cfg == nil {
		return nil, errRunChatTargetUnsupported
	}
	// An app is a sealed product: callers may not override its behaviour.
	// Streaming, however, is always available — it is a transport concern.
	if req.Temperature != nil || req.MaxTokens != nil {
		return nil, errRunOptionOverrideDenied
	}
	if err := validateRunAttachments(req.Attachments, *cfg); err != nil {
		return nil, err
	}
	resolved, err := resolveRunPrompt(expansion, req.Input, req.Variables)
	if err != nil {
		return nil, err
	}
	req.Input = resolved.Input
	return marshalRunChatBody(
		expansion.App.App.BoundModelID,
		resolved.Instruction,
		map[string]any{"temperature": cfg.Temperature()},
		req,
	), nil
}

func buildRunImageGenerationBodyFromExpansion(expansion coreruntime.InvokeExpansion, req runImageGenerationRequest) ([]byte, error) {
	if expansion.App.App.Status != application.StatusActive {
		return nil, errRunAppDisabled
	}
	appConfig := application.ParseRuntimeConfig(expansion.App.App.AppType, expansion.App.App.DefaultOptions).Image
	if appConfig == nil {
		return nil, errRunImageGenUnsupported
	}
	body := map[string]any{"model": expansion.App.App.BoundModelID}
	if err := applyRunImageOptions(body, appConfig, runImageOptions{
		n:              req.N,
		size:           req.Size,
		responseFormat: req.ResponseFormat,
		background:     req.Background,
		outputFormat:   req.OutputFormat,
	}); err != nil {
		return nil, err
	}
	input := strings.TrimSpace(req.Input)
	if input == "" {
		return nil, errRunInputRequired
	}
	resolved, err := resolveRunPrompt(expansion, input, req.Variables)
	if err != nil {
		return nil, err
	}
	prompt := resolved.CombinedText()
	if prompt == "" {
		return nil, errRunInputRequired
	}

	body["prompt"] = prompt
	if req.Stream {
		body["stream"] = true
	}
	return json.Marshal(body)
}

func buildRunImageEditBodyFromExpansion(
	expansion coreruntime.InvokeExpansion,
	req imageedit.Request,
	variables map[string]string,
) ([]byte, error) {
	if expansion.App.App.Status != application.StatusActive {
		return nil, errRunAppDisabled
	}
	appConfig := application.ParseRuntimeConfig(expansion.App.App.AppType, expansion.App.App.DefaultOptions).Image
	if appConfig == nil {
		return nil, errRunImageEditUnsupported
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errRunInputRequired
	}
	resolved, err := resolveRunPrompt(expansion, req.Prompt, variables)
	if err != nil {
		return nil, err
	}
	prompt := resolved.CombinedText()
	if prompt == "" {
		return nil, errRunInputRequired
	}
	if runImageEditOverridesApp(req) {
		return nil, errRunOptionOverrideDenied
	}
	req.Model = expansion.App.App.BoundModelID
	req.Prompt = prompt
	req.Size = application.ResolveOpenAIImageSize(appConfig.Resolution, appConfig.AspectRatio)
	count, ok := appConfig.ResolveOutputCount(req.N)
	if !ok {
		return nil, errRunOptionOverrideDenied
	}
	req.N = count
	req.ResponseFormat = normalizeRunResponseFormat(req.ResponseFormat)
	return imageedit.CanonicalJSON(req)
}

func marshalRunChatBody(modelCode, systemPrompt string, upstream map[string]any, req runRequest) []byte {
	body := map[string]any{}
	for k, v := range upstream {
		if k == "model" || k == "messages" || k == "stream" {
			continue
		}
		body[k] = v
	}
	body["model"] = modelCode
	body["stream"] = req.Stream
	messages := make([]map[string]any, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": systemPrompt})
	}
	messages = append(messages, map[string]any{"role": "user", "content": runChatUserContent(req)})
	body["messages"] = messages
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	}
	raw, _ := json.Marshal(body)
	return raw
}

func runChatUserContent(req runRequest) any {
	if len(req.Attachments) == 0 {
		return req.Input
	}
	parts := make([]map[string]any, 0, len(req.Attachments)+1)
	parts = append(parts, map[string]any{"type": "text", "text": req.Input})
	for _, item := range req.Attachments {
		kind := strings.TrimSpace(item.Type)
		// When the caller omits type, infer image from the MIME type, then fall
		// back to the URL extension so a bare image URL is not mis-sent as a file.
		if kind == "" && (strings.HasPrefix(strings.ToLower(item.MIMEType), "image/") || looksLikeImageURL(item.URL)) {
			kind = "image"
		}
		switch kind {
		case "image":
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": item.URL,
				},
			})
		default:
			file := map[string]any{"file_url": item.URL}
			if strings.TrimSpace(item.Name) != "" {
				file["filename"] = strings.TrimSpace(item.Name)
			}
			parts = append(parts, map[string]any{
				"type": "file",
				"file": file,
			})
		}
	}
	return parts
}

// runImageOptions carries the image option fields a caller may send on a run
// request. The app fixes the resolution and only response_format is honoured.
type runImageOptions struct {
	n              int
	size           string
	responseFormat string
	background     string
	outputFormat   string
}

func applyRunImageOptions(body map[string]any, appConfig *application.ImageRuntimeConfig, opts runImageOptions) error {
	if opts.size != "" || opts.background != "" || opts.outputFormat != "" {
		return errRunOptionOverrideDenied
	}
	body["size"] = application.ResolveOpenAIImageSize(appConfig.Resolution, appConfig.AspectRatio)
	count, ok := appConfig.ResolveOutputCount(opts.n)
	if !ok {
		return errRunOptionOverrideDenied
	}
	if count > application.DefaultImageOutputCount {
		body["n"] = count
	}
	body["response_format"] = normalizeRunResponseFormat(opts.responseFormat)
	return nil
}

// normalizeRunResponseFormat resolves the caller's requested image return format,
// defaulting to b64_json. This is the one image option a caller may choose on an app.
func normalizeRunResponseFormat(value string) string {
	if strings.TrimSpace(value) == domain.ImageResponseFormatURL {
		return domain.ImageResponseFormatURL
	}
	return domain.ImageResponseFormatB64
}

// looksLikeImageURL reports whether a URL's path ends in a common image
// extension, ignoring any query string or fragment.
func looksLikeImageURL(rawURL string) bool {
	path := strings.TrimSpace(rawURL)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.ToLower(path)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".avif"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func decodeStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	return out
}

func validateRunAttachments(items []runAttachment, cfg application.ChatRuntimeConfig) error {
	if len(items) == 0 {
		return nil
	}
	if !cfg.AllowAttachments || len(items) > cfg.MaxAttachments() {
		return errRunAttachmentsNotAllowed
	}
	for _, item := range items {
		parsed, err := url.Parse(strings.TrimSpace(item.URL))
		if err != nil || parsed == nil || parsed.Host == "" {
			return errRunAttachmentInvalid
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			return errRunAttachmentInvalid
		}
	}
	return nil
}

func runImageEditOverridesApp(req imageedit.Request) bool {
	return strings.TrimSpace(req.Model) != "" || strings.TrimSpace(req.Size) != "" ||
		strings.TrimSpace(req.Background) != "" ||
		strings.TrimSpace(req.InputFidelity) != "" || strings.TrimSpace(req.Moderation) != "" ||
		strings.TrimSpace(req.OutputFormat) != "" || req.OutputCompression != nil || strings.TrimSpace(req.User) != ""
}

func cloneRunMIMEHeader(h textproto.MIMEHeader) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader, len(h))
	for key, values := range h {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func resolveRunPrompt(expansion coreruntime.InvokeExpansion, input string, variables map[string]string) (application.ResolvedPrompt, error) {
	if expansion.App == nil {
		return application.ResolvedPrompt{}, errRunAppTypeUnsupported
	}
	strategy := expansion.App.App.PromptStrategy
	return application.ResolvePrompt(strategy, application.PromptResolveInput{
		Input: input, Variables: variables, Bindings: expansion.App.PromptBindings,
	})
}
