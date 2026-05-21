package serving

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"xiaodou/uni-ai-api/internal/audit"
	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/egress"
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
// It carries no timeout: the per-attempt deadline lives on the context passed
// to Transporter.Do (see runAttempt's attemptCtx).
type UpstreamRequest struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     []byte
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
	retryDeadline := time.Now().Add(budget.RetryWindow)

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
		if attempt > 1 && time.Now().After(retryDeadline) {
			lastErr = apiError(http.StatusGatewayTimeout, "retry_window_exhausted",
				"retry window exhausted before any route produced a response")
			break
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
				zap.L().Warn("pool credential selection failed",
					zap.String("pool_id", cand.PoolID), zap.Error(selErr))
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

		result := s.runAttempt(ctx, req, cand, body, score)
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

// precommitError marks an upstream attempt that failed AFTER 2xx response
// headers but BEFORE any byte was committed to the client (empty stream,
// 200-with-error-body, first SSE frame was an error, or a first-byte timeout).
// Because nothing was written downstream, the execute loop can still fail over
// to another route.
type precommitError struct {
	cause      error
	httpStatus int // upstream HTTP status (usually 200)
	message    string
}

func (e *precommitError) Error() string { return e.message }
func (e *precommitError) Unwrap() error { return e.cause }

// postcommitError marks a streaming attempt that broke AFTER 200 OK was
// committed to the client (idle timeout, max-duration, mid-stream upstream
// drop). No failover is possible; the client has already been sent a
// protocol-level error frame. It is reported to the breaker as a failure.
type postcommitError struct {
	code    string
	message string
}

func (e *postcommitError) Error() string { return e.message }

// runAttempt executes one upstream call and handles the response. It owns a
// deadlineController (defer stop) whose context is bound to the HTTP request,
// so the connect / first-byte / idle / max-duration timeouts each cancel the
// in-flight call cleanly. The relay finishes BEFORE this function returns.
func (s *ExecuteStep) runAttempt(parentCtx context.Context, req *Request, cand *domain.RouteCandidate, body []byte, score float64) attemptResult {
	dc := newDeadlineController(parentCtx, cand.Timeouts)
	defer dc.stop()

	// P3: track inflight count; always decrement even on error.
	if s.Stats != nil {
		s.Stats.IncrInflight(parentCtx, cand.RouteID)
		defer s.Stats.DecrInflight(parentCtx, cand.RouteID)
	}

	upstreamURL := buildURL(cand, req)
	zap.L().Info("upstream call started",
		zap.String("request_id", req.RequestID),
		zap.String("upstream_url", upstreamURL),
		zap.String("model_code", req.ModelCode),
		zap.String("upstream_model", cand.UpstreamModel),
		zap.String("provider_code", cand.ProviderCode),
		zap.Bool("is_stream", req.IsStream),
	)
	zap.L().Debug("upstream request body",
		zap.String("request_id", req.RequestID),
		zap.String("body", string(body)),
	)
	startTime := time.Now()
	upResp, callErr := s.Transport.Do(dc.ctx, &UpstreamRequest{
		Method:   "POST",
		URL:      upstreamURL,
		Headers:  buildHeaders(cand, req),
		Body:     body,
		Protocol: cand.Protocol,
	})
	defer drainAndClose(upResp)
	latencyMs := int(time.Since(startTime).Milliseconds())
	if callErr == nil {
		dc.headersReceived() // connect phase done → first-byte phase
	} else if cause := dc.cause(); cause != nil {
		// A phase timeout cancelled the request — surface the precise cause so
		// the classifier logs "timeout" rather than a generic transport error.
		callErr = cause
	}

	// P3: record latency for EWMA update (only on completed calls).
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

	// Pre-read error body once (consumed by DecisionRetry/GiveUp branches below)
	// and emit a single structured error log. Health for non-success outcomes
	// is notified here; the 2xx/Accept outcome is notified AFTER relay, once we
	// know whether the response actually committed.
	var errBody string
	if outcome.Status != ResultSuccess {
		if callErr == nil && upResp != nil {
			errBody = snippetBody(upResp)
		}
		logUpstreamFailure(parentCtx, req, cand, upstreamURL, status, latencyMs, callErr, errBody)
		s.notifyHealth(parentCtx, req, cand, outcome)
	}

	switch outcome.Decision(req.SelectedCredential != nil) {
	case DecisionAccept:
		err := s.relay(dc, req, upResp, startTime)
		var pre *precommitError
		switch {
		case err == nil:
			// Downstream write failures are terminal for this request but are
			// not upstream successes; do not reward the route or persist sticky.
			if req.RequestStatus == domain.RequestFailed && req.ErrorCode == "stream_write_error" {
				return attemptResult{finished: true, finalErr: nil}
			}
			// Upstream succeeded AND the response committed cleanly.
			s.notifyHealth(parentCtx, req, cand, outcome)
			s.writeSticky(parentCtx, req, cand)
			return attemptResult{finished: true, finalErr: nil}

		case errors.As(err, &pre):
			// 2xx headers but no usable response. Nothing was written
			// downstream, so fail over — and count it against the breaker so a
			// deployment serving garbage gets isolated.
			drainAndClose(upResp)
			markAttemptFailed(req, pre.message)
			s.notifyHealth(parentCtx, req, cand, Outcome{Status: ResultServerError, HTTPStatus: pre.httpStatus})
			logUpstreamFailure(parentCtx, req, cand, upstreamURL, pre.httpStatus, latencyMs, pre.cause, pre.message)
			req.UsedCandidates[cand.RouteID] = true
			return attemptResult{
				decision: DecisionRetry,
				finalErr: apiError(http.StatusBadGateway, "upstream_error", pre.message),
			}

		default:
			// postcommitError — the stream broke after 200 OK was committed.
			// The client already received a protocol error frame; this is
			// terminal but still a breaker failure for the deployment.
			markAttemptFailed(req, err.Error())
			s.notifyHealth(parentCtx, req, cand, Outcome{Status: ResultServerError, HTTPStatus: http.StatusOK})
			return attemptResult{finished: true, finalErr: nil}
		}

	case DecisionRetryNewCred:
		drainAndClose(upResp)
		if s.OAuthPool != nil && req.SelectedCredential != nil {
			oldCredID := req.SelectedCredential.ID
			zap.L().Warn("upstream 401, swapping credential",
				zap.String("old_cred_id", oldCredID))
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

// isPrecommitError reports whether err is a *precommitError.
func isPrecommitError(err error) bool {
	var pre *precommitError
	return errors.As(err, &pre)
}

// markAttemptFailed flips the most recent AttemptRecord to a failure outcome so
// X-Route-Trace and logs reflect what actually happened (the record is written
// optimistically right after 2xx headers, before the relay result is known).
func markAttemptFailed(req *Request, msg string) {
	if n := len(req.Attempts); n > 0 {
		req.Attempts[n-1].Outcome = ResultServerError
		req.Attempts[n-1].ErrorMsg = msg
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
func (s *ExecuteStep) relay(dc *deadlineController, req *Request, upResp *UpstreamResponse, startTime time.Time) error {
	if req.IsStream {
		return s.executeStream(dc, req, upResp, req.Envelope.W, startTime)
	}
	return s.executeSync(dc, req, upResp, req.Envelope.W)
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
	identity := req.RuntimeIdentity()
	if s.Sticky == nil || req.ConversationID == "" || identity == nil {
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
	if err := s.Sticky.SetBinding(ctx, identity.TenantID, identity.StickyKey(), req.ModelCode, req.ConversationID, &b); err != nil {
		zap.L().Warn("sticky write failed",
			zap.String("conv_id", req.ConversationID),
			zap.String("route_id", cand.RouteID),
			zap.Error(err),
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
	attrs := []zap.Field{
		zap.String("request_id", req.RequestID),
		zap.String("upstream_url", url),
		zap.Int("upstream_status", status),
		zap.Int("latency_ms", latencyMs),
		zap.String("route_id", cand.RouteID),
		zap.String("protocol", string(cand.Protocol)),
		zap.String("model_code", req.ModelCode),
		zap.String("upstream_model", cand.UpstreamModel),
		zap.String("provider_code", cand.ProviderCode),
		zap.String("endpoint_id", cand.EndpointID),
		zap.String("deployment_id", cand.DeploymentID),
		zap.Bool("is_stream", req.IsStream),
	}
	if cand.PoolID != "" {
		attrs = append(attrs, zap.String("pool_id", cand.PoolID))
	}
	if callErr != nil {
		attrs = append(attrs, zap.String("transport_error", callErr.Error()))
	}
	if errBody != "" {
		attrs = append(attrs, zap.String("upstream_body", truncateValidUTF8(errBody, 1024)))
	}
	zap.L().Error("upstream call failed", attrs...)
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

func (s *ExecuteStep) executeSync(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyReader := &syncBodyReader{r: resp.Body, onFirstByte: dc.syncBodyPhase}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, 32*1024*1024)) // 32MB cap
	if err != nil {
		if cause := dc.cause(); cause != nil {
			err = cause
		}
		// Body read failed before anything was written downstream — retryable.
		return &precommitError{
			cause:      err,
			httpStatus: resp.StatusCode,
			message:    "read upstream body: " + err.Error(),
		}
	}
	req.UpstreamResponseBody = bodyBytes

	policy := publicEgressPolicy(req, req.Candidate)
	bodyBytes = unwrapCodeAssistResponse(req.Candidate, bodyBytes)
	bodyBytes = egress.SanitizeJSON(bodyBytes, policy)

	// Extract the assistant reply for the audit log from the original body
	// (before model-name rewrite, but model identity is captured separately).
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(req.UpstreamResponseBody, req.Candidate.Protocol)

	// A 200 whose body is actually an error object is a failed attempt — fail
	// over to another route instead of relaying a broken success.
	if resp.StatusCode == http.StatusOK && payloadIsError(bodyBytes) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(bodyBytes)), 256),
		}
	}

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
	req.ResponseCommitted = true
	_, _ = w.Write(bodyBytes)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
	return nil
}

// syncBodyReader keeps the first-byte timer active until a non-streaming
// response actually yields body bytes, then switches the attempt to the
// max-duration body-read bound.
type syncBodyReader struct {
	r           io.Reader
	onFirstByte func()
	seen        bool
}

func (r *syncBodyReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 && !r.seen {
		r.seen = true
		r.onFirstByte()
	}
	return n, err
}

// ============================================================================
// Streaming execution — delayed-commit, line-level 1:1 passthrough.
//
// Phase 3 (确认): buffer leading event:/comment lines and read until the first
// usable `data:` frame. If the upstream produced nothing usable — empty
// stream, 200-with-error-body, or an error as the first frame — the relay
// returns a *precommitError and the execute loop fails over to another route
// WITHOUT having written anything to the client.
//
// Phase 4 (传输): once the first frame is confirmed the gateway commits 200 OK
// and forwards every subsequent SSE line verbatim (data: lines for
// gemini_cli / antigravity have the CodeAssist envelope stripped; usage frames
// are inspected for billing). A mid-stream failure after this point can no
// longer fail over, so the client is sent a protocol-level error frame.
// ============================================================================

// maxSSELineBytes is a defensive OOM ceiling, NOT a functional cap: a single
// SSE data frame may legitimately inline a multi-megabyte base64 image (4K
// images in particular). It only guards against a pathological upstream that
// never emits a newline.
const maxSSELineBytes = 256 * 1024 * 1024

// maxPreambleBytes caps the leading non-data lines buffered before the first
// data frame — a well-behaved upstream sends only a few small event: lines.
const maxPreambleBytes = 1 << 20

var (
	errStreamNoFlusher   = errors.New("response writer does not support streaming")
	errUpstreamErrorBody = errors.New("upstream returned an error body")
	errUpstreamEmpty     = errors.New("upstream returned an empty stream")
	errSSELineTooLong    = errors.New("sse frame exceeds the 256MiB safety ceiling")
)

func (s *ExecuteStep) executeStream(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}

	protocol := req.Candidate.Protocol
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	dataPrefix := []byte("data: ")
	eventPrefix := []byte("event: ")
	donePayload := []byte("[DONE]")

	// ---- Phase 3 确认：缓冲前导行，读到首个有效 data 帧才提交 ----
	var (
		preamble  []byte
		eventType string
		firstData []byte
		awaitErr  error
	)
awaitLoop:
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		trimmed := trimEOL(line)
		switch {
		case bytes.HasPrefix(trimmed, dataPrefix):
			firstData = line
			awaitErr = readErr
			break awaitLoop
		case bytes.HasPrefix(trimmed, eventPrefix):
			eventType = string(bytes.TrimPrefix(trimmed, eventPrefix))
			preamble = append(preamble, line...)
		default:
			// blank line / comment / non-SSE bytes — buffer it
			preamble = append(preamble, line...)
		}
		if len(preamble) > maxPreambleBytes {
			return &precommitError{
				cause:      errUpstreamErrorBody,
				httpStatus: resp.StatusCode,
				message:    "upstream sent an oversized preamble without a data frame",
			}
		}
		if readErr != nil {
			awaitErr = readErr
			break awaitLoop
		}
	}

	// No data frame at all — the upstream produced nothing usable. Fail over.
	if firstData == nil {
		if cause := dc.cause(); cause != nil {
			awaitErr = cause
		}
		return precommitFromNoFrame(resp.StatusCode, preamble, awaitErr)
	}

	// The first data frame itself is an error event. Fail over.
	firstPayload := trimEOL(firstData)[len(dataPrefix):]
	if !bytes.Equal(firstPayload, donePayload) && (eventType == "error" || payloadIsError(firstPayload)) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: resp.StatusCode,
			message:    "upstream streamed an error before any content: " + truncateValidUTF8(string(firstPayload), 256),
		}
	}

	// ---- commit：从这里起响应不可逆 ----
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	req.ResponseCommitted = true
	req.HTTPStatus = http.StatusOK
	dc.firstByte()
	req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
	if n := len(req.Attempts); n > 0 {
		req.Attempts[n-1].FirstByteMs = req.FirstTokenMs
	}
	zap.L().Info("stream started",
		zap.String("request_id", req.RequestID),
		zap.String("model_code", req.ModelCode),
		zap.String("upstream_model", req.Candidate.UpstreamModel),
		zap.String("provider_code", req.Candidate.ProviderCode),
		zap.String("route_id", req.Candidate.RouteID),
		zap.Int("first_byte_ms", req.FirstTokenMs),
	)

	accumulatedOutputBytes := 0
	publicSanitizer := egress.NewSanitizer(publicEgressPolicy(req, req.Candidate))
	auditAcc := audit.NewResponseAccumulator(req.Candidate.Protocol)

	// forward writes one already-framed SSE line to the client, stripping the
	// CodeAssist envelope on data lines, rewriting the model name to the
	// public-facing model code, and side-extracting usage for billing.
	forward := func(line []byte, evt string) error {
		trimmed := trimEOL(line)
		if !bytes.HasPrefix(trimmed, dataPrefix) {
			_, err := w.Write(line) // blank line / comment — verbatim
			return err
		}
		data := trimmed[len(dataPrefix):]
		if bytes.Equal(data, donePayload) {
			_, err := w.Write(line)
			return err
		}
		unwrapped := unwrapCodeAssistResponse(req.Candidate, data)
		finalData := publicSanitizer.SanitizeSSEData(unwrapped)
		auditAcc.AddChunk(finalData)
		if bytes.Equal(finalData, data) {
			if _, err := w.Write(line); err != nil {
				return err
			}
		} else {
			out := append(append([]byte(nil), dataPrefix...), finalData...)
			out = append(out, '\n')
			if _, err := w.Write(out); err != nil {
				return err
			}
		}
		if u, ok := formats.ExtractStreamUsage(req.TokenUsage, finalData, evt, protocol); ok {
			u.ImageCount = req.TokenUsage.ImageCount
			u.ImageResolution = req.TokenUsage.ImageResolution
			u.VideoSeconds = req.TokenUsage.VideoSeconds
			u.VideoResolution = req.TokenUsage.VideoResolution
			req.TokenUsage = u
			accumulatedOutputBytes = 0
		} else {
			accumulatedOutputBytes += len(finalData)
		}
		return nil
	}

	// Forward the buffered preamble + the confirmed first data frame.
	if len(preamble) > 0 {
		if _, err := w.Write(preamble); err != nil {
			return streamClientWriteError(req, err)
		}
	}
	if err := forward(firstData, eventType); err != nil {
		return streamClientWriteError(req, err)
	}
	flusher.Flush()
	eventType = ""

	if awaitErr != nil {
		req.AuditResponseMessage = auditAcc.Build()
		return finishStream(req, startTime, accumulatedOutputBytes, awaitErr, dc, w, flusher)
	}

	// ---- Phase 4 传输 ----
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		if len(line) > 0 {
			dc.chunkReceived()
			trimmed := trimEOL(line)
			if bytes.HasPrefix(trimmed, eventPrefix) {
				eventType = string(bytes.TrimPrefix(trimmed, eventPrefix))
				if _, err := w.Write(line); err != nil {
					return streamClientWriteError(req, err)
				}
			} else {
				if err := forward(line, eventType); err != nil {
					return streamClientWriteError(req, err)
				}
				flusher.Flush()
				eventType = ""
			}
		}
		if readErr != nil {
			req.AuditResponseMessage = auditAcc.Build()
			return finishStream(req, startTime, accumulatedOutputBytes, readErr, dc, w, flusher)
		}
	}
}

// finishStream handles the end of a committed stream. io.EOF is a clean
// finish; anything else is a post-commit failure that earns a protocol-shaped
// error frame so the client is not left with a silent truncation.
func finishStream(req *Request, startTime time.Time, accumulatedOutputBytes int, readErr error, dc *deadlineController, w http.ResponseWriter, flusher http.Flusher) error {
	fillEstimatedUsage(req, accumulatedOutputBytes)

	if readErr == io.EOF {
		req.RequestStatus = domain.RequestSuccess
		req.HTTPStatus = http.StatusOK
		zap.L().Info("stream finished",
			zap.String("request_id", req.RequestID),
			zap.String("model_code", req.ModelCode),
			zap.Int("first_token_ms", req.FirstTokenMs),
			zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
			zap.Int("prompt_tokens", req.TokenUsage.PromptTokens),
			zap.Int("completion_tokens", req.TokenUsage.CompletionTokens),
		)
		return nil
	}

	// Post-commit failure — 200 OK is already sent and the body is partial.
	code, msg := streamFailureReason(dc, readErr)
	msg = egress.SanitizeText(msg, PublicEgressPolicy(req))
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = http.StatusOK
	req.ErrorCode = code
	req.ErrorMessage = msg
	if _, werr := w.Write(formats.StreamErrorFrame(req.ClientProtocol, code, msg)); werr == nil {
		flusher.Flush()
	}
	zap.L().Warn("stream aborted after commit",
		zap.String("request_id", req.RequestID),
		zap.String("error_code", code),
		zap.String("error", msg),
		zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
	)
	return &postcommitError{code: code, message: msg}
}

// streamFailureReason maps a post-commit read failure to an error code/message.
func streamFailureReason(dc *deadlineController, readErr error) (string, string) {
	switch dc.cause() {
	case ErrIdleTimeout:
		return "stream_idle_timeout", "upstream stream stalled and was terminated by the gateway"
	case ErrMaxDuration:
		return "stream_max_duration", "upstream stream exceeded the maximum allowed duration"
	}
	if errors.Is(readErr, errSSELineTooLong) {
		return "stream_frame_too_large", "an upstream stream frame exceeded the gateway size limit"
	}
	return "stream_read_error", "the gateway lost the connection to the upstream mid-stream: " + readErr.Error()
}

// precommitFromNoFrame builds a precommitError for an upstream stream that
// ended before producing a single data frame.
func precommitFromNoFrame(status int, preamble []byte, readErr error) *precommitError {
	if payloadIsError(preamble) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: status,
			message:    "upstream returned a non-SSE error body: " + truncateValidUTF8(string(bytes.TrimSpace(preamble)), 256),
		}
	}
	cause := errUpstreamEmpty
	if readErr != nil && readErr != io.EOF {
		cause = readErr
	}
	return &precommitError{
		cause:      cause,
		httpStatus: status,
		message:    "upstream produced no stream content: " + cause.Error(),
	}
}

// readSSELine reads one line including its trailing '\n'. Unlike bufio.Scanner
// it has no fixed line-length limit (an SSE data frame may inline a
// multi-megabyte image); maxBytes is only the defensive OOM ceiling.
func readSSELine(r *bufio.Reader, maxBytes int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(line)+len(chunk) > maxBytes {
			return line, errSSELineTooLong
		}
		line = append(line, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, err
	}
}

// trimEOL strips a trailing \n or \r\n.
func trimEOL(line []byte) []byte {
	return bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
}

// payloadIsError reports whether a JSON payload is an error object. It
// recognises the OpenAI/Gemini `{"error":{...}}` shape and the Anthropic
// `{"type":"error",...}` shape.
func payloadIsError(data []byte) bool {
	t := bytes.TrimSpace(data)
	if len(t) == 0 || t[0] != '{' {
		return false
	}
	var probe struct {
		Error json.RawMessage `json:"error"`
		Type  string          `json:"type"`
	}
	if json.Unmarshal(t, &probe) != nil {
		return false
	}
	return len(probe.Error) > 0 || probe.Type == "error"
}

// streamClientWriteError marks the request as failed when the client socket
// breaks mid-stream and returns nil so the pipeline does not treat this as a
// pipeline-level error (the response is already committed, and the upstream
// itself was healthy).
func streamClientWriteError(req *Request, err error) error {
	req.RequestStatus = domain.RequestFailed
	req.ErrorCode = "stream_write_error"
	req.ErrorMessage = err.Error()
	return nil
}

// PublicEgressPolicy returns the route-derived public egress policy for this
// request. The policy is intentionally built from routing metadata, not from
// upstream response bytes.
func PublicEgressPolicy(req *Request) egress.Policy {
	if req == nil {
		return egress.Policy{}
	}
	return publicEgressPolicy(req, req.Candidate)
}

func publicEgressPolicy(req *Request, cand *domain.RouteCandidate) egress.Policy {
	if req == nil || cand == nil {
		return egress.Policy{}
	}
	aliases := make([]string, 0, 1)
	if cand.PoolUpstreamModel != "" && cand.PoolUpstreamModel != cand.UpstreamModel {
		aliases = append(aliases, cand.PoolUpstreamModel)
	}
	return egress.Policy{
		PublicModel:        req.ModelCode,
		UpstreamModel:      cand.UpstreamModel,
		Protocol:           cand.Protocol,
		ProviderCode:       cand.ProviderCode,
		EndpointBaseURL:    cand.BaseURL,
		Aliases:            aliases,
		AllowVersionSuffix: true,
	}
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
	contentType := ""
	if req.Envelope != nil && req.Envelope.R != nil {
		contentType = req.Envelope.R.Header.Get("Content-Type")
	}
	body, err := formats.RewriteModel(body, req.Candidate.UpstreamModel, contentType)
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

func buildURL(c *domain.RouteCandidate, req *Request) string {
	isStream := false
	clientPath := ""
	if req != nil {
		isStream = req.IsStream
		clientPath = req.ClientPath
	}
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
		path = defaultPath(c.Protocol, clientPath)
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

func defaultPath(protocol domain.UpstreamProtocol, clientPath string) string {
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
		if strings.Contains(clientPath, "/images/edits") {
			return "/v1/images/edits"
		}
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
	if req != nil && req.Envelope != nil && req.Envelope.R != nil {
		if ct := req.Envelope.R.Header.Get("Content-Type"); ct != "" {
			headers["Content-Type"] = ct
		}
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
			// Stable session/conversation IDs derived from caller identity + pool.
			identityKey := "web:" + req.RequestID
			if identity := req.RuntimeIdentity(); identity != nil {
				identityKey = identity.StickyKey()
			}
			sessionID := codexSessionID(identityKey, c.PoolID)
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
