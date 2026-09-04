package serving

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/audit"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/weborigin"
)

// ============================================================================
// AuthNStep — validates the API key and loads the key's owner context
// ============================================================================

// APIKeyResolver looks up an API key by its raw bearer token and returns the
// canonical runtime subject for the request.
type APIKeyResolver interface {
	ResolveSubject(ctx context.Context, token string) (coreidentity.Subject, error)
}

// AuthNStep validates the incoming API key from the Authorization header.
type AuthNStep struct {
	Resolver APIKeyResolver
}

func (s *AuthNStep) Name() string { return "authn" }

func (s *AuthNStep) Execute(ctx context.Context, req *Request) error {
	if req.RuntimeSubject() != nil {
		return nil
	}
	if req.Envelope == nil || req.Envelope.R == nil {
		return apiError(http.StatusInternalServerError, "missing_envelope", "request envelope not set")
	}
	token := bearerToken(req.Envelope.R.Header.Get("Authorization"))
	if token == "" {
		return apiError(http.StatusUnauthorized, "missing_api_key", "API key required")
	}
	subject, err := s.Resolver.ResolveSubject(ctx, token)
	if err != nil {
		return apiError(http.StatusUnauthorized, "invalid_api_key", "invalid or expired API key")
	}
	req.Subject = &subject
	return nil
}

func (s *AuthNStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// QuotaCheckStep — checks the API key's local quota (soft check, non-atomic)
// ============================================================================

type QuotaCheckStep struct{}

func (s *QuotaCheckStep) Name() string { return "quota_check" }

func (s *QuotaCheckStep) Execute(_ context.Context, req *Request) error {
	subject := req.RuntimeSubject()
	if !runtimeSubjectUsesAPIKeyQuota(subject) {
		return nil
	}
	if subject.QuotaLimit != nil {
		remaining := *subject.QuotaLimit - subject.QuotaUsed
		if remaining <= 0 {
			return apiError(http.StatusPaymentRequired, "quota_exceeded",
				"API key quota exhausted")
		}
	}
	return nil
}

func (s *QuotaCheckStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// RouteCandidatesStep — fetches all healthy candidate routes for the model
// ============================================================================

// RouteCandidateSelector returns the sorted candidate list for a model. The
// caller (ExecuteStep) is responsible for picking one to actually call and
// retrying against the remainder on failure. Sticky-pinned candidates should
// be returned at the head of the list.
type RouteCandidateSelector interface {
	SelectCandidates(ctx context.Context, req *Request) ([]*domain.RouteCandidate, error)
}

// RouteCandidatesStep populates req.Candidates with every route eligible to
// serve the current request. When Sticky is set and req.ConversationID is
// non-empty, it reads the sticky binding from Redis and promotes the matching
// candidate to the front of the list (setting req.StickyHit = true).
type RouteCandidatesStep struct {
	Selector RouteCandidateSelector
	Sticky   routing.StickyStore // optional; nil = sticky disabled
}

func (s *RouteCandidatesStep) Name() string { return "route_candidates" }

func (s *RouteCandidatesStep) Execute(ctx context.Context, req *Request) error {
	if len(req.Candidates) > 0 {
		req.Candidates = imageOutputEligibleCandidates(req, req.Candidates)
		if len(req.Candidates) == 0 {
			return unsupportedImageOutputCountError(req)
		}
		if req.UsedCandidates == nil {
			req.UsedCandidates = make(map[string]bool, len(req.Candidates))
		}
		if req.Candidate == nil {
			req.SetCandidate(req.Candidates[0])
		}
		return nil
	}

	candidates, err := s.Selector.SelectCandidates(ctx, req)
	if err != nil {
		// Preserve structured APIError (e.g. 400 no_matching_deployment)
		// instead of collapsing every selection failure into 503.
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return apiErrorWithCause(http.StatusServiceUnavailable, "no_available_route",
			"unable to resolve an available upstream route", err)
	}
	if len(candidates) == 0 {
		return apiError(http.StatusServiceUnavailable, "no_available_route",
			"no healthy upstream route available for this model")
	}
	candidates = imageOutputEligibleCandidates(req, candidates)
	if len(candidates) == 0 {
		return unsupportedImageOutputCountError(req)
	}

	// Sticky routing: promote the bound candidate to position 0 when found.
	if subject := req.RuntimeSubject(); s.Sticky != nil && req.ConversationID != "" && subject != nil {
		binding, berr := s.Sticky.GetBinding(ctx, subject.TenantID, runtimeSubjectStickyKey(subject), req.StickyModelKey(), req.ConversationID)
		if berr != nil {
			zap.L().Warn("sticky read failed", requestLogFields(req, zap.Error(berr))...)
		} else if binding != nil {
			if idx := findStickyCandidate(candidates, binding); idx > 0 {
				candidates[0], candidates[idx] = candidates[idx], candidates[0]
				req.StickyHit = true
				req.StickyBinding = binding
			} else if idx == 0 {
				req.StickyHit = true
				req.StickyBinding = binding
			}
		}
	}

	req.Candidates = candidates
	req.UsedCandidates = make(map[string]bool, len(candidates))
	req.SetCandidate(candidates[0])
	return nil
}

func imageOutputEligibleCandidates(req *Request, candidates []*domain.RouteCandidate) []*domain.RouteCandidate {
	if req == nil || req.CapabilityType != domain.CapabilityImage || req.TokenUsage.ImageCount <= domain.DefaultImageOutputCount {
		return candidates
	}
	requested := req.TokenUsage.ImageCount
	isEdit := strings.Contains(req.ClientPath, "/images/edits")
	eligible := make([]*domain.RouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		limit := candidate.ImageMaxOutputCount
		if isEdit {
			limit = candidate.ImageEditMaxOutputCount
		}
		if limit >= requested {
			eligible = append(eligible, candidate)
		}
	}
	return eligible
}

func unsupportedImageOutputCountError(req *Request) error {
	count := domain.DefaultImageOutputCount
	if req != nil && req.TokenUsage.ImageCount > 0 {
		count = req.TokenUsage.ImageCount
	}
	return apiError(http.StatusBadRequest, "image_count_not_supported",
		fmt.Sprintf("no available upstream route supports %d output images for this request", count))
}

func (s *RouteCandidatesStep) Rollback(_ context.Context, _ *Request) {}

// findStickyCandidate returns the index of the candidate matching the binding,
// or -1 when not found.
func findStickyCandidate(candidates []*domain.RouteCandidate, b *routing.StickyBinding) int {
	for i, c := range candidates {
		if c.RouteID != b.RouteID {
			continue
		}
		switch b.TargetKind {
		case "endpoint", "account": // account is the legacy pre-endpoint label.
			if c.EndpointID == b.EndpointID {
				return i
			}
		case "credential":
			// Pool routes match by RouteID; credential selection happens in Execute.
			if c.IsPoolRoute() {
				return i
			}
		}
	}
	return -1
}

// ============================================================================
// RateLimitStep — enforces caller concurrency limits
// ============================================================================

// RateLimiter checks and records rate limit tokens.
type RateLimiter interface {
	Acquire(ctx context.Context, req *Request) (RateLimitLease, error)
}

type RateLimitLease interface {
	Release(ctx context.Context)
}

var (
	ErrRateLimitExceeded      = errors.New("rate limit exceeded")
	ErrRateLimiterUnavailable = errors.New("rate limiter unavailable")
)

type RateLimitStep struct {
	Limiter RateLimiter
}

func (s *RateLimitStep) Name() string { return "rate_limit" }

func (s *RateLimitStep) Execute(ctx context.Context, req *Request) error {
	if s.Limiter == nil {
		return nil
	}
	lease, err := s.Limiter.Acquire(ctx, req)
	if err != nil {
		if errors.Is(err, ErrRateLimiterUnavailable) {
			return apiErrorWithCause(http.StatusServiceUnavailable, "rate_limiter_unavailable", "unable to verify runtime limits", err)
		}
		return apiErrorWithCause(http.StatusTooManyRequests, "rate_limit_exceeded", "request rate limit exceeded", err)
	}
	req.RateLimitLease = lease
	return nil
}

func (s *RateLimitStep) Rollback(ctx context.Context, req *Request) {
	releaseRateLimitLease(ctx, req)
}

type RateLimitFinalizer struct{}

func (RateLimitFinalizer) Name() string { return "rate_limit_release" }

func (RateLimitFinalizer) Finalize(ctx context.Context, req *Request) {
	releaseRateLimitLease(ctx, req)
}

func releaseRateLimitLease(ctx context.Context, req *Request) {
	if req == nil || req.RateLimitLease == nil {
		return
	}
	lease := req.RateLimitLease
	req.RateLimitLease = nil
	lease.Release(ctx)
}

// ============================================================================
// UsageLogFinalizer — records usage and completes financial mutations after
// every routed request, including terminal upstream failures.
// ============================================================================

// UsageLogger persists a usage log entry.
type UsageLogger interface {
	Log(ctx context.Context, req *Request) error
}

// MetricsRecorder observes completed request metrics.
type MetricsRecorder interface {
	RecordRequest(req *Request)
}

type UsageLogFinalizer struct {
	Logger  UsageLogger
	Metrics MetricsRecorder // optional
}

const financialCompletionTimeout = 8 * time.Second

func (s *UsageLogFinalizer) Name() string { return "usage_completion" }

func (s *UsageLogFinalizer) Finalize(ctx context.Context, req *Request) {
	if req != nil && req.AuditPayload == nil {
		req.AuditPayload = BuildAuditPayload(req)
	}
	req.MarkCompleted(time.Now())
	if s.Logger != nil {
		// The upstream result may already be committed to the client. Financial
		// completion must therefore survive a client disconnect, while retaining
		// a strict deadline so shutdown and database outages cannot hang workers.
		completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), financialCompletionTimeout)
		err := s.Logger.Log(completionCtx, req)
		cancel()
		if err != nil {
			req.FinancialCompletionFailed = true
			zap.L().Error("financial completion failed", requestLogFields(req, zap.Error(err))...)
		} else {
			req.FinancialCompletionFailed = false
		}
	}
	if s.Metrics != nil {
		s.Metrics.RecordRequest(req)
	}
}

// ============================================================================
// AuditFinalizer — durably enqueues the full request/response payload
// ============================================================================

// AuditSubmitter enqueues a payload for durable asynchronous materialization.
type AuditSubmitter interface {
	Submit(p *audit.Payload) bool
}

// ContextAuditSubmitter is implemented by durable submitters that can retain
// the enqueue briefly after a client disconnects.
type ContextAuditSubmitter interface {
	SubmitContext(ctx context.Context, p *audit.Payload) bool
}

// AuditFinalizer implements Finalizer: builds an audit.Payload from the
// completed request (success or failure) and submits it to the Worker.
// It runs unconditionally after the pipeline completes, including for
// requests that fail before reaching the Execute step (auth/quota/routing).
type AuditFinalizer struct {
	Worker AuditSubmitter
}

func (f *AuditFinalizer) Name() string { return "audit_log" }

func (f *AuditFinalizer) Finalize(ctx context.Context, req *Request) {
	if f.Worker == nil {
		return
	}
	p := req.AuditPayload
	if p == nil {
		p = BuildAuditPayload(req)
		req.AuditPayload = p
	}
	if submitter, ok := f.Worker.(ContextAuditSubmitter); ok {
		enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		submitter.SubmitContext(enqueueCtx, p)
		cancel()
		return
	}
	f.Worker.Submit(p)
}

// BuildAuditPayload converts the completed request into the durable audit
// envelope. It is shared by UsageLogFinalizer and AuditFinalizer so normal
// requests can enqueue it in the usage transaction without rebuilding it.
func BuildAuditPayload(req *Request) *audit.Payload {
	if req == nil {
		return nil
	}

	var messages, params json.RawMessage
	authHeader := ""
	requestPath := ""
	clientIP := ""
	userAgent := ""
	if req.Envelope != nil {
		contentType := ""
		if req.Envelope.R != nil {
			contentType = req.Envelope.R.Header.Get("Content-Type")
		}
		messages, params = audit.ExtractRequestPayloadWithContentType(req.Envelope.ClientBody, req.ClientProtocol, contentType)
		if req.Envelope.R != nil {
			authHeader = audit.MaskAuthorization(req.Envelope.R.Header.Get("Authorization"))
			requestPath = req.Envelope.R.URL.Path
			clientIP = clientIPFromRequest(req.Envelope.R)
			userAgent = req.Envelope.R.Header.Get("User-Agent")
		}
	}

	// For the images protocol the response body contains b64_json arrays;
	// pass the raw upstream body so the Worker can extract blobs from it.
	responseMessage := req.AuditResponseMessage
	if req.ClientProtocol == domain.ProtocolOpenAIImages {
		responseMessage = req.UpstreamResponseBody
	}

	p := &audit.Payload{
		RequestID:                   req.RequestID,
		ClientProtocol:              string(req.ClientProtocol),
		ClientIP:                    clientIP,
		UserAgent:                   userAgent,
		RequestPath:                 requestPath,
		AuthMasked:                  authHeader,
		RequestModel:                req.PublicModel(),
		MatchedDispatchRuleID:       req.MatchedDispatchRuleID,
		MatchedDispatchRuleSummary:  req.MatchedDispatchRuleSummary,
		ResolvedLogicalModel:        req.ResolvedLogicalModel,
		ResolvedProviderFamily:      req.ResolvedProviderFamily,
		ProtocolConversionEnabled:   req.ProtocolConversionEnabled,
		SelectedUpstreamProtocol:    req.SelectedUpstreamProtocol,
		SelectedUpstreamModel:       req.SelectedUpstreamModel,
		UpstreamModelMappingApplied: req.UpstreamModelMappingApplied,
		PublicResponseModel:         req.PublicModel(),
		RequestMessages:             messages,
		RequestParams:               params,
		ResponseMessage:             responseMessage,
		RequestStatus:               string(req.RequestStatus),
		HTTPStatus:                  req.HTTPStatus,
		ErrorCode:                   req.ErrorCode,
		InternalErrorDetail:         req.InternalErrorDetail,
		FailedStep:                  req.FailedStep,
		AttemptsDetail:              BuildAttemptsDetail(req.Attempts),
	}
	return p
}

// ============================================================================
// Helpers
// ============================================================================

// APIError is a structured error that maps to an HTTP response.
type APIError struct {
	Status  int
	Code    string
	Message string
	cause   error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d %s] %s", e.Status, e.Code, e.Message)
}

func (e *APIError) Unwrap() error { return e.cause }

func apiError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func apiErrorWithCause(status int, code, message string, cause error) *APIError {
	return &APIError{Status: status, Code: code, Message: message, cause: cause}
}

// clientIPFromRequest returns the originating client IP through the shared
// trusted-proxy resolver, falling back to the direct connection address.
func clientIPFromRequest(r *http.Request) string {
	return weborigin.ClientIPFromRequest(r)
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) {
		return ""
	}
	if header[:len(prefix)] != prefix {
		return ""
	}
	return header[len(prefix):]
}
