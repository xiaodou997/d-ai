package serving

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/internal/ai/clientruntime"
	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
)

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
