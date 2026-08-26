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
	ModuleGate      ModuleGate                 // optional; controls feature module activation
	Privacy         *privacy.Protector         // optional; protects upstream request content
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
