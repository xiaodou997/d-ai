package serving

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/formats"
	"xiaodou/uni-ai-api/internal/formats/claude"
	"xiaodou/uni-ai-api/internal/formats/gemini"
	"xiaodou/uni-ai-api/internal/routing"
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

	upstreamURL := buildURL(cand, req.IsStream)
	slog.InfoContext(parentCtx, "upstream call started",
		"request_id", req.RequestID,
		"upstream_url", upstreamURL,
		"model_code", req.ModelCode,
		"upstream_model", cand.UpstreamModel,
		"provider_code", cand.ProviderCode,
		"is_stream", req.IsStream,
	)
	slog.DebugContext(parentCtx, "upstream request body",
		"request_id", req.RequestID,
		"body", string(body),
	)
	startTime := time.Now()
	upResp, callErr := s.Transport.Do(attemptCtx, &UpstreamRequest{
		Method:   "POST",
		URL:      upstreamURL,
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

	// Pre-read error body once (consumed by DecisionRetry/GiveUp branches below) and
	// emit a single structured error log carrying full upstream context. Without
	// this, transport failures and upstream 4xx/5xx leave NO trace in app logs.
	var errBody string
	if outcome.Status != ResultSuccess {
		if callErr == nil && upResp != nil {
			errBody = snippetBody(upResp)
		}
		logUpstreamFailure(parentCtx, req, cand, upstreamURL, status, latencyMs, callErr, errBody)
	}

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
		drainAndClose(upResp)
		return attemptResult{
			decision: DecisionRetry,
			finalErr: apiError(upstreamStatusToGateway(status), "upstream_error",
				fmt.Sprintf("upstream returned %d: %s", status, truncateValidUTF8(errBody, 512))),
		}

	case DecisionGiveUp:
		drainAndClose(upResp)
		req.RequestStatus = domain.RequestFailed
		req.HTTPStatus = upstreamStatusToGateway(status)
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

// logUpstreamFailure emits a single structured Error log line for any
// non-success upstream attempt — transport errors AND HTTP 4xx/5xx. It is the
// authoritative observability hook for "why did the upstream call fail": it
// carries the URL we hit, route/deployment/provider, status, latency, and
// (when available) the truncated upstream response body. Without this, the
// only trace of an upstream failure is the access log info line, which has
// no upstream context.
func logUpstreamFailure(ctx context.Context, req *Request, cand *domain.RouteCandidate, url string, status, latencyMs int, callErr error, errBody string) {
	attrs := []any{
		"request_id", req.RequestID,
		"upstream_url", url,
		"upstream_status", status,
		"latency_ms", latencyMs,
		"route_id", cand.RouteID,
		"protocol", string(cand.Protocol),
		"model_code", req.ModelCode,
		"upstream_model", cand.UpstreamModel,
		"provider_code", cand.ProviderCode,
		"endpoint_id", cand.EndpointID,
		"deployment_id", cand.DeploymentID,
		"is_stream", req.IsStream,
	}
	if cand.PoolID != "" {
		attrs = append(attrs, "pool_id", cand.PoolID)
	}
	if callErr != nil {
		attrs = append(attrs, "transport_error", callErr.Error())
	}
	if errBody != "" {
		attrs = append(attrs, "upstream_body", truncateValidUTF8(errBody, 1024))
	}
	slog.ErrorContext(ctx, "upstream call failed", attrs...)
}

// snippetBody returns up to 4 KiB of the upstream response body for diagnostics.
// The cap is generous so logs / DB error_message capture meaningful vendor
// payloads; callers that surface this to clients should re-truncate before
// echoing it back.
func snippetBody(resp *UpstreamResponse) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
// Non-streaming execution — strict 1:1 passthrough.
//
// The route candidate is guaranteed (by routes.go SQL filter) to share the
// client's protocol, so the upstream response body is already in the wire
// format the client expects. We extract usage as a side effect for billing
// and forward the bytes verbatim. The only mutation is unwrapping the
// CodeAssist {"response": {...}} envelope for gemini_cli / antigravity so
// the client receives a vanilla Gemini response.
// ============================================================================

func (s *ExecuteStep) executeSync(_ context.Context, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32MB cap
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}
	req.UpstreamResponseBody = bodyBytes

	bodyBytes = unwrapCodeAssistResponse(req.Candidate, bodyBytes)

	// Side-channel: usage for billing. ExtractSyncUsage returns a zero value
	// when upstream did not report tokens; fillEstimatedUsage then falls back
	// to byte-length estimation.
	if u := formats.ExtractSyncUsage(bodyBytes, req.Candidate.Protocol); u.PromptTokens != 0 || u.CompletionTokens != 0 {
		// Preserve any pre-populated image/video billing fields.
		u.ImageCount = req.TokenUsage.ImageCount
		u.ImageResolution = req.TokenUsage.ImageResolution
		u.VideoSeconds = req.TokenUsage.VideoSeconds
		u.VideoResolution = req.TokenUsage.VideoResolution
		req.TokenUsage = u
	}
	fillEstimatedUsage(req, len(bodyBytes))

	// Forward upstream Content-Type when present (Anthropic returns
	// `application/json`; OpenAI/Gemini do likewise). Defaults are safe.
	if ct := resp.Headers.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
	return nil
}

// ============================================================================
// Streaming execution — strict 1:1 line-level passthrough.
//
// Each SSE line from the upstream is forwarded to the client verbatim, with
// two exceptions:
//   - data: lines for gemini_cli / antigravity have the CodeAssist
//     {"response": {...}} envelope stripped before forwarding.
//   - Usage frames are inspected (not modified) to update the running token
//     counters used for billing and first-token latency tracking.
// ============================================================================

func (s *ExecuteStep) executeStream(ctx context.Context, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support streaming")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	req.HTTPStatus = http.StatusOK

	slog.InfoContext(ctx, "stream started",
		"request_id", req.RequestID,
		"model_code", req.ModelCode,
		"upstream_model", req.Candidate.UpstreamModel,
		"provider_code", req.Candidate.ProviderCode,
		"endpoint_id", req.Candidate.EndpointID,
		"route_id", req.Candidate.RouteID,
	)

	protocol := req.Candidate.Protocol
	firstToken := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var eventType string // tracks the most recent SSE `event:` line for Anthropic
	var accumulatedOutputBytes int
	dataPrefix := []byte("data: ")
	eventPrefix := []byte("event: ")
	donePayload := []byte("[DONE]")

	writeLine := func(line []byte) error {
		if _, err := w.Write(line); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		// Take a local copy because scanner.Bytes() is invalidated on the next
		// scan and we use the slice past the write call.
		lineCopy := append([]byte(nil), line...)
		slog.DebugContext(ctx, "upstream sse line", "request_id", req.RequestID, "line", string(lineCopy))

		switch {
		case len(lineCopy) == 0:
			// Blank line is the SSE event terminator — forward verbatim.
			if err := writeLine(lineCopy); err != nil {
				return streamClientWriteError(req, err)
			}
			flusher.Flush()
			continue

		case bytes.HasPrefix(lineCopy, eventPrefix):
			eventType = string(bytes.TrimPrefix(lineCopy, eventPrefix))
			if err := writeLine(lineCopy); err != nil {
				return streamClientWriteError(req, err)
			}
			continue

		case bytes.HasPrefix(lineCopy, dataPrefix):
			data := lineCopy[len(dataPrefix):]

			if bytes.Equal(data, donePayload) {
				if err := writeLine(lineCopy); err != nil {
					return streamClientWriteError(req, err)
				}
				flusher.Flush()
				eventType = ""
				continue
			}

			// Strip the CodeAssist response envelope so the client sees a
			// vanilla Gemini chunk. unwrapCodeAssistResponse is a no-op for
			// non-CodeAssist providers.
			dataUnwrapped := unwrapCodeAssistResponse(req.Candidate, data)
			payloadForClient := dataUnwrapped
			if bytes.Equal(dataUnwrapped, data) {
				// Fast path: nothing changed — forward original framing.
				if err := writeLine(lineCopy); err != nil {
					return streamClientWriteError(req, err)
				}
			} else {
				out := make([]byte, 0, len(dataPrefix)+len(dataUnwrapped))
				out = append(out, dataPrefix...)
				out = append(out, dataUnwrapped...)
				if err := writeLine(out); err != nil {
					return streamClientWriteError(req, err)
				}
			}
			flusher.Flush()

			// First-token latency: any non-empty data payload counts.
			if !firstToken {
				req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
				firstToken = true
			}

			// Side-extract usage. ExtractStreamUsage merges into the running
			// counters; output_tokens are cumulative per provider semantics.
			if u, ok := formats.ExtractStreamUsage(req.TokenUsage, payloadForClient, eventType, protocol); ok {
				u.ImageCount = req.TokenUsage.ImageCount
				u.ImageResolution = req.TokenUsage.ImageResolution
				u.VideoSeconds = req.TokenUsage.VideoSeconds
				u.VideoResolution = req.TokenUsage.VideoResolution
				req.TokenUsage = u
				accumulatedOutputBytes = 0
			} else {
				accumulatedOutputBytes += len(payloadForClient)
			}
			eventType = ""

		default:
			// Comment / unknown SSE line — forward verbatim, clear pending event.
			if err := writeLine(lineCopy); err != nil {
				return streamClientWriteError(req, err)
			}
			eventType = ""
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		req.RequestStatus = domain.RequestFailed
		req.ErrorCode = "stream_read_error"
		req.ErrorMessage = err.Error()
		return nil
	}

	fillEstimatedUsage(req, accumulatedOutputBytes)
	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = http.StatusOK

	slog.InfoContext(ctx, "stream finished",
		"request_id", req.RequestID,
		"model_code", req.ModelCode,
		"upstream_model", req.Candidate.UpstreamModel,
		"provider_code", req.Candidate.ProviderCode,
		"first_token_ms", req.FirstTokenMs,
		"total_ms", int(time.Since(startTime).Milliseconds()),
		"prompt_tokens", req.TokenUsage.PromptTokens,
		"completion_tokens", req.TokenUsage.CompletionTokens,
	)
	return nil
}

// streamClientWriteError marks the request as failed when the client socket
// breaks mid-stream and returns nil so the pipeline does not treat this as a
// pipeline-level error (response is already committed).
func streamClientWriteError(req *Request, err error) error {
	req.RequestStatus = domain.RequestFailed
	req.ErrorCode = "stream_write_error"
	req.ErrorMessage = err.Error()
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// buildUpstreamBody produces the bytes we send to the upstream. The starting
// point is the unmodified client request body (req.Envelope.ClientBody) — we
// only mutate the `model` field (so upstream sees the deployment's
// upstream_model rather than the client's logical model name) and apply
// vendor-specific body transforms required by OAuth fixed providers:
//
//   - Codex: strip max_output_tokens / temperature / top_p, force store=false,
//     default instructions. ApplyCodexRequestModifications operates on raw
//     bytes so client-supplied fields (previous_response_id, reasoning, etc.)
//     are preserved.
//   - Claude OAuth: drop unsigned thinking blocks (SanitizeOAuthRequestBody).
//   - Gemini CLI / Antigravity: wrap the body in CodeAssist envelope.
//
// Strict 1:1 protocol matching means the body's wire format already matches
// the upstream; no canonical round-trip and no cross-protocol translation
// happens here.
func buildUpstreamBody(req *Request) ([]byte, error) {
	if req.Envelope == nil || len(req.Envelope.ClientBody) == 0 {
		return nil, fmt.Errorf("missing client request body")
	}
	body := req.Envelope.ClientBody

	// Model substitution. RewriteModel returns the original slice unchanged
	// when the client already named upstream_model, so zero-cost in the
	// common path.
	body, err := formats.RewriteModel(body, req.Candidate.UpstreamModel)
	if err != nil {
		return nil, err
	}

	// Codex needs strict body shape before OAuth transforms (which it does
	// not have — Codex is a fixed provider in the OpenAI Responses family,
	// not a Claude/Gemini OAuth pool).
	if req.Candidate.FixedProviderType == domain.FixedProviderCodex {
		body, err = formats.ApplyCodexRequestModifications(body)
		if err != nil {
			return nil, err
		}
	}

	return applyOAuthBodyTransform(req, body)
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

	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	path := c.RequestPath
	if path == "" {
		path = defaultPath(c.Protocol)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// 防止 base_url 末尾路径与 path 前缀重复（如 base_url 存了 /v1，defaultPath 也以 /v1/ 开头）
	if u, err := url.Parse(base); err == nil {
		if basePath := strings.TrimRight(u.Path, "/"); basePath != "" {
			if trimmed, ok := strings.CutPrefix(path, basePath+"/"); ok {
				path = "/" + trimmed
			}
		}
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
	case code == 0:
		return http.StatusBadGateway // transport error (timeout, connection refused, etc.)
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

