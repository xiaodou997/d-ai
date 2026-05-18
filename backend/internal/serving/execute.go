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
	"uni-ai-api/backend/internal/routing"
)

// stickyWriter writes a sticky binding after a successful upstream call.
// The interface is satisfied by redis.RedisSticky; nil = feature disabled.
type stickyWriter interface {
	SetBinding(ctx context.Context, tenantID, identity, model, convID string, b *routing.StickyBinding) error
	DeleteBinding(ctx context.Context, tenantID, identity, model, convID string) error
}

// OAuthCredentialPool handles credential lifecycle for OAuth upstreams.
type OAuthCredentialPool interface {
	SelectCredentialFromPool(ctx context.Context, endpointID, strategy string) (*domain.OAuthCredential, error)
	MarkInvalid(ctx context.Context, credID, reason string) error
	RecordSuccess(ctx context.Context, credID string)
	RecordFailure(ctx context.Context, credID, reason string)
}

// ExecuteStep is the upstream call + retry loop. It pulls candidates from
// req.Candidates, attempts them in order subject to RetryBudget, classifies
// each outcome, and either retries with a different route, swaps the OAuth
// credential, gives up (4xx), or relays the successful response to the client
// via the protocol-appropriate Relay.
type ExecuteStep struct {
	Transport Transporter
	Health    routing.HealthTracker   // optional; nil = no circuit breaking
	OAuthPool OAuthCredentialPool     // optional; enables 401 credential swap
	Budget    RetryBudget             // zero value falls back to DefaultRetryBudget
	Scorer    RouteScorer             // optional; nil = first unused candidate (P1 behaviour)
	Stats     routing.RouteStatsStore // optional; used for inflight tracking alongside scorer
	Sticky    stickyWriter            // optional; writes/deletes sticky binding on success/failure
}

// Transporter makes the actual HTTP call to an upstream provider.
type Transporter interface {
	Do(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error)
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
	if len(req.Candidates) == 0 {
		return apiError(http.StatusServiceUnavailable, "no_available_route", "no route candidates")
	}
	if req.Envelope == nil || req.Envelope.W == nil {
		return apiError(http.StatusInternalServerError, "missing_envelope", "request envelope not set")
	}

	budget := s.Budget
	if budget.MaxAttempts == 0 {
		budget = DefaultRetryBudget()
	}
	deadline := time.Now().Add(budget.TotalTimeout)

	body, err := s.prepareBody(req)
	if err != nil {
		return err
	}
	req.UpstreamBodySize = len(body)

	var lastErr error
	for attempt := 1; attempt <= budget.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return apiError(http.StatusGatewayTimeout, "client_disconnected", err.Error())
		}

		cand, score := s.pickCandidate(ctx, req)
		if cand == nil {
			break // exhausted all candidates
		}
		req.Candidate = cand

		// Pool routes: select a fresh credential per attempt so 401-swap and
		// new-route paths both get a clean credential.
		if cand.IsPoolRoute() && s.OAuthPool != nil && req.SelectedCredential == nil {
			cred, selErr := s.OAuthPool.SelectCredentialFromPool(ctx, cand.PoolID, cand.OAuthStrategy)
			if selErr != nil {
				slog.WarnContext(ctx, "pool credential selection failed",
					"pool_id", cand.PoolID, "error", selErr)
				req.UsedCandidates[cand.RouteID] = true
				lastErr = apiError(http.StatusServiceUnavailable, "no_credential", selErr.Error())
				continue
			}
			req.SelectedCredential = cred
		}

		// 429 backoff: applies on retry only and only when the previous
		// attempt was rate-limited.
		if attempt > 1 && lastAttemptWas(req, ResultRateLimited) {
			if delay := budget.BackoffFor(attempt); delay > 0 {
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return apiError(http.StatusGatewayTimeout, "client_disconnected", ctx.Err().Error())
				}
			}
		}

		// Rebuild body if the protocol/upstream model differs from the previous
		// attempt (e.g. retry from openai_chat → anthropic_messages).
		if attempt > 1 && candidateNeedsBodyRebuild(req, cand) {
			newBody, berr := s.prepareBody(req)
			if berr != nil {
				return berr
			}
			body = newBody
			req.UpstreamBodySize = len(body)
		}

		perTimeout := budget.PerAttemptTimeout(deadline, cand.TimeoutMs)
		if perTimeout <= 0 {
			lastErr = apiError(http.StatusGatewayTimeout, "retry_budget_exhausted", "no time remaining for upstream call")
			break
		}

		result := s.runAttempt(ctx, req, cand, body, perTimeout, score)
		if result.finished {
			return result.finalErr
		}
		// Non-terminal outcomes — record and loop.
		lastErr = result.finalErr
		switch result.decision {
		case DecisionRetryNewCred:
			// Same route, fresh credential next iteration.
			req.SelectedCredential = nil
		case DecisionRetry:
			req.UsedCandidates[cand.RouteID] = true
			req.SelectedCredential = nil
		}
	}

	// Budget exhausted or candidates depleted.
	req.RequestStatus = domain.RequestFailed
	if req.ErrorCode == "" {
		req.ErrorCode = "retry_budget_exhausted"
	}
	if lastErr != nil {
		return lastErr
	}
	return apiError(http.StatusBadGateway, "retry_budget_exhausted",
		fmt.Sprintf("all %d upstream attempts failed", len(req.Attempts)))
}

// attemptResult is what one runAttempt invocation tells the loop to do next.
type attemptResult struct {
	finished bool     // true = exit Execute, return finalErr (nil = success)
	decision Decision // valid when finished=false
	finalErr error    // success error (nil) when finished, otherwise lastErr to remember
}

// runAttempt executes one upstream call and handles the response. It owns the
// per-attempt timeout context (defer cancel) so the Execute loop never leaks
// a context. For the accept case the relay finishes BEFORE this function
// returns, so cancel firing afterward is safe.
func (s *ExecuteStep) runAttempt(parentCtx context.Context, req *Request, cand *domain.RouteCandidate, body []byte, perTimeout time.Duration, score float64) attemptResult {
	attemptCtx, cancel := context.WithTimeout(parentCtx, perTimeout)
	defer cancel()

	// P3: track inflight count; always decrement even on error.
	if s.Stats != nil {
		s.Stats.IncrInflight(parentCtx, cand.RouteID)
		defer s.Stats.DecrInflight(parentCtx, cand.RouteID)
	}

	startTime := time.Now()
	upResp, callErr := s.Transport.Do(attemptCtx, &UpstreamRequest{
		Method:   "POST",
		URL:      buildURL(cand, req.IsStream),
		Headers:  buildHeaders(cand, req),
		Body:     body,
		Timeout:  perTimeout,
		Protocol: cand.Protocol,
	})
	latencyMs := int(time.Since(startTime).Milliseconds())

	// P3: record latency for EWMA update (only on completed calls, not context cancellations).
	if s.Stats != nil && callErr == nil {
		s.Stats.RecordLatency(parentCtx, cand.RouteID, latencyMs)
	}

	status := 0
	if upResp != nil {
		status = upResp.StatusCode
	}
	outcome := ClassifyOutcome(status, callErr)
	req.UpstreamStatus = status
	req.LatencyMs = latencyMs
	s.recordAttempt(req, cand, outcome, latencyMs, score)
	s.notifyHealth(parentCtx, req, cand, outcome)

	switch outcome.Decision(req.SelectedCredential != nil) {
	case DecisionAccept:
		err := s.relay(attemptCtx, req, upResp, startTime)
		if err == nil {
			s.writeSticky(parentCtx, req, cand)
		}
		return attemptResult{finished: true, finalErr: err}

	case DecisionRetryNewCred:
		drainAndClose(upResp)
		if s.OAuthPool != nil && req.SelectedCredential != nil {
			oldCredID := req.SelectedCredential.ID
			slog.WarnContext(parentCtx, "upstream 401, swapping credential",
				"old_cred_id", oldCredID)
			_ = s.OAuthPool.MarkInvalid(parentCtx, oldCredID, "upstream 401 unauthorized")
		}
		return attemptResult{
			decision: DecisionRetryNewCred,
			finalErr: apiError(http.StatusBadGateway, "upstream_error", "credential rejected"),
		}

	case DecisionRetry:
		errBody := snippetBody(upResp)
		drainAndClose(upResp)
		return attemptResult{
			decision: DecisionRetry,
			finalErr: apiError(upstreamStatusToGateway(status), "upstream_error",
				fmt.Sprintf("upstream returned %d: %s", status, errBody)),
		}

	case DecisionGiveUp:
		errBody := snippetBody(upResp)
		drainAndClose(upResp)
		req.RequestStatus = domain.RequestFailed
		req.HTTPStatus = status
		req.ErrorCode = "upstream_http_error"
		req.ErrorMessage = truncateValidUTF8(errBody, 1024)
		return attemptResult{
			finished: true,
			finalErr: apiError(upstreamStatusToGateway(status), "upstream_error",
				fmt.Sprintf("upstream returned %d", status)),
		}

	default:
		drainAndClose(upResp)
		return attemptResult{decision: DecisionRetry}
	}
}

func (s *ExecuteStep) prepareBody(req *Request) ([]byte, error) {
	body, err := buildUpstreamBody(req)
	if err != nil {
		return nil, err
	}
	req.UpstreamBody = body
	return body, nil
}

// pickCandidate returns the next candidate using the multi-dim scorer when
// available, or falls back to linear priority order (P1 behaviour).
// Health gate (IsBlocked) is applied here — only on the candidate about to be
// used — to avoid consuming HALF_OPEN probe slots for unchosen candidates.
func (s *ExecuteStep) pickCandidate(ctx context.Context, req *Request) (*domain.RouteCandidate, float64) {
	for {
		var cand *domain.RouteCandidate
		var score float64
		if sp, ok := s.Scorer.(ScoringPicker); ok {
			cand, score = sp.PickWithScore(ctx, req.Candidates, req.UsedCandidates)
		} else if s.Scorer != nil {
			cand = s.Scorer.Pick(ctx, req.Candidates, req.UsedCandidates)
		} else {
			for _, c := range req.Candidates {
				if !req.UsedCandidates[c.RouteID] {
					cand = c
					break
				}
			}
		}
		if cand == nil {
			return nil, 0
		}
		// IsBlocked is called only now — for deployment routes that are OPEN the
		// first expired probe window atomically transitions to HALF_OPEN and returns
		// false (allowing exactly one probe). Calling it earlier (in SelectCandidates)
		// would waste probe slots on candidates the scorer doesn't ultimately choose.
		if s.Health != nil && !cand.IsPoolRoute() {
			if s.Health.IsBlocked(cand.DeploymentID) {
				req.UsedCandidates[cand.RouteID] = true
				continue
			}
		}
		return cand, score
	}
}

// relay dispatches to sync or streaming relay based on req.IsStream.
func (s *ExecuteStep) relay(ctx context.Context, req *Request, upResp *UpstreamResponse, startTime time.Time) error {
	if req.IsStream {
		return s.executeStream(ctx, req, upResp, req.Envelope.W, startTime)
	}
	return s.executeSync(ctx, req, upResp, req.Envelope.W)
}

// recordAttempt appends a structured trace record for this attempt.
func (s *ExecuteStep) recordAttempt(req *Request, cand *domain.RouteCandidate, outcome Outcome, latencyMs int, score float64) {
	targetID := cand.DeploymentID
	if cand.IsPoolRoute() {
		if req.SelectedCredential != nil {
			targetID = req.SelectedCredential.ID
		} else {
			targetID = cand.PoolID
		}
	}
	errMsg := ""
	if outcome.Err != nil {
		errMsg = outcome.Err.Error()
	}
	req.Attempts = append(req.Attempts, AttemptRecord{
		RouteID:    cand.RouteID,
		TargetID:   targetID,
		HTTPStatus: outcome.HTTPStatus,
		Outcome:    outcome.Status,
		LatencyMs:  latencyMs,
		ErrorMsg:   errMsg,
		Score:      score,
	})
}

// notifyHealth records the outcome with the HealthTracker and the OAuth pool.
// 429 is intentionally excluded from health failure accounting (see
// CountsAsHealthFailure). Both deployment and credential targets share the
// same HealthTracker interface; TargetKind distinguishes them in Snapshot.
func (s *ExecuteStep) notifyHealth(ctx context.Context, req *Request, cand *domain.RouteCandidate, outcome Outcome) {
	if s.Health != nil {
		targetID, kind := healthTarget(cand, req)
		switch outcome.Status {
		case ResultSuccess:
			s.Health.RecordSuccess(targetID, kind)
		default:
			if outcome.CountsAsHealthFailure() {
				s.Health.RecordFailure(targetID, kind)
			}
		}
	}

	// OAuthPool persists credential failures to DB independently of the
	// in-memory HealthTracker so that invalid credentials survive restarts.
	if cand.IsPoolRoute() && s.OAuthPool != nil && req.SelectedCredential != nil {
		credID := req.SelectedCredential.ID
		switch outcome.Status {
		case ResultSuccess:
			s.OAuthPool.RecordSuccess(ctx, credID)
		case ResultUnauthorized:
			s.OAuthPool.RecordFailure(ctx, credID, "401 unauthorized")
		default:
			if outcome.CountsAsHealthFailure() {
				msg := fmt.Sprintf("http %d", outcome.HTTPStatus)
				if outcome.Err != nil {
					msg = outcome.Err.Error()
				}
				s.OAuthPool.RecordFailure(ctx, credID, msg)
			}
		}
	}
}

// healthTarget resolves the HealthTracker target ID and kind for a candidate.
func healthTarget(cand *domain.RouteCandidate, req *Request) (string, routing.TargetKind) {
	if cand.IsPoolRoute() {
		if req.SelectedCredential != nil {
			return req.SelectedCredential.ID, routing.TargetCredential
		}
		return cand.PoolID, routing.TargetCredential
	}
	return cand.DeploymentID, routing.TargetDeployment
}

func (s *ExecuteStep) Rollback(_ context.Context, _ *Request) {}

// writeSticky persists the sticky binding for this conversation after a
// successful upstream call. No-op when Sticky is nil or req.ConversationID is
// empty (opt-in by caller).
func (s *ExecuteStep) writeSticky(ctx context.Context, req *Request, cand *domain.RouteCandidate) {
	if s.Sticky == nil || req.ConversationID == "" || req.APIKey == nil {
		return
	}
	var b routing.StickyBinding
	b.RouteID = cand.RouteID
	if cand.IsPoolRoute() {
		b.TargetKind = "credential"
		if req.SelectedCredential != nil {
			b.CredentialID = req.SelectedCredential.ID
		}
	} else {
		b.TargetKind = "deployment"
		b.DeploymentID = cand.DeploymentID
		b.EndpointID = cand.EndpointID
	}
	identity := req.APIKey.KeyID
	if err := s.Sticky.SetBinding(ctx, req.APIKey.TenantID, identity, req.ModelCode, req.ConversationID, &b); err != nil {
		slog.WarnContext(ctx, "sticky write failed",
			"conv_id", req.ConversationID,
			"route_id", cand.RouteID,
			"error", err,
		)
	}
}

// candidateNeedsBodyRebuild reports whether switching from the previous attempt
// to cand requires regenerating the upstream body. The body bytes embed both
// the protocol shape (openai_chat vs anthropic_messages) and the upstream model
// name, so any change to either invalidates the cached body.
func candidateNeedsBodyRebuild(req *Request, cand *domain.RouteCandidate) bool {
	if len(req.Attempts) == 0 {
		return false
	}
	prev := req.Attempts[len(req.Attempts)-1]
	// We don't store protocol/model on AttemptRecord (yet), so be conservative:
	// any new RouteID means a potential change.
	return prev.RouteID != cand.RouteID
}

// lastAttemptWas reports whether the most recent recorded attempt has the
// given status.
func lastAttemptWas(req *Request, status ResultStatus) bool {
	if len(req.Attempts) == 0 {
		return false
	}
	return req.Attempts[len(req.Attempts)-1].Outcome == status
}

// snippetBody returns up to 512 bytes of the upstream response body for error
// reporting. Safe to call with a nil UpstreamResponse.
func snippetBody(resp *UpstreamResponse) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return string(b)
}

// drainAndClose discards any remaining body and closes the response. Safe for
// nil. Must be called before retrying so the underlying connection can be
// reused.
func drainAndClose(resp *UpstreamResponse) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16*1024))
	_ = resp.Body.Close()
}

// ============================================================================
// Non-streaming execution
// ============================================================================

func (s *ExecuteStep) executeSync(ctx context.Context, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32MB max
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}
	req.UpstreamResponseBody = bodyBytes

	bodyBytes = unwrapCodeAssistResponse(req.Candidate, bodyBytes)

	canonResp, err := formats.ToCanonicalChatResponse(bodyBytes, req.Candidate.Protocol, req.RequestID, req.Candidate.UpstreamModel)
	if err != nil {
		return fmt.Errorf("parse upstream response: %w", err)
	}

	// Extract usage for billing
	if canonResp.Usage != nil {
		req.TokenUsage = domain.TokenUsage{
			PromptTokens:     canonResp.Usage.PromptTokens,
			CompletionTokens: canonResp.Usage.CompletionTokens,
			CacheWriteTokens: canonResp.Usage.CacheWriteTokens,
			CacheReadTokens:  canonResp.Usage.CacheReadTokens,
			ReasoningTokens:  canonResp.Usage.ReasoningTokens,
		}
	}
	fillEstimatedUsage(req, len(bodyBytes))

	// Public-facing model code, not upstream's internal name.
	canonResp.Model = req.ModelCode

	relay := RelayFor(req.ClientProtocol)
	if err := relay.WriteSync(w, canonResp); err != nil {
		return fmt.Errorf("relay sync response: %w", err)
	}

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
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

	relay := RelayFor(req.ClientProtocol)
	sink := relay.NewStream(w, flusher.Flush, req.RequestID, req.ModelCode)
	if err := sink.Start(0); err != nil {
		return fmt.Errorf("stream start: %w", err)
	}
	// Mark headers as committed so runtime_pipeline.go skips the error WriteHeader path.
	req.HTTPStatus = http.StatusOK

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

		// Always report the public model code to the client.
		chunk.Model = req.ModelCode

		if err := sink.Push(chunk); err != nil {
			// Client likely disconnected — stop the stream.
			req.RequestStatus = domain.RequestFailed
			req.ErrorCode = "stream_write_error"
			req.ErrorMessage = err.Error()
			return nil
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		req.RequestStatus = domain.RequestFailed
		req.ErrorCode = "stream_read_error"
		req.ErrorMessage = err.Error()
		_ = sink.Error("stream_read_error", err.Error())
		return nil
	}

	_ = sink.Finish()
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
			if req.Envelope != nil && req.Envelope.R != nil {
				incomingBeta = req.Envelope.R.Header.Get("anthropic-beta")
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

