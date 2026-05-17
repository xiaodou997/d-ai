package serving

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats"
	"uni-ai-api/backend/internal/formats/claude"
	"uni-ai-api/backend/internal/formats/gemini"
	"uni-ai-api/backend/internal/formats/openai/responses"
)

// OAuthCredentialPool handles credential lifecycle for OAuth upstreams.
type OAuthCredentialPool interface {
	SelectCredentialFromPool(ctx context.Context, endpointID, strategy string) (*domain.OAuthCredential, error)
	MarkInvalid(ctx context.Context, credID, reason string) error
	RecordSuccess(ctx context.Context, credID string)
	RecordFailure(ctx context.Context, credID, reason string)
}

// ExecuteStep forwards the request to the upstream and writes the response to
// the HTTP client. It populates req.TokenUsage, req.HTTPStatus, etc.
type ExecuteStep struct {
	Transport   Transporter
	Breaker     CircuitBreakerNotifier // optional; nil = no circuit breaking
	OAuthPool   OAuthCredentialPool   // optional; enables 401 retry for OAuth upstreams
}

// Transporter makes the actual HTTP call to an upstream provider.
type Transporter interface {
	Do(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error)
}

// CircuitBreakerNotifier receives upstream call outcomes per deployment.
type CircuitBreakerNotifier interface {
	RecordSuccess(deploymentID string)
	RecordFailure(ctx context.Context, deploymentID string, errMsg string)
}

// UpstreamRequest contains everything needed to call an upstream endpoint.
type UpstreamRequest struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     []byte
	Timeout  time.Duration
	Protocol domain.UpstreamProtocol
}

// UpstreamResponse is the upstream HTTP response.
type UpstreamResponse struct {
	StatusCode int
	Headers    http.Header
	Body       io.ReadCloser
}

func (s *ExecuteStep) Name() string { return "execute" }

func (s *ExecuteStep) Execute(ctx context.Context, req *Request) error {
	candidate := req.Candidate
	if candidate == nil {
		return fmt.Errorf("no route candidate")
	}

	body, err := buildUpstreamBody(req)
	if err != nil {
		return err
	}
	req.UpstreamBodySize = len(body)

	resp, startTime, err := s.doUpstream(ctx, req, body)
	if err != nil {
		return err
	}

	// OAuth 401: mark credential invalid and retry once with a new one.
	if resp.StatusCode == http.StatusUnauthorized && req.SelectedCredential != nil && s.OAuthPool != nil {
		body401, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()

		// Persist the first attempt's metrics so observability (and the
		// breaker) see the 401 even when the retry succeeds and overwrites
		// req.UpstreamStatus/LatencyMs further down.
		req.UpstreamStatus = http.StatusUnauthorized
		req.LatencyMs = int(time.Since(startTime).Milliseconds())
		if s.Breaker != nil {
			s.Breaker.RecordFailure(ctx, candidate.DeploymentID, "http 401")
		}

		oldCredID := req.SelectedCredential.ID
		slog.WarnContext(ctx, "upstream 401, retrying with new credential",
			"old_cred_id", oldCredID,
			"upstream_body", string(body401),
		)
		_ = s.OAuthPool.MarkInvalid(ctx, oldCredID, "upstream 401 unauthorized")
		s.OAuthPool.RecordFailure(ctx, oldCredID, "401 unauthorized")

		newCred, selectErr := s.OAuthPool.SelectCredentialFromPool(ctx, candidate.PoolID, candidate.OAuthStrategy)
		if selectErr != nil {
			return &APIError{
				Status:  http.StatusServiceUnavailable,
				Code:    "no_credential",
				Message: "no available credential after 401 retry: " + selectErr.Error(),
			}
		}
		req.SelectedCredential = newCred
		resp, startTime, err = s.doUpstream(ctx, req, body)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	req.UpstreamStatus = resp.StatusCode
	req.LatencyMs = int(time.Since(startTime).Milliseconds())

	deploymentID := candidate.DeploymentID

	// Non-2xx from upstream
	if resp.StatusCode >= 400 {
		req.RequestStatus = domain.RequestFailed
		req.HTTPStatus = resp.StatusCode
		req.ErrorCode = "upstream_http_error"
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		req.ErrorMessage = truncateValidUTF8(string(bodyBytes), 1024)
		if s.Breaker != nil {
			s.Breaker.RecordFailure(ctx, deploymentID, fmt.Sprintf("http %d", resp.StatusCode))
		}
		if req.SelectedCredential != nil && s.OAuthPool != nil {
			s.OAuthPool.RecordFailure(ctx, req.SelectedCredential.ID, fmt.Sprintf("http %d", resp.StatusCode))
		}
		return &APIError{
			Status:  upstreamStatusToGateway(resp.StatusCode),
			Code:    "upstream_error",
			Message: "upstream returned error",
		}
	}

	if s.Breaker != nil {
		s.Breaker.RecordSuccess(deploymentID)
	}
	if req.SelectedCredential != nil && s.OAuthPool != nil {
		s.OAuthPool.RecordSuccess(ctx, req.SelectedCredential.ID)
	}

	// Route to streaming or non-streaming execution
	if req.IsStream {
		return s.executeStream(ctx, req, resp, req.W, startTime)
	}
	return s.executeSync(ctx, req, resp, req.W)
}

// doUpstream builds headers, fires the upstream request, and returns the response.
func (s *ExecuteStep) doUpstream(ctx context.Context, req *Request, body []byte) (*UpstreamResponse, time.Time, error) {
	candidate := req.Candidate
	upReq := &UpstreamRequest{
		Method:   "POST",
		URL:      buildURL(candidate, req.IsStream),
		Headers:  buildHeaders(candidate, req),
		Body:     body,
		Timeout:  time.Duration(candidate.TimeoutMs) * time.Millisecond,
		Protocol: candidate.Protocol,
	}

	startTime := time.Now()
	resp, err := s.Transport.Do(ctx, upReq)
	if err != nil {
		req.RequestStatus = domain.RequestFailed
		req.ErrorCode = "upstream_error"
		req.ErrorMessage = err.Error()
		if s.Breaker != nil {
			s.Breaker.RecordFailure(ctx, candidate.DeploymentID, err.Error())
		}
		return nil, startTime, apiError(http.StatusBadGateway, "upstream_error", "upstream request failed")
	}
	return resp, startTime, nil
}

func (s *ExecuteStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// Non-streaming execution
// ============================================================================

func (s *ExecuteStep) executeSync(ctx context.Context, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32MB max
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}

	bodyBytes = unwrapCodeAssistResponse(req.Candidate, bodyBytes)

	canonical, err := formats.ToCanonicalChatResponse(bodyBytes, req.Candidate.Protocol, req.RequestID, req.Candidate.UpstreamModel)
	if err != nil {
		return fmt.Errorf("parse upstream response: %w", err)
	}

	// Extract usage for billing
	if canonical.Usage != nil {
		req.TokenUsage = domain.TokenUsage{
			PromptTokens:     canonical.Usage.PromptTokens,
			CompletionTokens: canonical.Usage.CompletionTokens,
			CacheWriteTokens: canonical.Usage.CacheWriteTokens,
			CacheReadTokens:  canonical.Usage.CacheReadTokens,
			ReasoningTokens:  canonical.Usage.ReasoningTokens,
		}
	}
	fillEstimatedUsage(req, len(bodyBytes))

	// Ensure the model field in the response shows the public model code
	canonical.Model = req.ModelCode

	outBytes, err := formats.CanonicalResponseToOpenAI(canonical)
	if err != nil {
		return fmt.Errorf("serialise response: %w", err)
	}

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(outBytes)
	return nil
}

// ============================================================================
// Streaming execution
// ============================================================================

func (s *ExecuteStep) executeStream(ctx context.Context, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support streaming")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	protocol := req.Candidate.Protocol
	responseID := req.RequestID
	upstreamModel := req.Candidate.UpstreamModel
	firstToken := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var eventType string // used for Anthropic SSE event: field
	var accumulatedOutputBytes int

	for scanner.Scan() {
		line := scanner.Text()

		// Anthropic SSE has "event: <type>" lines
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			eventType = ""
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}

		dataBytes := unwrapCodeAssistResponse(req.Candidate, []byte(data))
		chunk, err := formats.ToCanonicalStreamChunk(
			dataBytes, eventType, protocol, responseID, upstreamModel, 0,
		)
		eventType = ""

		if err != nil || chunk == nil {
			continue
		}

		// Track first-token latency
		if !firstToken && len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
			firstToken = true
		}

		// Accumulate usage from final chunk
		if chunk.Usage != nil {
			req.TokenUsage = domain.TokenUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				CacheWriteTokens: chunk.Usage.CacheWriteTokens,
				CacheReadTokens:  chunk.Usage.CacheReadTokens,
				ReasoningTokens:  chunk.Usage.ReasoningTokens,
			}
			accumulatedOutputBytes = 0 // upstream provided real counts, reset estimator
		} else if chunk.Choices != nil {
			for _, c := range chunk.Choices {
				accumulatedOutputBytes += len(c.Delta.Content)
			}
		}

		// Always report the public model code to the client
		chunk.Model = req.ModelCode

		outBytes, err := formats.CanonicalChunkToOpenAI(chunk)
		if err != nil {
			continue
		}

		_, _ = fmt.Fprintf(w, "data: %s\n\n", outBytes)
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		req.RequestStatus = domain.RequestFailed
		req.ErrorCode = "stream_read_error"
		req.ErrorMessage = err.Error()
		_, _ = fmt.Fprintf(w, "data: {\"error\":{\"message\":\"stream read error\",\"type\":\"stream_error\",\"code\":\"stream_read_error\"}}\n\ndata: [DONE]\n\n")
		flusher.Flush()
		return nil
	}

	fillEstimatedUsage(req, accumulatedOutputBytes)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = http.StatusOK
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func buildUpstreamBody(req *Request) ([]byte, error) {
	switch {
	case req.ChatReq != nil && req.Candidate.Protocol == domain.ProtocolOpenAIResponses:
		respReq := responses.FromCanonical(req.ChatReq, req.Candidate.UpstreamModel)
		if req.Candidate.FixedProviderType == domain.FixedProviderCodex {
			responses.ApplyCodexModifications(respReq)
		}
		return json.Marshal(respReq)

	case req.ChatReq != nil:
		body, err := formats.ToUpstreamChatRequest(req.ChatReq, req.Candidate.Protocol, req.Candidate.UpstreamModel)
		if err != nil {
			return nil, err
		}
		return applyOAuthBodyTransform(req, body)

	case req.EmbedReq != nil:
		embedCopy := *req.EmbedReq
		embedCopy.Model = req.Candidate.UpstreamModel
		return json.Marshal(embedCopy)

	case req.ImageReq != nil:
		imgCopy := *req.ImageReq
		imgCopy.Model = req.Candidate.UpstreamModel
		return json.Marshal(imgCopy)

	default:
		return nil, fmt.Errorf("no request payload")
	}
}

func buildURL(c *domain.RouteCandidate, isStream bool) string {
	// gemini_cli / antigravity use Google's internal CodeAssist endpoint with
	// its own path template, independent of c.RequestPath.
	if c.FixedProviderType == domain.FixedProviderGeminiCLI ||
		c.FixedProviderType == domain.FixedProviderAntigravity {
		action := gemini.CLISyncGenerate
		if isStream {
			action = gemini.CLIStreamGenerate
		}
		return gemini.BuildCLIURL(c.BaseURL, action)
	}

	base := strings.TrimRight(c.BaseURL, "/")
	path := c.RequestPath
	if path == "" {
		path = defaultPath(c.Protocol)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := base + path
	// {model} placeholder for public Gemini API (gateway-internal candidates
	// pre-fill UpstreamModel, but path templates may still embed the token).
	if strings.Contains(url, "{model}") {
		url = strings.ReplaceAll(url, "{model}", c.UpstreamModel)
	}
	return url
}

func defaultPath(protocol domain.UpstreamProtocol) string {
	switch protocol {
	case domain.ProtocolOpenAIChat:
		return "/v1/chat/completions"
	case domain.ProtocolOpenAIResponses:
		return "/v1/responses"
	case domain.ProtocolOpenAICompletions:
		return "/v1/completions"
	case domain.ProtocolOpenAIEmbeddings:
		return "/v1/embeddings"
	case domain.ProtocolOpenAIImages:
		return "/v1/images/generations"
	case domain.ProtocolAnthropicMessages:
		return "/v1/messages"
	case domain.ProtocolGeminiGenerate:
		return "/v1beta/models/{model}:generateContent"
	case domain.ProtocolGeminiEmbeddings:
		return "/v1beta/models/{model}:embedContent"
	default:
		return "/v1/chat/completions"
	}
}

func buildHeaders(c *domain.RouteCandidate, req *Request) map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if req.IsStream {
		headers["Accept"] = "text/event-stream"
	}

	cred := req.SelectedCredential

	if cred != nil {
		// OAuth credential takes precedence over stored API key.
		headers["Authorization"] = "Bearer " + cred.AccessToken

		// Codex-specific headers.
		if c.FixedProviderType == domain.FixedProviderCodex {
			headers["user-agent"] = "codex-tui/0.122.0 (Mac OS 15.2.0; arm64) vscode/2.6.11"
			headers["originator"] = "codex-tui"
			if accountID := cred.AccountID(); accountID != "" {
				headers["chatgpt-account-id"] = accountID
			}
			// Stable session/conversation IDs derived from (api_key_id + pool_id).
			sessionID := codexSessionID(req.APIKey.KeyID, c.PoolID)
			headers["session_id"] = sessionID
			headers["conversation_id"] = sessionID
		}

		// Claude Code OAuth — anthropic-beta / version / x-app etc.
		if c.FixedProviderType == domain.FixedProviderClaudeOAuth {
			incomingBeta := ""
			if req.R != nil {
				incomingBeta = req.R.Header.Get("anthropic-beta")
			}
			claude.ApplyOAuthHeaders(headers, incomingBeta)
		}

		// Gemini CLI / Antigravity — user-agent (Antigravity UA lives in body).
		if c.FixedProviderType == domain.FixedProviderGeminiCLI {
			headers["user-agent"] = gemini.CLIUserAgent()
		}
	} else {
		// API-key auth — varies by protocol.
		switch c.Protocol {
		case domain.ProtocolAnthropicMessages:
			headers["x-api-key"] = c.APIKeyCiphertext
			headers["anthropic-version"] = "2023-06-01"
		case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
			// Gemini uses query param ?key=, attached by transport.resolveURL.
			headers["x-gemini-api-key"] = c.APIKeyCiphertext
		default:
			headers["Authorization"] = "Bearer " + c.APIKeyCiphertext
		}
	}

	// Extra headers from endpoint config (can override defaults).
	for k, v := range c.ExtraHeaders {
		headers[k] = v
	}
	return headers
}

// unwrapCodeAssistResponse peels the {"response": {...}} envelope returned by
// Google's CodeAssist endpoint (gemini_cli / antigravity) so the rest of the
// pipeline can parse a vanilla Gemini response shape. Returns the input
// unchanged for non-CodeAssist providers or when no envelope is present.
func unwrapCodeAssistResponse(c *domain.RouteCandidate, body []byte) []byte {
	if c == nil {
		return body
	}
	if c.FixedProviderType != domain.FixedProviderGeminiCLI &&
		c.FixedProviderType != domain.FixedProviderAntigravity {
		return body
	}
	if len(body) == 0 {
		return body
	}
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Response) == 0 {
		return body
	}
	return env.Response
}

// applyOAuthBodyTransform mutates a freshly-serialised upstream body when the
// candidate is an OAuth fixed provider whose API contract differs from the
// vanilla protocol (claude_oauth sanitisation, gemini_cli/antigravity envelope
// wrapping).
func applyOAuthBodyTransform(req *Request, body []byte) ([]byte, error) {
	c := req.Candidate
	switch c.FixedProviderType {
	case domain.FixedProviderClaudeOAuth:
		return claude.SanitizeOAuthRequestBody(body)

	case domain.FixedProviderGeminiCLI:
		projectID := ""
		if req.SelectedCredential != nil {
			projectID = gemini.ProjectIDFromMetadata(req.SelectedCredential.AuthMetadata)
		}
		return gemini.WrapCLIRequest(body, c.UpstreamModel, projectID)

	case domain.FixedProviderAntigravity:
		projectID := ""
		if req.SelectedCredential != nil {
			projectID = gemini.ProjectIDFromMetadata(req.SelectedCredential.AuthMetadata)
		}
		return gemini.WrapAntigravityRequest(body, c.UpstreamModel, projectID, req.RequestID)
	}
	return body, nil
}

// codexSessionID returns the first 16 hex characters of SHA256(keyID+endpointID).
func codexSessionID(keyID, endpointID string) string {
	h := sha256.Sum256([]byte(keyID + endpointID))
	return hex.EncodeToString(h[:])[:16]
}

func upstreamStatusToGateway(code int) int {
	switch {
	case code == 429:
		return http.StatusTooManyRequests
	case code >= 500:
		return http.StatusBadGateway
	case code == 401 || code == 403:
		return http.StatusBadGateway // auth error is our config issue, not client's
	default:
		return code
	}
}

// fillEstimatedUsage sets TokenUsage to conservative byte-based estimates when
// the upstream did not return token counts (PromptTokens == 0). outputBytes is
// the response body size for sync calls, or the accumulated delta-content length
// for streaming calls.
//
// Estimation formula: bytes / 3 (rounded up). Dividing by 3 is conservative —
// English text is ~4 chars/token but CJK and code are closer to 2–3 bytes/token,
// so using 3 avoids systematic under-billing.
func fillEstimatedUsage(req *Request, outputBytes int) {
	if req.TokenUsage.PromptTokens != 0 {
		req.TokenCountSource = "upstream"
		return
	}
	req.TokenCountSource = "estimated"
	if req.UpstreamBodySize > 0 {
		req.TokenUsage.PromptTokens = (req.UpstreamBodySize + 2) / 3
	}
	if outputBytes > 0 {
		req.TokenUsage.CompletionTokens = (outputBytes + 2) / 3
	} else {
		req.TokenUsage.CompletionTokens = 256 // minimum fallback when output is not measurable
	}
}

// truncateValidUTF8 clips s to at most maxBytes bytes while keeping the result
// valid UTF-8 (never cuts a multi-byte sequence in half).
func truncateValidUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

