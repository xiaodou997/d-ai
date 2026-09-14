package serving

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"xiaodou/dai/internal/ai/clientruntime"
	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/egress"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/privacy"
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

type ModuleGate interface {
	IsActive(ctx context.Context, name string) (bool, error)
}

type PIIProtectionProvider interface {
	PIIProtection(ctx context.Context) (bool, *privacy.Protector, error)
}

const modulePIIProtection = "pii_protection"

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
	CompletionRecorder UsageLogger
	Transport          Transporter
	ClientRuntime      clientruntime.Invoker      // fixed OAuth providers; nil keeps the legacy path
	UpstreamLimiter    UpstreamConcurrencyLimiter // optional unless a direct account caps concurrency
	Bridge             ProtocolBridge             // required for cross-surface request/response conversion
	Availability       routing.Availability
	OAuthPool          OAuthCredentialPool     // optional; enables rejected-credential swaps
	Budget             RetryBudget             // zero value falls back to DefaultRetryBudget
	Scorer             RouteScorer             // optional; nil = first unused candidate (P1 behaviour)
	Stats              routing.RouteStatsStore // optional; used for inflight tracking alongside scorer
	Sticky             stickyWriter            // optional; writes/deletes sticky binding on success/failure
	ImageNormalizer    ImageResponseNormalizer // optional; normalizes image URL/Base64 response mismatches
	ModuleGate         ModuleGate              // optional; controls feature module activation
	ContentModeration  *ContentModerationStep  // optional; runs after each candidate is selected
	PromptAudit        *PromptAuditStep        // optional; runs after each candidate is selected
	Privacy            *privacy.Protector      // optional; protects upstream request content
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
	if req.RecordLeaseContext != nil {
		ctx = req.RecordLeaseContext
	}
	initializeSettlementState(req)
	if s == nil || s.Transport == nil || s.Bridge == nil {
		return apiError(http.StatusInternalServerError, "runtime_not_configured", "runtime execution is not fully configured")
	}
	if len(req.Candidates) == 0 {
		return apiError(http.StatusServiceUnavailable, "no_available_route", "no route candidates")
	}
	if req.Envelope == nil || req.Envelope.W == nil {
		return apiError(http.StatusInternalServerError, "missing_envelope", "request envelope not set")
	}

	budget := s.Budget.Normalize(req)
	executionCtx, cancelExecution := context.WithTimeoutCause(ctx, budget.MaxElapsed, ErrRetryDeadlineExceeded)
	defer cancelExecution()
	if err := s.expandCredentialCandidates(executionCtx, req); err != nil {
		return apiErrorWithCause(503, "credential_state_unavailable", "upstream credentials cannot be verified", err)
	}

	// The upstream body is (re)built per picked candidate: its bytes embed both
	// the provider wire format (client→provider conversion) and the upstream
	// model name, so it must track the actually-selected route — including the
	// very first attempt, whose pick may differ from candidates[0].
	var (
		prepared     corebridge.PreparedRequest
		bodyBuiltFor string
		lastErr      error
	)
	for len(req.Attempts) < budget.MaxAttempts {
		if err := requestContextError(ctx, executionCtx, req); err != nil {
			return err
		}
		cand, score := s.pickCandidate(executionCtx, req)
		if req.SelectionError != nil {
			return apiErrorWithCause(503, "availability_state_unavailable", "upstream availability cannot be verified", req.SelectionError)
		}
		if cand == nil {
			break // exhausted all candidates
		}
		req.SetCandidate(cand)
		if cand.IsPoolRoute() && req.PreparedCredentials != nil {
			req.SelectedCredential = req.PreparedCredentials[cand.Key()]
		}
		if s.PromptAudit != nil {
			if err := s.PromptAudit.Execute(executionCtx, req); err != nil {
				return err
			}
		}
		if s.ContentModeration != nil {
			if err := s.ContentModeration.Execute(executionCtx, req); err != nil {
				return err
			}
		}

		// Pool routes: select a fresh credential per attempt so auth-swap and
		// new-route paths both get a clean credential.
		if cand.IsPoolRoute() && s.OAuthPool != nil && req.SelectedCredential == nil {
			var cred *domain.OAuthCredential
			var selErr error
			if cand.CredentialID != "" {
				cred, selErr = s.OAuthPool.(PinnedCredentialSelector).SelectPinnedCredential(executionCtx, cand.PoolID, cand.CredentialID)
			} else {
				cred, selErr = s.selectPoolCredential(executionCtx, req, cand)
			}
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

		// Build (or rebuild) the upstream body for the selected route. The body
		// embeds the provider wire format and upstream model, so any change of
		// route — including the first pick when it isn't candidates[0] — requires
		// a fresh build (e.g. openai_chat client → anthropic_messages provider).
		if len(prepared.Body) == 0 || cand.Key() != bodyBuiltFor {
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
			bodyBuiltFor = cand.Key()
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
		if err := s.acquireAvailability(executionCtx, req, cand); err != nil {
			var denied *routing.AdmissionDenied
			if !errors.As(err, &denied) {
				return apiErrorWithCause(503, "availability_state_unavailable", "upstream availability cannot be verified", err)
			}
			req.recordSkippedCandidate(cand, denied.Reason)
			req.UsedCandidates[cand.Key()] = true
			req.SelectedCredential = nil
			continue
		}
		// Claim a concurrency slot for the account. It is released inside
		// runAttempt, which returns only after the relay has finished, so the
		// slot is held for exactly as long as the attempt occupies the upstream.
		slot, err := s.acquireUpstreamSlot(executionCtx, req, cand)
		if err != nil {
			s.releaseAvailability(req)
			if errors.Is(err, ErrUpstreamConcurrencyExceeded) {
				req.recordSkippedCandidate(cand, "upstream_capacity_exhausted")
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
		if len(req.SkippedAttempts) > 0 {
			zap.L().Info("no healthy upstream route: every candidate skipped before reaching transport",
				requestLogFields(req, zap.Int("skipped_candidates", len(req.SkippedAttempts)))...)
		}
		setAvailabilityRetryAfter(req)
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

func (s *ExecuteStep) prepareBody(req *Request) (corebridge.PreparedRequest, error) {
	prepared, err := s.buildUpstreamBody(req)
	if err != nil {
		return corebridge.PreparedRequest{}, err
	}
	return prepared, nil
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

// recordSkippedCandidate appends a Synthetic AttemptRecord for a candidate that
// was filtered out before any upstream transport call (e.g. circuit breaker
// open). It carries observability fields + outcome=circuit_open but is stored
// on req.SkippedAttempts — never req.Attempts — so it stays out of the retry
// budget, the X-Route-Trace header, and 429 backoff bookkeeping.
func (req *Request) recordSkippedCandidate(cand *domain.RouteCandidate, reason string) {
	if req == nil || cand == nil {
		return
	}
	targetID := cand.EndpointID
	if cand.IsPoolRoute() {
		targetID = cand.PoolID
	}
	status := ResultCircuitOpen
	if reason == "no_credential" || reason == "local_request_error" || reason == "upstream_capacity_exhausted" {
		status = ResultRejected
	}
	skipped := AttemptRecord{
		Sequence:    len(req.Attempts) + len(req.SkippedAttempts) + 1,
		CandidateID: cand.Key(), AccountID: cand.EffectiveAccountID(), ModelCode: cand.ModelCode, Operation: cand.OperationKey(), Stream: req.IsStream, CompletedAt: time.Now(),
		RouteID:         cand.RouteID,
		GroupID:         cand.GroupID,
		RoutePolicy:     cand.RoutePolicy,
		GroupRank:       cand.GroupRank,
		TargetPriority:  cand.TargetPriority,
		SelectionReason: req.SelectionReason,
		TargetID:        targetID,
		ProviderCode:    cand.ProviderCode,
		EndpointID:      cand.EndpointID,
		PoolID:          cand.PoolID,
		UpstreamModel:   cand.EffectiveUpstreamModel(),
		HTTPStatus:      0,
		Outcome:         status,
		ErrorMsg:        reason,
		Score:           0,
	}
	req.SkippedAttempts = append(req.SkippedAttempts, skipped)
}
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
		PricingSnapshot: req.BillingSnapshots[cand.Key()],
		CandidateID:     cand.Key(), AccountID: cand.EffectiveAccountID(), ModelCode: cand.ModelCode, Operation: cand.OperationKey(), Stream: req.IsStream,
		Sequence:           len(req.Attempts) + len(req.SkippedAttempts) + 1,
		RouteID:            cand.RouteID,
		GroupID:            cand.GroupID,
		RoutePolicy:        cand.RoutePolicy,
		GroupRank:          cand.GroupRank,
		TargetPriority:     cand.TargetPriority,
		SelectionReason:    req.SelectionReason,
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

// recordOutcome derives the single availability verdict for this actual attempt.
func (s *ExecuteStep) recordOutcome(ctx context.Context, req *Request, cand *domain.RouteCandidate, outcome Outcome) {
	req.AvailabilityResult = availabilityVerdict(req, cand, outcome)
	if len(req.Attempts) > 0 {
		a := &req.Attempts[len(req.Attempts)-1]
		a.Outcome = outcome.Status
		a.UpstreamErrorCode = outcome.ErrorCode
		a.AvailabilityOutcome = outcome.Status.String()
		a.FailureScope = req.AvailabilityResult.FailureScope
	}

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
		b.TargetKind = "endpoint"
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
	outcome := formats.InspectStreamOutcome(data, "")
	return outcome.State == domain.ProviderTerminalFailed || outcome.State == domain.ProviderTerminalIncomplete || outcome.State == domain.ProviderTerminalCancelled

}

// streamClientWriteError marks the request as failed when the client socket
// breaks mid-stream and returns nil so the pipeline does not treat this as a
// pipeline-level error (the response is already committed, and the upstream
// itself was healthy).
func streamClientWriteError(req *Request, err error) error {
	initializeSettlementState(req)
	if isClientDisconnectError(err) {
		markClientCancellation(req)
		req.ErrorCode = "client_disconnected"
	} else {
		req.RequestStatus = domain.RequestFailed
		markClientDelivery(req, domain.ClientDeliveryWriteFailed)
		req.ErrorCode = "stream_write_error"
	}
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
	body := req.Envelope.ClientBody
	if s.Privacy != nil && s.ModuleGate != nil {
		checkCtx := context.Background()
		if req.Envelope.R != nil {
			checkCtx = req.Envelope.R.Context()
		}
		active := false
		protector := s.Privacy
		if provider, ok := s.ModuleGate.(PIIProtectionProvider); ok {
			configuredProtector := (*privacy.Protector)(nil)
			var err error
			active, configuredProtector, err = provider.PIIProtection(checkCtx)
			if err != nil {
				return corebridge.PreparedRequest{}, fmt.Errorf("load pii protection config: %w", err)
			}
			if configuredProtector != nil {
				protector = configuredProtector
			}
		} else {
			var err error
			active, err = s.ModuleGate.IsActive(checkCtx, modulePIIProtection)
			if err != nil {
				return corebridge.PreparedRequest{}, fmt.Errorf("check pii protection module: %w", err)
			}
		}
		if active && req.PIIMap == nil {
			protected, mapping, err := protector.RedactJSON(body)
			if err != nil {
				return corebridge.PreparedRequest{}, fmt.Errorf("protect request body: %w", err)
			}
			req.PIIProtectedBody, req.PIIMap = protected, mapping
		}
		if active && req.PIIProtectedBody != nil {
			body = req.PIIProtectedBody
		}
	}
	prepared, err := s.Bridge.PrepareRequest(req, body)
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

// fillEstimatedUsage is kept as an internal call-site name during migration;
// no capability may synthesize counters from body length.
func fillEstimatedUsage(req *Request, _ int) {
	if domain.UsesReportedTokenBilling(req.CapabilityType) {
		ApplyReportedUsage(req)
		return
	}
	req.TokenCountSource = domain.TokenUsageSourceMissing
	if req.TokenUsage.PromptTokens > 0 || req.TokenUsage.CompletionTokens > 0 {
		req.TokenCountSource = domain.TokenUsageSourceUpstream
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
