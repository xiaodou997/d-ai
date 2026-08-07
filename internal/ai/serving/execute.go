package serving

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/internal/ai/clientruntime"
	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/egress"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/routing"
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
}

type OAuthCredentialCooldownRecorder interface {
	MarkCooldown(ctx context.Context, credID string, until time.Time) error
}

// PinnedCredentialSelector is the optional half of OAuthCredentialPool that
// resolves one specific credential inside a pool. Sticky routing needs it:
// binding a conversation to a route is meaningless for pool routes unless the
// same physical credential (= upstream account) is reused on every turn.
type PinnedCredentialSelector interface {
	// SelectPinnedCredential returns the credential when it still belongs to
	// the pool and is active; any other case must return an error so the
	// caller can fall back to normal selection.
	SelectPinnedCredential(ctx context.Context, poolID, credID string) (*domain.OAuthCredential, error)
}

// DirectAccountState records permanent credential rejection for API-key based
// upstream accounts. It is intentionally separate from transient health state.
type DirectAccountState interface {
	MarkAccountInvalid(ctx context.Context, accountID, reason string) (domain.UpstreamAccount, error)
}

// ProtocolBridge encapsulates execute-path cross-surface conversion so
// ExecuteStep does not assemble bridge envelopes itself.
type ProtocolBridge interface {
	PrepareRequest(req *Request, body []byte) (corebridge.PreparedRequest, error)
	BridgeRequest(req *Request, body []byte) ([]byte, error)
	BridgeResponse(req *Request, body []byte) ([]byte, error)
	BridgeImageStream(req *Request, rawBody []byte) (corebridge.ImageStreamResult, error)
	// AggregateImageProviderBody collapses a (possibly SSE) upstream image
	// response into a single provider-format JSON body.
	AggregateImageProviderBody(req *Request, rawBody []byte) ([]byte, error)
	// BuildImageClientStream wraps a client-format image JSON body into the
	// client's SSE stream shape (one completed frame per image).
	BuildImageClientStream(req *Request, clientBody []byte) ([]byte, error)
	BuildUpstreamRequest(req *Request, prepared corebridge.PreparedRequest) (*UpstreamRequest, error)
	NewProvider(req *Request) (corebridge.StreamProvider, error)
	NewEmitter(req *Request) (corebridge.StreamEmitter, error)
	ExtractSyncUsage(req *Request, body []byte) domain.TokenUsage
	ExtractStreamUsage(req *Request, prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool)
	NormalizeResponseBody(req *Request, body []byte) []byte
	StreamErrorFrame(req *Request, code, msg string) []byte
}

// ImageResponseNormalizer restores the response representation requested by
// an OpenAI Images caller after the upstream response has been converted to
// the client protocol. Implementations own image validation, URL fetching and
// short-lived asset materialization.
type ImageResponseNormalizer interface {
	NormalizeImageResponse(ctx context.Context, clientBody []byte, responseFormat string) ([]byte, error)
}

// ExecuteStep is the upstream call + retry loop. It pulls candidates from
// req.Candidates, attempts them in order subject to RetryBudget, classifies
// each outcome, and either retries with a different route, swaps the OAuth
// credential, gives up (4xx), or relays the successful response to the client
// via the protocol-appropriate Relay.
type ExecuteStep struct {
	Transport       Transporter
	ClientRuntime   clientruntime.Invoker      // fixed OAuth providers; nil keeps the legacy path
	UpstreamLimiter UpstreamConcurrencyLimiter // optional unless a direct account caps concurrency
	Bridge          ProtocolBridge             // required for cross-surface request/response conversion
	Health          routing.HealthTracker      // optional; nil = no circuit breaking
	OAuthPool       OAuthCredentialPool        // optional; enables rejected-credential swaps
	AccountState    DirectAccountState         // optional; persists direct-account credential rejection
	Budget          RetryBudget                // zero value falls back to DefaultRetryBudget
	Scorer          RouteScorer                // optional; nil = first unused candidate (P1 behaviour)
	Stats           routing.RouteStatsStore    // optional; used for inflight tracking alongside scorer
	Sticky          stickyWriter               // optional; writes/deletes sticky binding on success/failure
	ImageNormalizer ImageResponseNormalizer    // optional; normalizes image URL/Base64 response mismatches
}

// Transporter makes the actual HTTP call to an upstream provider.
type Transporter interface {
	Do(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error)
}

// UpstreamConcurrencyLimiter caps simultaneous in-flight requests per direct
// upstream account. Acquire returns a nil slot when the account is unlimited;
// a returned slot must be released once the attempt stops occupying the
// upstream.
type UpstreamConcurrencyLimiter interface {
	Acquire(ctx context.Context, accountID, requestID string, limit int, ttl time.Duration) (UpstreamSlot, error)
}

// UpstreamSlot is one claimed concurrency slot on an upstream account.
type UpstreamSlot interface {
	Release(ctx context.Context)
}

var (
	ErrUpstreamConcurrencyExceeded           = errors.New("upstream concurrency exceeded")
	ErrUpstreamConcurrencyLimiterUnavailable = errors.New("upstream concurrency limiter unavailable")
)

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
	if s == nil || s.Transport == nil || s.Bridge == nil {
		return apiError(http.StatusInternalServerError, "runtime_not_configured", "runtime execution is not fully configured")
	}
	if len(req.Candidates) == 0 {
		return apiError(http.StatusServiceUnavailable, "no_available_route", "no route candidates")
	}
	if req.Envelope == nil || req.Envelope.W == nil {
		return apiError(http.StatusInternalServerError, "missing_envelope", "request envelope not set")
	}

	budget := s.Budget.ApplyRequestFloor(req)
	executionCtx, cancelExecution := context.WithTimeoutCause(ctx, budget.MaxElapsed, ErrRetryDeadlineExceeded)
	defer cancelExecution()

	// The upstream body is (re)built per picked candidate: its bytes embed both
	// the provider wire format (client→provider conversion) and the upstream
	// model name, so it must track the actually-selected route — including the
	// very first attempt, whose pick may differ from candidates[0].
	var (
		prepared     corebridge.PreparedRequest
		bodyBuiltFor string
		lastErr      error
	)
	for attempt := 1; attempt <= budget.MaxAttempts; attempt++ {
		if err := requestContextError(ctx, executionCtx, req); err != nil {
			return err
		}
		cand, score := s.pickCandidate(executionCtx, req)
		if cand == nil {
			break // exhausted all candidates
		}
		req.SetCandidate(cand)

		// Pool routes: select a fresh credential per attempt so auth-swap and
		// new-route paths both get a clean credential.
		if cand.IsPoolRoute() && s.OAuthPool != nil && req.SelectedCredential == nil {
			cred, selErr := s.selectPoolCredential(executionCtx, req, cand)
			if selErr != nil {
				zap.L().Warn("pool credential selection failed",
					requestLogFields(req, zap.String("pool_id", cand.PoolID), zap.Error(selErr))...,
				)
				exhaustPhysicalTarget(req, cand)
				lastErr = apiErrorWithCause(http.StatusServiceUnavailable, "no_credential",
					"no active credential is available for this upstream pool", selErr)
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
				case <-executionCtx.Done():
					return requestContextError(ctx, executionCtx, req)
				}
			}
		}

		// Build (or rebuild) the upstream body for the selected route. The body
		// embeds the provider wire format and upstream model, so any change of
		// route — including the first pick when it isn't candidates[0] — requires
		// a fresh build (e.g. openai_chat client → anthropic_messages provider).
		if len(prepared.Body) == 0 || cand.RouteID != bodyBuiltFor {
			newPrepared, berr := s.prepareBody(req)
			if berr != nil {
				zap.L().Warn("upstream request preparation failed", requestLogFields(req, zap.Error(berr))...)
				exhaustPhysicalTarget(req, cand)
				req.SelectedCredential = nil
				lastErr = apiErrorWithCause(http.StatusBadGateway, "upstream_request_build_failed",
					"unable to prepare a request for the selected upstream", berr)
				continue
			}
			prepared = newPrepared
			bodyBuiltFor = cand.RouteID
			req.UpstreamBodySize = len(prepared.Body)
		}

		var upReq *UpstreamRequest
		if !s.usesClientRuntime(cand) {
			var buildErr error
			upReq, buildErr = s.Bridge.BuildUpstreamRequest(req, prepared)
			if buildErr != nil {
				zap.L().Warn("upstream request construction failed", requestLogFields(req, zap.Error(buildErr))...)
				exhaustPhysicalTarget(req, cand)
				req.SelectedCredential = nil
				lastErr = apiErrorWithCause(http.StatusBadGateway, "upstream_request_build_failed",
					"unable to construct a request for the selected upstream", buildErr)
				continue
			}
		}
		// IsBlocked atomically claims a HALF_OPEN probe. Keep it immediately
		// before Transport.Do so local preparation failures cannot strand the slot.
		if s.candidateBlocked(cand) {
			exhaustPhysicalTarget(req, cand)
			req.SelectedCredential = nil
			continue
		}
		// Claim a concurrency slot for the account. It is released inside
		// runAttempt, which returns only after the relay has finished, so the
		// slot is held for exactly as long as the attempt occupies the upstream.
		slot, err := s.acquireUpstreamSlot(executionCtx, req, cand)
		if err != nil {
			s.releaseHealthProbe(cand)
			if errors.Is(err, ErrUpstreamConcurrencyExceeded) {
				exhaustPhysicalTarget(req, cand)
				req.SelectedCredential = nil
				lastErr = apiErrorWithCause(http.StatusTooManyRequests, "upstream_capacity_exhausted",
					"all eligible upstream capacity is currently exhausted", err)
				continue
			}
			return apiErrorWithCause(http.StatusServiceUnavailable, "upstream_concurrency_limiter_unavailable",
				"unable to verify upstream account capacity", err)
		}

		result := s.runAttempt(executionCtx, req, cand, prepared, upReq, score, slot)
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
			exhaustPhysicalTarget(req, cand)
			req.SelectedCredential = nil
		}
	}

	// Budget exhausted or candidates depleted. Keep these outcomes distinct:
	// an open circuit is an availability condition, exhausting every route is an
	// upstream failure, and hitting the hard cap is request amplification control.
	req.RequestStatus = domain.RequestFailed
	if len(req.Attempts) == 0 {
		if lastErr != nil {
			var apiErr *APIError
			if errors.As(lastErr, &apiErr) {
				req.HTTPStatus = apiErr.Status
				req.ErrorCode = apiErr.Code
				req.ErrorMessage = apiErr.Message
			}
			return lastErr
		}
		req.ErrorCode = "no_healthy_route"
		req.ErrorMessage = "all upstream routes are temporarily unavailable"
		return apiError(http.StatusServiceUnavailable, req.ErrorCode, req.ErrorMessage)
	}
	if len(req.Attempts) >= budget.MaxAttempts && hasUnusedCandidate(req) {
		req.ErrorCode = "retry_budget_exhausted"
		req.ErrorMessage = fmt.Sprintf("upstream attempt limit reached after %d attempts", len(req.Attempts))
		return apiError(http.StatusBadGateway, req.ErrorCode, req.ErrorMessage)
	}
	req.ErrorCode = "all_routes_failed"
	req.ErrorMessage = fmt.Sprintf("all %d available upstream routes failed", len(req.Attempts))
	status := http.StatusBadGateway
	var lastAPIError *APIError
	if errors.As(lastErr, &lastAPIError) {
		status = lastAPIError.Status
	}
	return apiError(status, req.ErrorCode, req.ErrorMessage)
}

func (s *ExecuteStep) usesClientRuntime(candidate *domain.RouteCandidate) bool {
	return s != nil &&
		s.ClientRuntime != nil &&
		candidate != nil &&
		candidate.IsPoolRoute() &&
		s.ClientRuntime.SupportsInvocation(candidate.FixedProviderType, candidate.Protocol)
}

func buildClientInvocation(req *Request, candidate *domain.RouteCandidate, prepared corebridge.PreparedRequest, contentType string) clientruntime.Invocation {
	operation := clientruntime.OperationGenerateContent
	switch candidate.FixedProviderType {
	case domain.FixedProviderCodex:
		operation = clientruntime.OperationResponses
		if req != nil && strings.Contains(req.ClientPath, "/responses/compact") {
			operation = clientruntime.OperationCompact
		}
	case domain.FixedProviderClaudeOAuth:
		operation = clientruntime.OperationMessages
	}
	affinityKey := ""
	if req != nil {
		affinityKey = req.ConversationID
	}
	invocation := clientruntime.Invocation{
		Provider:    candidate.FixedProviderType,
		Operation:   operation,
		Protocol:    candidate.Protocol,
		Model:       candidate.EffectiveUpstreamModel(),
		Body:        prepared.Body,
		ContentType: contentType,
		Stream:      req != nil && req.UpstreamStream(),
		AffinityKey: affinityKey,
	}
	if req != nil {
		invocation.RequestID = req.RequestID
		invocation.Credential = clientruntime.SnapshotCredential(req.SelectedCredential)
		if req.Envelope != nil && req.Envelope.R != nil {
			invocation.IncomingAnthropicBeta = req.Envelope.R.Header.Get("anthropic-beta")
		}
	}
	return invocation
}

// acquireUpstreamSlot claims one concurrency slot on a direct upstream account.
// Pool routes carry no EndpointID and are not capped here.
func (s *ExecuteStep) acquireUpstreamSlot(ctx context.Context, req *Request, candidate *domain.RouteCandidate) (UpstreamSlot, error) {
	if candidate == nil || candidate.EndpointID == "" || candidate.UpstreamConcurrencyLimit == nil || *candidate.UpstreamConcurrencyLimit <= 0 {
		return nil, nil
	}
	if s.UpstreamLimiter == nil {
		return nil, ErrUpstreamConcurrencyLimiterUnavailable
	}
	requestID := ""
	if req != nil {
		requestID = req.RequestID
	}
	return s.UpstreamLimiter.Acquire(ctx, candidate.EndpointID, requestID, *candidate.UpstreamConcurrencyLimit, upstreamSlotLeaseTTL(candidate))
}

// upstreamSlotLeaseTTL bounds how long a slot can be stranded if the process
// dies before releasing it. The normal path releases via defer, so this only
// has to outlast one attempt: the attempt's own duration cap plus a margin.
func upstreamSlotLeaseTTL(candidate *domain.RouteCandidate) time.Duration {
	ttl := time.Duration(0)
	if candidate != nil {
		ttl = candidate.Timeouts.MaxDuration
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return ttl + time.Minute
}

func requestContextError(parentCtx, executionCtx context.Context, req *Request) error {
	if executionCtx == nil || executionCtx.Err() == nil {
		return nil
	}
	if parentCtx == nil || parentCtx.Err() == nil {
		if errors.Is(context.Cause(executionCtx), ErrRetryDeadlineExceeded) {
			req.RequestStatus = domain.RequestFailed
			req.ErrorCode = "retry_deadline_exhausted"
			req.ErrorMessage = "the total upstream retry deadline was exhausted"
			return apiError(http.StatusGatewayTimeout, req.ErrorCode, req.ErrorMessage)
		}
	}
	req.RequestStatus = domain.RequestFailed
	req.ErrorCode = "client_disconnected"
	req.ErrorMessage = "request context ended before execution completed"
	return apiError(http.StatusGatewayTimeout, req.ErrorCode, req.ErrorMessage)
}

func hasUnusedCandidate(req *Request) bool {
	if req == nil {
		return false
	}
	for _, candidate := range req.Candidates {
		if candidate != nil && !req.UsedCandidates[candidate.RouteID] {
			return true
		}
	}
	return false
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
// so response-header / first-byte / idle / max-duration timeouts each cancel the
// in-flight call cleanly. The relay finishes BEFORE this function returns.
//
// slot is the caller-acquired upstream concurrency slot (nil when the account is
// unlimited). Releasing it here — rather than in the caller — keeps the slot
// held across the relay and returns it on every exit path, including the ones
// that abandon this route and fail over to another account.
func (s *ExecuteStep) runAttempt(parentCtx context.Context, req *Request, cand *domain.RouteCandidate, prepared corebridge.PreparedRequest, upReq *UpstreamRequest, score float64, slot UpstreamSlot) attemptResult {
	if slot != nil {
		defer slot.Release(parentCtx)
	}
	dc := newDeadlineController(parentCtx, cand.Timeouts)
	defer dc.stop()
	attemptStartedAt := time.Now()
	req.MarkFirstAttemptStarted(attemptStartedAt)

	// P3: track inflight count; always decrement even on error.
	if s.Stats != nil {
		s.Stats.IncrInflight(parentCtx, cand.RouteID)
		defer s.Stats.DecrInflight(parentCtx, cand.RouteID)
	}

	upstreamURL := cand.BaseURL
	upstreamContentType := prepared.ContentType
	if upReq != nil {
		upstreamURL = upReq.URL
		upstreamContentType = upReq.Headers["Content-Type"]
	}
	if upstreamContentType == "" && req.Envelope != nil && req.Envelope.R != nil {
		upstreamContentType = req.Envelope.R.Header.Get("Content-Type")
	}
	upstreamRequestSummary := audit.SummarizeRequestBody(prepared.Body, upstreamContentType)
	startFields := []zap.Field{
		zap.String("upstream_url", upstreamURL),
		zap.String("model_code", req.ModelCode),
		zap.String("upstream_model", cand.UpstreamModel),
		zap.String("provider_code", cand.ProviderCode),
		zap.Bool("is_stream", req.IsStream),
	}
	startFields = append(startFields, timeoutLogFields(cand.Timeouts)...)
	startFields = append(requestLogFields(req), startFields...)
	zap.L().Info("upstream call started", startFields...)
	zap.L().Debug("upstream request body",
		requestLogFields(req, zap.String("body", upstreamRequestSummary))...,
	)
	transportStartedAt := time.Now()
	var (
		upResp       *UpstreamResponse
		callErr      error
		runtimeTrace *clientruntime.Trace
	)
	if s.usesClientRuntime(cand) {
		exchange, invokeErr := s.ClientRuntime.Invoke(dc.ctx, buildClientInvocation(req, cand, prepared, upstreamContentType))
		callErr = invokeErr
		if exchange != nil {
			trace := exchange.Trace
			runtimeTrace = &trace
			if trace.RequestURL != "" {
				upstreamURL = trace.RequestURL
			}
			if exchange.Response != nil {
				upResp = &UpstreamResponse{
					StatusCode: exchange.Response.StatusCode,
					Headers:    exchange.Response.Headers,
					Body:       exchange.Response.Body,
				}
			}
		}
	} else {
		upResp, callErr = s.Transport.Do(dc.ctx, upReq)
	}
	defer drainAndClose(upResp)
	latencyMs := int(time.Since(transportStartedAt).Milliseconds())
	phaseCause := dc.cause()
	if callErr == nil {
		dc.headersReceived() // response-header phase done → first-byte phase
	} else if phaseCause != nil {
		// A phase timeout cancelled the request — surface the precise cause so
		// the classifier logs "timeout" rather than a generic transport error.
		callErr = phaseCause
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
	if callErr != nil && phaseCause == nil && parentCtx.Err() != nil {
		outcome = Outcome{Status: ResultCanceled, Err: parentCtx.Err()}
	}
	req.UpstreamStatus = status
	req.LatencyMs = latencyMs
	s.recordAttempt(req, cand, outcome, attemptStartedAt, transportStartedAt, latencyMs, score)
	attemptIndex := len(req.Attempts) - 1
	if runtimeTrace != nil {
		req.Attempts[attemptIndex].ProfileRevision = runtimeTrace.ProfileRevision
	}
	defer func() {
		req.CompleteAttempt(attemptIndex, time.Now())
	}()
	if outcome.Status == ResultCanceled {
		s.releaseHealthProbe(cand)
		req.RequestStatus = domain.RequestFailed
		if errors.Is(context.Cause(parentCtx), ErrRetryDeadlineExceeded) {
			req.ErrorCode = "retry_deadline_exhausted"
			req.ErrorMessage = "the total upstream retry deadline was exhausted"
			return attemptResult{
				finished: true,
				finalErr: apiError(http.StatusGatewayTimeout, req.ErrorCode, req.ErrorMessage),
			}
		}
		req.ErrorCode = "client_disconnected"
		req.ErrorMessage = "request context ended before the upstream call completed"
		return attemptResult{
			finished: true,
			finalErr: apiError(http.StatusGatewayTimeout, req.ErrorCode, req.ErrorMessage),
		}
	}

	// Pre-read error body once (consumed by DecisionRetry/GiveUp branches below)
	// and emit a single structured error log. Health for non-success outcomes
	// is notified here; the 2xx/Accept outcome is notified AFTER relay, once we
	// know whether the response actually committed.
	var errBody string
	if outcome.Status != ResultSuccess {
		if callErr == nil && upResp != nil {
			errBody = snippetBody(upResp)
		}
		logUpstreamFailure(parentCtx, req, cand, upstreamURL, status, latencyMs, callErr, errBody, upstreamContentType, upstreamRequestSummary)
		s.notifyHealth(parentCtx, req, cand, outcome)
	}
	if req.CapabilityType == domain.CapabilityImage && outcome.Status == ResultTimeout {
		// A timed-out image request may already be queued and running upstream.
		// Retrying another route can create duplicate billable generations.
		drainAndClose(upResp)
		return finishAmbiguousImageTimeout(req, "image generation timed out while the upstream may still be processing it")
	}

	decision := outcome.Decision(req.SelectedCredential != nil)
	if runtimeTrace != nil &&
		runtimeTrace.CredentialEffect == clientruntime.CredentialEffectCooldown &&
		req.SelectedCredential != nil {
		decision = DecisionRetryNewCred
	}
	switch decision {
	case DecisionAccept:
		err := s.relay(dc, req, upResp, transportStartedAt)
		if err != nil && errors.Is(context.Cause(parentCtx), ErrRetryDeadlineExceeded) {
			s.releaseHealthProbe(cand)
			markAttemptFailed(req, "total upstream retry deadline exceeded")
			if req.ResponseCommitted {
				return attemptResult{finished: true, finalErr: nil}
			}
			req.RequestStatus = domain.RequestFailed
			req.ErrorCode = "retry_deadline_exhausted"
			req.ErrorMessage = "the total upstream retry deadline was exhausted"
			return attemptResult{finished: true, finalErr: apiError(http.StatusGatewayTimeout, req.ErrorCode, req.ErrorMessage)}
		}
		var pre *precommitError
		switch {
		case err == nil:
			// Downstream write failures are terminal for this request but are
			// not upstream successes; do not reward the route or persist sticky.
			if req.RequestStatus == domain.RequestFailed && req.ErrorCode == "stream_write_error" {
				s.releaseHealthProbe(cand)
				return attemptResult{finished: true, finalErr: nil}
			}
			// Upstream succeeded AND the response committed cleanly.
			s.notifyHealth(parentCtx, req, cand, outcome)
			s.writeSticky(parentCtx, req, cand)
			return attemptResult{finished: true, finalErr: nil}

		case errors.As(err, &pre):
			// 2xx headers but no usable response. Count it against the breaker;
			// non-image requests can fail over because nothing was committed.
			drainAndClose(upResp)
			markAttemptFailed(req, pre.message)
			s.notifyHealth(parentCtx, req, cand, Outcome{Status: ResultServerError, HTTPStatus: pre.httpStatus})
			logUpstreamFailure(parentCtx, req, cand, upstreamURL, pre.httpStatus, latencyMs, pre.cause, pre.message, upstreamContentType, upstreamRequestSummary)
			if req.CapabilityType == domain.CapabilityImage && isAmbiguousImageTimeout(pre.cause) {
				return finishAmbiguousImageTimeout(req, "image generation timed out while the upstream may still be processing it")
			}
			exhaustPhysicalTarget(req, cand)
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
			zap.L().Warn("upstream rejected credential, swapping credential",
				requestLogFields(req, zap.String("old_cred_id", oldCredID))...,
			)
			shouldInvalidate := runtimeTrace == nil ||
				runtimeTrace.CredentialEffect == clientruntime.CredentialEffectInvalidate
			if shouldInvalidate {
				_ = s.OAuthPool.MarkInvalid(parentCtx, oldCredID,
					fmt.Sprintf("upstream %d rejected credential", status))
			} else if runtimeTrace.CredentialEffect == clientruntime.CredentialEffectCooldown {
				until := runtimeTrace.CooldownUntil
				if until.IsZero() {
					until = time.Now().Add(5 * time.Minute)
				}
				if recorder, ok := s.OAuthPool.(OAuthCredentialCooldownRecorder); ok {
					if err := recorder.MarkCooldown(parentCtx, oldCredID, until); err != nil {
						zap.L().Warn("failed to cool down oauth credential",
							requestLogFields(req, zap.String("credential_id", oldCredID), zap.Error(err))...,
						)
					}
				}
			}
		}
		decision := DecisionRetryNewCred
		if unauthorizedAttemptsForRoute(req, cand.RouteID) >= 2 {
			decision = DecisionRetry
		}
		return attemptResult{
			decision: decision,
			finalErr: apiError(http.StatusBadGateway, "upstream_error", "credential rejected"),
		}

	case DecisionRetry:
		drainAndClose(upResp)
		if outcome.Status == ResultUnauthorized && !cand.IsPoolRoute() && cand.EndpointID != "" && s.AccountState != nil {
			reason := fmt.Sprintf("runtime request: upstream returned HTTP %d", status)
			if _, err := s.AccountState.MarkAccountInvalid(parentCtx, cand.EndpointID, reason); err != nil {
				zap.L().Warn("failed to mark upstream account invalid",
					requestLogFields(req, zap.String("account_id", cand.EndpointID), zap.Error(err))...,
				)
			}
		}
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
		req.FailedStep = "execute"
		req.InternalErrorDetail = RedactInternalErrorDetail(errBody)
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

func isAmbiguousImageTimeout(err error) bool {
	return errors.Is(err, ErrResponseHeaderTimeout) ||
		errors.Is(err, ErrFirstByteTimeout) ||
		errors.Is(err, ErrIdleTimeout) ||
		errors.Is(err, ErrMaxDuration) ||
		errors.Is(err, context.DeadlineExceeded)
}

func finishAmbiguousImageTimeout(req *Request, message string) attemptResult {
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = http.StatusGatewayTimeout
	req.ErrorCode = "upstream_timeout"
	req.ErrorMessage = message
	req.FailedStep = "execute"
	return attemptResult{
		finished: true,
		finalErr: apiError(http.StatusGatewayTimeout, req.ErrorCode, message),
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

func (s *ExecuteStep) prepareBody(req *Request) (corebridge.PreparedRequest, error) {
	prepared, err := s.buildUpstreamBody(req)
	if err != nil {
		return corebridge.PreparedRequest{}, err
	}
	return prepared, nil
}

// pickCandidate returns the next candidate using the multi-dim scorer when
// available, or falls back to linear priority order. Health admission happens
// after local request preparation and immediately before the transport call.
func (s *ExecuteStep) pickCandidate(ctx context.Context, req *Request) (*domain.RouteCandidate, float64) {
	// Sticky is an explicit caller affinity decision. RouteCandidatesStep has
	// already validated the binding and BillingGuard has kept Candidate aligned
	// with the filtered list, so honor it for the first attempt before applying
	// structural tiers and dynamic scoring.
	if req.StickyHit && len(req.Attempts) == 0 && req.Candidate != nil && !req.UsedCandidates[req.Candidate.RouteID] {
		candidate := req.Candidate
		req.ModelCode = candidate.ModelCode
		return candidate, 0
	}
	for {
		// GroupRank and target Priority are hard failover boundaries. Protocol
		// conversion preference and dynamic scoring apply only among peers in the
		// active target-priority tier.
		tier := activeBucketTier(
			activePriorityTier(
				activeGroupTier(req.Candidates, req.UsedCandidates),
				req.UsedCandidates,
			),
			req.UsedCandidates,
		)
		if len(tier) == 0 {
			return nil, 0
		}
		var cand *domain.RouteCandidate
		var score float64
		scoring := RouteScoringContext{}
		if subject := req.RuntimeSubject(); subject != nil {
			scoring.TenantID = subject.TenantID
		}
		if sp, ok := s.Scorer.(ScoringPicker); ok {
			cand, score = sp.PickWithScore(ctx, scoring, tier, req.UsedCandidates)
		} else if s.Scorer != nil {
			cand = s.Scorer.Pick(ctx, scoring, tier, req.UsedCandidates)
		} else {
			cand = tier[0]
		}
		if cand == nil {
			return nil, 0
		}
		req.ModelCode = cand.ModelCode
		return cand, score
	}
}

// selectPoolCredential resolves the OAuth credential for a pool route. On the
// first attempt of a sticky-bound conversation it reuses the pinned credential
// so the upstream keeps seeing one continuous session; every other case (retry,
// pin no longer usable, no binding) falls back to the pool strategy.
func (s *ExecuteStep) selectPoolCredential(ctx context.Context, req *Request, cand *domain.RouteCandidate) (*domain.OAuthCredential, error) {
	if credID := stickyCredentialID(req, cand); credID != "" {
		if pinner, ok := s.OAuthPool.(PinnedCredentialSelector); ok {
			cred, err := pinner.SelectPinnedCredential(ctx, cand.PoolID, credID)
			if err == nil {
				return cred, nil
			}
			zap.L().Info("sticky credential unusable, falling back to pool selection",
				requestLogFields(req,
					zap.String("pool_id", cand.PoolID),
					zap.String("credential_id", credID),
					zap.Error(err),
				)...,
			)
		}
	}
	return s.OAuthPool.SelectCredentialFromPool(ctx, cand.PoolID, cand.OAuthStrategy)
}

// stickyCredentialID returns the credential this conversation is pinned to for
// the given candidate, or "" when the request is not sticky-bound to it.
func stickyCredentialID(req *Request, cand *domain.RouteCandidate) string {
	if req == nil || cand == nil || len(req.Attempts) > 0 {
		return ""
	}
	b := req.StickyBinding
	if !req.StickyHit || b == nil || b.TargetKind != "credential" {
		return ""
	}
	if b.RouteID != cand.RouteID {
		return ""
	}
	return b.CredentialID
}

func (s *ExecuteStep) candidateBlocked(candidate *domain.RouteCandidate) bool {
	if s.Health == nil || candidate == nil {
		return false
	}
	targetID := candidate.EndpointID
	if candidate.IsPoolRoute() {
		targetID = candidate.PoolID
	}
	return targetID != "" && s.Health.IsBlocked(targetID, candidateProbeLease(candidate))
}

func candidateProbeLease(candidate *domain.RouteCandidate) time.Duration {
	if candidate == nil || candidate.Timeouts.MaxDuration <= 0 {
		return 30 * time.Minute
	}
	return candidate.Timeouts.MaxDuration + 2*time.Minute
}

func (s *ExecuteStep) releaseHealthProbe(candidate *domain.RouteCandidate) {
	if s.Health == nil || candidate == nil {
		return
	}
	targetID, _ := healthTarget(candidate)
	if targetID != "" {
		s.Health.ReleaseProbe(targetID)
	}
}

func exhaustPhysicalTarget(req *Request, failed *domain.RouteCandidate) {
	if req == nil || failed == nil {
		return
	}
	key := physicalTargetKey(failed)
	for _, candidate := range req.Candidates {
		if candidate == nil {
			continue
		}
		if physicalTargetKey(candidate) == key {
			req.UsedCandidates[candidate.RouteID] = true
		}
	}
}

func physicalTargetKey(candidate *domain.RouteCandidate) string {
	if candidate == nil {
		return ""
	}
	if candidate.IsPoolRoute() {
		return "pool:" + candidate.PoolID
	}
	if candidate.EndpointID != "" {
		return "account:" + candidate.EndpointID
	}
	return "route:" + candidate.RouteID
}

// activeGroupTier exposes only the highest-ranked group that still has an
// unused route. A lower-priority group is failover, never a peer in scoring.
func activeGroupTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minRank := int(^uint(0) >> 1)
	for _, candidate := range candidates {
		if candidate == nil || used[candidate.RouteID] {
			continue
		}
		if candidate.GroupRank < minRank {
			minRank = candidate.GroupRank
		}
	}
	if minRank == int(^uint(0)>>1) {
		return nil
	}
	tier := make([]*domain.RouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && !used[candidate.RouteID] && candidate.GroupRank == minRank {
			tier = append(tier, candidate)
		}
	}
	return tier
}

// activeBucketTier returns the not-yet-used candidates sharing the lowest
// ConversionBucket. Restricting the scorer to this tier makes zero-conversion
// routes strictly preferred: a higher (more lossy) bucket is only reached once
// every lower-bucket route has been exhausted by failover (marked used).
func activeBucketTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minBucket := int(^uint(0) >> 1) // max int
	for _, c := range candidates {
		if used[c.RouteID] {
			continue
		}
		if c.ConversionBucket < minBucket {
			minBucket = c.ConversionBucket
		}
	}
	var tier []*domain.RouteCandidate
	for _, c := range candidates {
		if !used[c.RouteID] && c.ConversionBucket == minBucket {
			tier = append(tier, c)
		}
	}
	return tier
}

// activePriorityTier returns the not-yet-used candidates sharing the lowest
// Priority inside the current group. Restricting the scorer to this tier makes
// target priority a real runtime failover layer.
func activePriorityTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minPriority := int(^uint(0) >> 1)
	for _, c := range candidates {
		if used[c.RouteID] {
			continue
		}
		if c.Priority < minPriority {
			minPriority = c.Priority
		}
	}
	if minPriority == int(^uint(0)>>1) {
		return nil
	}
	var tier []*domain.RouteCandidate
	for _, c := range candidates {
		if !used[c.RouteID] && c.Priority == minPriority {
			tier = append(tier, c)
		}
	}
	return tier
}

// relay dispatches to sync or streaming relay based on req.IsStream, and within
// each, to the passthrough or cross-protocol-conversion path based on whether
// the client protocol differs from the chosen provider protocol (cand.Protocol).
// Same-protocol traffic stays on the battle-tested verbatim passthrough path;
// only a genuine client≠provider gap engages internal/formats translation.
func (s *ExecuteStep) relay(dc *deadlineController, req *Request, upResp *UpstreamResponse, startTime time.Time) error {
	// Image generation runs through a single unified relay that decouples the
	// upstream transport (streaming vs sync — governed by binding ImageStreamMode)
	// from the client-facing transport (governed by req.IsStream). It aggregates
	// any upstream SSE into a provider body and re-emits in the client's shape.
	if req.CapabilityType == domain.CapabilityImage {
		return s.executeImageRelay(dc, req, upResp, req.Envelope.W, startTime)
	}
	convert := req.Candidate != nil && req.ClientProtocol != req.Candidate.Protocol
	if req.IsStream {
		if convert {
			return s.executeStreamConvert(dc, req, upResp, req.Envelope.W, startTime)
		}
		return s.executeStream(dc, req, upResp, req.Envelope.W, startTime)
	}
	if convert {
		return s.executeSyncConvert(dc, req, upResp, req.Envelope.W)
	}
	return s.executeSync(dc, req, upResp, req.Envelope.W)
}

// recordAttempt appends a structured trace record for this attempt.
func (s *ExecuteStep) recordAttempt(req *Request, cand *domain.RouteCandidate, outcome Outcome, attemptStartedAt, transportStartedAt time.Time, latencyMs int, score float64) {
	targetID := cand.EndpointID
	credentialID := ""
	// Pool routes send PoolUpstreamModel to the upstream; account routes send
	// the account-mapping-rewritten UpstreamModel. Mirrors usage.go's
	// createUsageLog so the per-attempt trail and the final usage-log row
	// agree on which model string was actually sent.
	upstreamModel := cand.UpstreamModel
	if cand.IsPoolRoute() {
		upstreamModel = cand.PoolUpstreamModel
		if req.SelectedCredential != nil {
			targetID = req.SelectedCredential.ID
			credentialID = req.SelectedCredential.ID
		} else {
			targetID = cand.PoolID
		}
	}
	errMsg := ""
	if outcome.Err != nil {
		errMsg = outcome.Err.Error()
	}
	req.Attempts = append(req.Attempts, AttemptRecord{
		RouteID:            cand.RouteID,
		TargetID:           targetID,
		ProviderCode:       cand.ProviderCode,
		UpstreamModel:      upstreamModel,
		EndpointID:         cand.EndpointID,
		PoolID:             cand.PoolID,
		CredentialID:       credentialID,
		HTTPStatus:         outcome.HTTPStatus,
		Outcome:            outcome.Status,
		StartedAt:          attemptStartedAt,
		TransportStartedAt: transportStartedAt,
		LatencyMs:          latencyMs,
		ErrorMsg:           errMsg,
		Score:              score,
	})
}

// notifyHealth records the outcome with the HealthTracker and the OAuth pool.
// 429 is intentionally excluded from health failure accounting (see
// CountsAsHealthFailure). Both deployment and credential targets share the
// same HealthTracker interface; TargetKind distinguishes them in Snapshot.
func (s *ExecuteStep) notifyHealth(ctx context.Context, req *Request, cand *domain.RouteCandidate, outcome Outcome) {
	if s.Health != nil {
		targetID, kind := healthTarget(cand)
		switch outcome.Status {
		case ResultSuccess:
			s.Health.RecordSuccess(targetID, kind)
		default:
			if outcome.CountsAsHealthFailure() || (!cand.IsPoolRoute() && outcome.Status == ResultUnauthorized) {
				s.Health.RecordFailure(targetID, kind)
			}
		}
	}

	// Credential invalidation is explicit: only provider 401/403 responses
	// invalidate a credential. Route-level 5xx/network failures belong to the
	// pool circuit breaker and must not poison otherwise valid credentials.
	if cand.IsPoolRoute() && s.OAuthPool != nil && req.SelectedCredential != nil {
		credID := req.SelectedCredential.ID
		switch outcome.Status {
		case ResultSuccess:
			s.OAuthPool.RecordSuccess(ctx, credID)
		}
	}
}

// healthTarget resolves the HealthTracker target ID and kind for a candidate.
func healthTarget(cand *domain.RouteCandidate) (string, routing.TargetKind) {
	if cand.IsPoolRoute() {
		return cand.PoolID, routing.TargetPool
	}
	return cand.EndpointID, routing.TargetAccount
}

func (s *ExecuteStep) Rollback(_ context.Context, _ *Request) {}

// writeSticky persists the sticky binding for this conversation after a
// successful upstream call. No-op when Sticky is nil or req.ConversationID is
// empty (opt-in by caller).
func (s *ExecuteStep) writeSticky(ctx context.Context, req *Request, cand *domain.RouteCandidate) {
	subject := req.RuntimeSubject()
	if s.Sticky == nil || req.ConversationID == "" || subject == nil {
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
		b.TargetKind = "account"
		b.EndpointID = cand.EndpointID
	}
	if err := s.Sticky.SetBinding(ctx, subject.TenantID, runtimeSubjectStickyKey(subject), req.StickyModelKey(), req.ConversationID, &b); err != nil {
		zap.L().Warn("sticky write failed",
			requestLogFields(req,
				zap.String("conv_id", req.ConversationID),
				zap.String("route_id", cand.RouteID),
				zap.Error(err),
			)...,
		)
	}
}

// lastAttemptWas reports whether the most recent recorded attempt has the
// given status.
func lastAttemptWas(req *Request, status ResultStatus) bool {
	if len(req.Attempts) == 0 {
		return false
	}
	return req.Attempts[len(req.Attempts)-1].Outcome == status
}

func unauthorizedAttemptsForRoute(req *Request, routeID string) int {
	count := 0
	for _, attempt := range req.Attempts {
		if attempt.RouteID == routeID && attempt.Outcome == ResultUnauthorized {
			count++
		}
	}
	return count
}

// logUpstreamFailure emits a single structured Error log line for any
// non-success upstream attempt — transport errors AND HTTP 4xx/5xx. It is the
// authoritative observability hook for "why did the upstream call fail": it
// carries the URL we hit, route/deployment/provider, status, latency, the
// truncated upstream response body, and a bounded request summary. Without
// this, the only trace of an upstream failure is the access log info line,
// which has no upstream context.
func logUpstreamFailure(ctx context.Context, req *Request, cand *domain.RouteCandidate, url string, status, latencyMs int, callErr error, errBody, requestContentType, requestSummary string) {
	attrs := []zap.Field{
		zap.String("upstream_url", url),
		zap.Int("upstream_status", status),
		zap.Int("latency_ms", latencyMs),
		zap.String("route_id", cand.RouteID),
		zap.String("protocol", string(cand.Protocol)),
		zap.String("model_code", req.ModelCode),
		zap.String("upstream_model", cand.UpstreamModel),
		zap.String("provider_code", cand.ProviderCode),
		zap.String("endpoint_id", cand.EndpointID),
		zap.Bool("is_stream", req.IsStream),
	}
	attrs = append(attrs, timeoutLogFields(cand.Timeouts)...)
	if cand.PoolID != "" {
		attrs = append(attrs, zap.String("pool_id", cand.PoolID))
	}
	if callErr != nil {
		attrs = append(attrs, zap.String("transport_error", callErr.Error()))
	}
	if errBody != "" {
		attrs = append(attrs, zap.String("upstream_body", truncateValidUTF8(errBody, 1024)))
	}
	if requestContentType != "" {
		attrs = append(attrs, zap.String("upstream_content_type", requestContentType))
	}
	if requestSummary != "" {
		attrs = append(attrs, zap.String("upstream_request", truncateValidUTF8(requestSummary, 4096)))
	}
	attrs = append(requestLogFields(req), attrs...)
	zap.L().Error("upstream call failed", attrs...)
}

func timeoutLogFields(t domain.RouteTimeouts) []zap.Field {
	return []zap.Field{
		zap.Int64("response_header_timeout_ms", t.ResponseHeader.Milliseconds()),
		zap.Int64("first_byte_timeout_ms", t.FirstByte.Milliseconds()),
		zap.Int64("idle_timeout_ms", t.Idle.Milliseconds()),
		zap.Int64("max_duration_timeout_ms", t.MaxDuration.Milliseconds()),
	}
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
	bodyReader := &responseBodyReader{r: resp.Body, onFirstByte: dc.syncBodyPhase}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, syncBodyLimit(req)))
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
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return &precommitError{
			cause:      errUpstreamEmpty,
			httpStatus: resp.StatusCode,
			message:    fmt.Sprintf("upstream returned %d with an empty response body", resp.StatusCode),
		}
	}

	policy := publicEgressPolicy(req, req.Candidate)
	bodyBytes = s.Bridge.NormalizeResponseBody(req, bodyBytes)
	bodyBytes = egress.SanitizeJSON(bodyBytes, policy)

	// Extract the assistant reply for the audit log from the original body
	// (before model-name rewrite, but model identity is captured separately).
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(req.UpstreamResponseBody, req.Candidate.Protocol)

	// A 200 whose body is actually an error object is a failed attempt — fail
	// over to another route instead of relaying a broken success.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && payloadIsError(bodyBytes) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(bodyBytes)), 256),
		}
	}

	// Side-channel: usage for billing. ExtractSyncUsage returns a zero value
	// when upstream did not report tokens; fillEstimatedUsage then falls back
	// to byte-length estimation.
	if u := s.extractSyncUsage(req, bodyBytes); u.PromptTokens != 0 || u.CompletionTokens != 0 {
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
	req.MarkFirstResponseByte(time.Now())
	_, _ = w.Write(bodyBytes)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
	return nil
}

// responseBodyReader keeps the first-byte timer active until body data arrives.
// Streaming/aggregated callers can also reset an idle timer for later chunks.
type responseBodyReader struct {
	r           io.Reader
	onFirstByte func()
	onChunk     func()
	seen        bool
}

func (r *responseBodyReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		if !r.seen {
			r.seen = true
			r.onFirstByte()
		} else if r.onChunk != nil {
			r.onChunk()
		}
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

const (
	defaultSyncBodyLimitBytes = 32 * 1024 * 1024
	imageSyncBodyLimitBytes   = 256 * 1024 * 1024
)

func (s *ExecuteStep) executeStream(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}

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
	req.MarkFirstResponseByte(time.Now())
	req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
	zap.L().Info("stream started",
		requestLogFields(req,
			zap.String("model_code", req.ModelCode),
			zap.String("upstream_model", req.Candidate.UpstreamModel),
			zap.String("provider_code", req.Candidate.ProviderCode),
			zap.String("route_id", req.Candidate.RouteID),
			zap.Int("first_byte_ms", req.FirstTokenMs),
		)...,
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
		unwrapped := s.Bridge.NormalizeResponseBody(req, data)
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
		if u, ok := s.extractStreamUsage(req, req.TokenUsage, finalData, evt); ok {
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
		return finishStream(s, req, startTime, accumulatedOutputBytes, awaitErr, dc, w, flusher)
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
			return finishStream(s, req, startTime, accumulatedOutputBytes, readErr, dc, w, flusher)
		}
	}
}

// finishStream handles the end of a committed stream. io.EOF is a clean
// finish; anything else is a post-commit failure that earns a protocol-shaped
// error frame so the client is not left with a silent truncation.
func finishStream(s *ExecuteStep, req *Request, startTime time.Time, accumulatedOutputBytes int, readErr error, dc *deadlineController, w http.ResponseWriter, flusher http.Flusher) error {
	fillEstimatedUsage(req, accumulatedOutputBytes)

	if readErr == io.EOF {
		req.RequestStatus = domain.RequestSuccess
		req.HTTPStatus = http.StatusOK
		zap.L().Info("stream finished",
			requestLogFields(req,
				zap.String("model_code", req.ModelCode),
				zap.Int("first_token_ms", req.FirstTokenMs),
				zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
				zap.Int("prompt_tokens", req.TokenUsage.PromptTokens),
				zap.Int("completion_tokens", req.TokenUsage.CompletionTokens),
			)...,
		)
		return nil
	}

	// Post-commit failure — 200 OK is already sent and the body is partial.
	code, msg := streamFailureReason(dc, readErr)
	req.FailedStep = "execute"
	req.InternalErrorDetail = RedactInternalErrorDetail(msg)
	msg = egress.SanitizeText(msg, PublicEgressPolicy(req))
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = http.StatusOK
	req.ErrorCode = code
	req.ErrorMessage = msg
	if _, werr := w.Write(s.streamErrorFrame(req, code, msg)); werr == nil {
		flusher.Flush()
	}
	zap.L().Warn("stream aborted after commit",
		requestLogFields(req,
			zap.String("error_code", code),
			zap.String("error", msg),
			zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
		)...,
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

// ============================================================================
// Cross-protocol execution (P-C) — client≠provider 时经 internal/formats 翻译。
//
// 非流：读 provider-format 响应 → 提 usage/audit（provider 口径）→ ConvertResponse
// 翻成 client 格式 → 按 client 协议 egress sanitize → 写。
// 流式：两侧式翻译——NewStreamProvider(provider 格式) 把上游 SSE 解析成中性帧，
// NewStreamEmitter(client 格式) 重放成 client SSE；复用 passthrough 路径的 precommit
// 探错/commit/postcommit 错误帧语义。帧的 Model 改写为对外 model_code（替代 egress
// 的模型名重写，emit 产物本就是结构化干净输出）。
// ============================================================================

// applyCanonicalUsage maps canonical stream/response usage into req.TokenUsage,
// preserving any pre-populated image/video billing fields. No-op when usage
// carries no prompt/completion tokens (caller falls back to byte estimation).
func applyCanonicalUsage(req *Request, u *corebridge.Usage) {
	if u == nil || (u.InputTokens == 0 && u.OutputTokens == 0) {
		return
	}
	tu := domain.TokenUsage{
		PromptTokens:     int(u.InputTokens),
		CompletionTokens: int(u.OutputTokens),
		CacheWriteTokens: int(u.CacheWriteTokens),
		CacheReadTokens:  int(u.CacheReadTokens),
		ReasoningTokens:  int(u.ReasoningTokens),
	}
	tu.ImageCount = req.TokenUsage.ImageCount
	tu.ImageResolution = req.TokenUsage.ImageResolution
	tu.VideoSeconds = req.TokenUsage.VideoSeconds
	tu.VideoResolution = req.TokenUsage.VideoResolution
	req.TokenUsage = tu
}

func (s *ExecuteStep) executeSyncConvert(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyReader := &responseBodyReader{r: resp.Body, onFirstByte: dc.syncBodyPhase}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, syncBodyLimit(req)))
	if err != nil {
		if cause := dc.cause(); cause != nil {
			err = cause
		}
		return &precommitError{cause: err, httpStatus: resp.StatusCode, message: "read upstream body: " + err.Error()}
	}
	req.UpstreamResponseBody = bodyBytes
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return &precommitError{
			cause:      errUpstreamEmpty,
			httpStatus: resp.StatusCode,
			message:    fmt.Sprintf("upstream returned %d with an empty response body", resp.StatusCode),
		}
	}

	// provider-format body (strip CodeAssist envelope for gemini pools).
	provBody := s.Bridge.NormalizeResponseBody(req, bodyBytes)

	// A 200 whose body is an error object is a failed attempt — fail over.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && payloadIsError(provBody) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(provBody)), 256),
		}
	}

	// Usage + audit from the provider-format body, keyed on the provider protocol.
	if u := s.Bridge.ExtractSyncUsage(req, provBody); u.PromptTokens != 0 || u.CompletionTokens != 0 {
		u.ImageCount = req.TokenUsage.ImageCount
		u.ImageResolution = req.TokenUsage.ImageResolution
		u.VideoSeconds = req.TokenUsage.VideoSeconds
		u.VideoResolution = req.TokenUsage.VideoResolution
		req.TokenUsage = u
	}
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(req.UpstreamResponseBody, req.Candidate.Protocol)

	// Translate provider → client.
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "runtime bridge is not configured for sync response conversion"}
	}
	converted, cerr := s.Bridge.BridgeResponse(req, provBody)
	if cerr != nil {
		// 2xx but untranslatable — nothing written downstream, so fail over.
		return &precommitError{cause: cerr, httpStatus: resp.StatusCode, message: "convert response: " + cerr.Error()}
	}

	fillEstimatedUsage(req, len(provBody))

	// Sanitize with the CLIENT protocol policy — the bytes are now client-format.
	policy := publicEgressPolicy(req, req.Candidate)
	policy.Protocol = req.ClientProtocol
	out := egress.SanitizeJSON([]byte(converted), policy)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	req.ResponseCommitted = true
	req.MarkFirstResponseByte(time.Now())
	_, _ = w.Write(out)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
	return nil
}

func (s *ExecuteStep) executeStreamConvert(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "runtime bridge is not configured for stream conversion"}
	}
	provider, perr := s.Bridge.NewProvider(req)
	if perr != nil {
		return &precommitError{cause: perr, httpStatus: resp.StatusCode, message: perr.Error()}
	}
	emitter, eerr := s.Bridge.NewEmitter(req)
	if eerr != nil {
		return &precommitError{cause: eerr, httpStatus: resp.StatusCode, message: eerr.Error()}
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	dataPrefix := []byte("data: ")
	eventPrefix := []byte("event: ")
	donePayload := []byte("[DONE]")

	// ---- Phase 3 确认：读上游直到首个 data 帧（探错/空流 → failover）----
	var (
		consumed     [][]byte
		consumedLen  int
		firstData    []byte
		firstPayload []byte
		eventType    string
		awaitErr     error
	)
awaitLoop:
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		if len(line) > 0 {
			trimmed := trimEOL(line)
			switch {
			case bytes.HasPrefix(trimmed, dataPrefix):
				firstData = line
				firstPayload = trimmed[len(dataPrefix):]
				consumed = append(consumed, line)
				consumedLen += len(line)
				awaitErr = readErr
				break awaitLoop
			case bytes.HasPrefix(trimmed, eventPrefix):
				eventType = string(bytes.TrimPrefix(trimmed, eventPrefix))
				consumed = append(consumed, line)
				consumedLen += len(line)
			default:
				consumed = append(consumed, line)
				consumedLen += len(line)
			}
			if consumedLen > maxPreambleBytes {
				return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
					message: "upstream sent an oversized preamble without a data frame"}
			}
		}
		if readErr != nil {
			awaitErr = readErr
			break awaitLoop
		}
	}

	if firstData == nil {
		if cause := dc.cause(); cause != nil {
			awaitErr = cause
		}
		return precommitFromNoFrame(resp.StatusCode, joinSSELines(consumed), awaitErr)
	}
	if !bytes.Equal(firstPayload, donePayload) && (eventType == "error" || payloadIsError(firstPayload)) {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "upstream streamed an error before any content: " + truncateValidUTF8(string(firstPayload), 256)}
	}

	// ---- commit：响应不可逆 ----
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	req.ResponseCommitted = true
	req.HTTPStatus = http.StatusOK
	dc.firstByte()
	req.MarkFirstResponseByte(time.Now())
	req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
	zap.L().Info("stream started (convert)",
		requestLogFields(req,
			zap.String("model_code", req.ModelCode),
			zap.String("client_protocol", string(req.ClientProtocol)),
			zap.String("provider_protocol", string(req.Candidate.Protocol)),
			zap.String("route_id", req.Candidate.RouteID),
			zap.Int("first_byte_ms", req.FirstTokenMs),
		)...,
	)

	var auditBuf strings.Builder
	accumulatedOutputBytes := 0

	emitFrames := func(frames []corebridge.StreamFrame) error {
		for _, fr := range frames {
			if fr.Model != "" {
				fr.Model = req.PublicModel() // 对外模型名；替代 egress 模型重写
			}
			switch fr.Event {
			case corebridge.EvTextDelta, corebridge.EvReasoningDelta:
				auditBuf.WriteString(fr.Text)
				accumulatedOutputBytes += len(fr.Text)
			}
			if fr.HasFinish && fr.Usage != nil {
				applyCanonicalUsage(req, fr.Usage)
			}
			out, eerr := emitter.Emit(fr)
			if eerr != nil || len(out) == 0 {
				continue
			}
			if _, werr := w.Write(out); werr != nil {
				return werr
			}
		}
		return nil
	}
	pushLine := func(line []byte) error {
		frames, perr := provider.PushLine(line)
		if perr != nil {
			return nil // 解析瑕疵不致命，继续
		}
		return emitFrames(frames)
	}

	// 回放已消费的前导行 + 首帧。
	for _, line := range consumed {
		if err := pushLine(line); err != nil {
			return streamClientWriteError(req, err)
		}
	}
	flusher.Flush()

	if awaitErr != nil {
		return s.finishStreamConvert(req, startTime, &auditBuf, accumulatedOutputBytes, awaitErr, dc, w, flusher, provider, emitFrames, emitter)
	}

	// ---- Phase 4 传输 ----
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		if len(line) > 0 {
			dc.chunkReceived()
			if err := pushLine(line); err != nil {
				return streamClientWriteError(req, err)
			}
			flusher.Flush()
		}
		if readErr != nil {
			return s.finishStreamConvert(req, startTime, &auditBuf, accumulatedOutputBytes, readErr, dc, w, flusher, provider, emitFrames, emitter)
		}
	}
}

// finishStreamConvert ends a committed converted stream: on clean EOF it flushes
// the provider's trailing finish frame and the emitter's terminator (e.g. OpenAI
// [DONE] / Claude message_stop); on any other error it emits a client-protocol
// error frame (post-commit failure, no failover).
func (s *ExecuteStep) finishStreamConvert(
	req *Request, startTime time.Time, auditBuf *strings.Builder, accumulatedOutputBytes int,
	readErr error, dc *deadlineController, w http.ResponseWriter, flusher http.Flusher,
	provider corebridge.StreamProvider, emitFrames func([]corebridge.StreamFrame) error, emitter corebridge.StreamEmitter,
) error {
	if readErr == io.EOF {
		if frames, ferr := provider.Finish(); ferr == nil && len(frames) > 0 {
			_ = emitFrames(frames)
		}
		if tail, ferr := emitter.Finish(); ferr == nil && len(tail) > 0 {
			_, _ = w.Write(tail)
		}
		flusher.Flush()
		req.AuditResponseMessage = []byte(auditBuf.String())
		fillEstimatedUsage(req, accumulatedOutputBytes)
		req.RequestStatus = domain.RequestSuccess
		req.HTTPStatus = http.StatusOK
		zap.L().Info("stream finished (convert)",
			requestLogFields(req,
				zap.String("model_code", req.ModelCode),
				zap.Int("first_token_ms", req.FirstTokenMs),
				zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
				zap.Int("prompt_tokens", req.TokenUsage.PromptTokens),
				zap.Int("completion_tokens", req.TokenUsage.CompletionTokens),
			)...,
		)
		return nil
	}

	req.AuditResponseMessage = []byte(auditBuf.String())
	fillEstimatedUsage(req, accumulatedOutputBytes)
	code, msg := streamFailureReason(dc, readErr)
	req.FailedStep = "execute"
	req.InternalErrorDetail = RedactInternalErrorDetail(msg)
	msg = egress.SanitizeText(msg, PublicEgressPolicy(req))
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = http.StatusOK
	req.ErrorCode = code
	req.ErrorMessage = msg
	if _, werr := w.Write(s.Bridge.StreamErrorFrame(req, code, msg)); werr == nil {
		flusher.Flush()
	}
	zap.L().Warn("stream aborted after commit (convert)",
		requestLogFields(req,
			zap.String("error_code", code),
			zap.String("error", msg),
			zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
		)...,
	)
	return &postcommitError{code: code, message: msg}
}

// executeImageRelay is the unified image-generation relay. It reads the whole
// upstream response (image bodies are small and aggregatable), collapses any
// SSE into a single provider-format body, converts provider→client when the
// protocols differ (passthrough otherwise), and re-emits in the client's shape:
// a JSON object when the client asked for sync, or a single-frame SSE stream
// when the client asked to stream. This is what decouples the upstream
// transport (binding ImageStreamMode) from the client transport (req.IsStream).
func (s *ExecuteStep) executeImageRelay(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "runtime bridge is not configured for image relay"}
	}

	bodyReader := &responseBodyReader{
		r:           resp.Body,
		onFirstByte: dc.firstByte,
		onChunk:     dc.chunkReceived,
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, syncBodyLimit(req)))
	if err != nil {
		if cause := dc.cause(); cause != nil {
			err = cause
		}
		return &precommitError{cause: err, httpStatus: resp.StatusCode, message: "read upstream body: " + err.Error()}
	}
	req.UpstreamResponseBody = bodyBytes

	// Collapse the upstream response into a single provider-format JSON body.
	// If the upstream streamed (SSE) we aggregate its frames; otherwise we only
	// strip provider envelopes (e.g. CodeAssist wrapper for Gemini pools).
	var providerBody []byte
	if looksLikeSSEBody(bodyBytes) {
		providerBody, err = s.Bridge.AggregateImageProviderBody(req, bodyBytes)
		if err != nil {
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return &precommitError{cause: err, httpStatus: resp.StatusCode, message: "aggregate image stream: " + err.Error()}
		}
	} else {
		providerBody = s.Bridge.NormalizeResponseBody(req, bodyBytes)
	}

	// A 200 whose body is actually an error object is a failed attempt — fail over.
	if resp.StatusCode == http.StatusOK && payloadIsError(providerBody) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(providerBody)), 256),
		}
	}

	// Usage + audit from the provider-format body, keyed on the provider protocol.
	if u := s.Bridge.ExtractSyncUsage(req, providerBody); u.PromptTokens != 0 || u.CompletionTokens != 0 {
		u.ImageCount = req.TokenUsage.ImageCount
		u.ImageResolution = req.TokenUsage.ImageResolution
		u.VideoSeconds = req.TokenUsage.VideoSeconds
		u.VideoResolution = req.TokenUsage.VideoResolution
		req.TokenUsage = u
	}
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(providerBody, req.Candidate.Protocol)
	fillEstimatedUsage(req, len(providerBody))

	// Translate provider → client (passthrough when the protocols match, since
	// the image response bridge only defines cross-surface conversions).
	clientBody := providerBody
	if req.ClientProtocol != req.Candidate.Protocol {
		converted, cerr := s.Bridge.BridgeResponse(req, providerBody)
		if cerr != nil {
			return &precommitError{cause: cerr, httpStatus: resp.StatusCode, message: "convert image response: " + cerr.Error()}
		}
		clientBody = converted
	}
	if req.ClientProtocol == domain.ProtocolOpenAIImages && s.ImageNormalizer != nil {
		responseFormat := req.ImageClientResponseFormat
		if responseFormat == "" {
			responseFormat = domain.ImageResponseFormatB64
		}
		normalized, nerr := s.ImageNormalizer.NormalizeImageResponse(dc.ctx, clientBody, responseFormat)
		if nerr != nil {
			return &precommitError{cause: nerr, httpStatus: http.StatusBadGateway, message: "normalize image response: " + nerr.Error()}
		}
		clientBody = normalized
	}

	// Sanitize with the CLIENT protocol policy — the bytes are now client-format.
	policy := publicEgressPolicy(req, req.Candidate)
	policy.Protocol = req.ClientProtocol
	clientBody = egress.SanitizeJSON(clientBody, policy)

	if req.IsStream {
		return s.commitImageClientStream(dc, req, w, clientBody, resp.StatusCode, startTime)
	}
	return s.commitImageClientSync(req, w, clientBody, resp.StatusCode)
}

func (s *ExecuteStep) commitImageClientSync(req *Request, w http.ResponseWriter, clientBody []byte, statusCode int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	req.ResponseCommitted = true
	req.MarkFirstResponseByte(time.Now())
	_, _ = w.Write(clientBody)
	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = statusCode
	return nil
}

func (s *ExecuteStep) commitImageClientStream(dc *deadlineController, req *Request, w http.ResponseWriter, clientBody []byte, statusCode int, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}
	clientStream, serr := s.Bridge.BuildImageClientStream(req, clientBody)
	if serr != nil {
		var apiErr *APIError
		if errors.As(serr, &apiErr) {
			return apiErr
		}
		return &precommitError{cause: serr, httpStatus: statusCode, message: "build image client stream: " + serr.Error()}
	}
	if len(clientStream) == 0 {
		return &precommitError{cause: errUpstreamEmpty, httpStatus: statusCode, message: "image client stream is empty"}
	}

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	req.ResponseCommitted = true
	req.HTTPStatus = http.StatusOK
	dc.firstByte()
	req.MarkFirstResponseByte(time.Now())
	req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
	if _, err := w.Write(clientStream); err != nil {
		return streamClientWriteError(req, err)
	}
	flusher.Flush()
	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = http.StatusOK
	return nil
}

func (s *ExecuteStep) extractSyncUsage(req *Request, body []byte) domain.TokenUsage {
	return s.Bridge.ExtractSyncUsage(req, body)
}

func (s *ExecuteStep) extractStreamUsage(req *Request, prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool) {
	return s.Bridge.ExtractStreamUsage(req, prev, data, eventType)
}

func (s *ExecuteStep) streamErrorFrame(req *Request, code, msg string) []byte {
	return s.Bridge.StreamErrorFrame(req, code, msg)
}

// joinSSELines concatenates buffered SSE lines back into a single byte slice for
// error/empty diagnosis.
func joinSSELines(lines [][]byte) []byte {
	var out []byte
	for _, l := range lines {
		out = append(out, l...)
	}
	return out
}

// looksLikeSSEBody reports whether an upstream body is an SSE event stream
// rather than a single JSON document.
func looksLikeSSEBody(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.HasPrefix(trimmed, []byte("data:")) ||
		bytes.HasPrefix(trimmed, []byte("event:")) ||
		bytes.Contains(trimmed, []byte("\ndata:"))
}

func syncBodyLimit(req *Request) int64 {
	if req != nil && req.CapabilityType == domain.CapabilityImage {
		return imageSyncBodyLimitBytes
	}
	return defaultSyncBodyLimitBytes
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
	if probe.Type == "error" {
		return true
	}
	return formats.ErrorFieldIsPresent(probe.Error)
}

// streamClientWriteError marks the request as failed when the client socket
// breaks mid-stream and returns nil so the pipeline does not treat this as a
// pipeline-level error (the response is already committed, and the upstream
// itself was healthy).
func streamClientWriteError(req *Request, err error) error {
	req.RequestStatus = domain.RequestFailed
	req.ErrorCode = "stream_write_error"
	req.ErrorMessage = err.Error()
	req.FailedStep = "execute"
	req.InternalErrorDetail = RedactInternalErrorDetail(err.Error())
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
		PublicModel:        req.PublicModel(),
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
// upstream_model rather than the client's logical model name). Provider-side
// body shaping and cross-surface conversion are both injected via Bridge.
//
//   - Codex: strip max_output_tokens / temperature / top_p, force store=false,
//     default instructions. ApplyCodexRequestModifications operates on raw
//     bytes so client-supplied fields (previous_response_id, reasoning, etc.)
//     are preserved.
//
// Strict 1:1 protocol matching means the body's wire format already matches
// the upstream; no canonical round-trip and no cross-protocol translation
// happens here.
func (s *ExecuteStep) buildUpstreamBody(req *Request) (corebridge.PreparedRequest, error) {
	if req.Envelope == nil || len(req.Envelope.ClientBody) == 0 {
		return corebridge.PreparedRequest{}, fmt.Errorf("missing client request body")
	}
	if s == nil || s.Bridge == nil {
		return corebridge.PreparedRequest{}, fmt.Errorf("runtime bridge is not configured for request preparation")
	}
	prepared, err := s.Bridge.PrepareRequest(req, req.Envelope.ClientBody)
	if err != nil {
		return corebridge.PreparedRequest{}, err
	}
	return prepared, nil
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

// fillEstimatedUsage fills only token counters that were absent from the
// upstream response. outputBytes is the response body size for sync calls, or
// the accumulated delta-content length for streaming calls.
//
// Estimation formula: bytes / 3 (rounded up). Dividing by 3 is conservative —
// English text is ~4 chars/token but CJK and code are closer to 2–3 bytes/token,
// so using 3 avoids systematic under-billing.
func fillEstimatedUsage(req *Request, outputBytes int) {
	missingPrompt := req.TokenUsage.PromptTokens == 0
	missingCompletion := req.TokenUsage.CompletionTokens == 0
	switch {
	case !missingPrompt && !missingCompletion:
		req.TokenCountSource = domain.TokenUsageSourceUpstream
		return
	case missingPrompt && missingCompletion:
		req.TokenCountSource = domain.TokenUsageSourceEstimated
	default:
		req.TokenCountSource = domain.TokenUsageSourceMixed
	}
	if missingPrompt && req.UpstreamBodySize > 0 {
		req.TokenUsage.PromptTokens = (req.UpstreamBodySize + 2) / 3
	}
	if !missingCompletion {
		return
	}
	if outputBytes > 0 {
		req.TokenUsage.CompletionTokens = (outputBytes + 2) / 3
	} else {
		// A committed stream may fail before any measurable content arrives. Do
		// not invent a token charge; the settled amount must reflect observed
		// usage and may legitimately be zero for that request.
		req.TokenUsage.CompletionTokens = 0
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
