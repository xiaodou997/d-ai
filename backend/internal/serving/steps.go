package serving

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

// ============================================================================
// AuthNStep — validates the API key and loads the key's owner context
// ============================================================================

// APIKeyResolver looks up an API key by its raw bearer token and populates req.APIKey.
type APIKeyResolver interface {
	ResolveAPIKey(ctx context.Context, token string, req *Request) error
}

// AuthNStep validates the incoming API key from the Authorization header.
type AuthNStep struct {
	Resolver APIKeyResolver
}

func (s *AuthNStep) Name() string { return "authn" }

func (s *AuthNStep) Execute(ctx context.Context, req *Request) error {
	token := bearerToken(req.R.Header.Get("Authorization"))
	if token == "" {
		return apiError(http.StatusUnauthorized, "missing_api_key", "API key required")
	}
	if err := s.Resolver.ResolveAPIKey(ctx, token, req); err != nil {
		return apiError(http.StatusUnauthorized, "invalid_api_key", "invalid or expired API key")
	}
	return nil
}

func (s *AuthNStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// AuthZStep — checks model access permission (tenant → user → key inheritance)
// ============================================================================

// ModelGrantChecker verifies the API key owner has access to the requested model.
type ModelGrantChecker interface {
	CheckModelGrant(ctx context.Context, req *Request) error
}

type AuthZStep struct {
	Checker ModelGrantChecker
}

func (s *AuthZStep) Name() string { return "authz" }

func (s *AuthZStep) Execute(ctx context.Context, req *Request) error {
	if err := s.Checker.CheckModelGrant(ctx, req); err != nil {
		return apiError(http.StatusForbidden, "model_not_authorized",
			fmt.Sprintf("access to model %q is not authorized for this API key", req.ModelCode))
	}
	return nil
}

func (s *AuthZStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// QuotaCheckStep — checks the API key's local quota (soft check, non-atomic)
// ============================================================================

type QuotaCheckStep struct{}

func (s *QuotaCheckStep) Name() string { return "quota_check" }

func (s *QuotaCheckStep) Execute(_ context.Context, req *Request) error {
	if req.APIKey == nil {
		return nil
	}
	avail := req.APIKey.QuotaAvailable()
	if avail == 0 {
		return apiError(http.StatusPaymentRequired, "quota_exceeded",
			"API key quota exhausted")
	}
	return nil
}

func (s *QuotaCheckStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// RouteSelectStep — selects the best upstream candidate for the model
// ============================================================================

// RouteSelector picks the best RouteCandidate for a request.
type RouteSelector interface {
	SelectRoute(ctx context.Context, req *Request) error
}

type RouteSelectStep struct {
	Selector RouteSelector
}

func (s *RouteSelectStep) Name() string { return "route_select" }

func (s *RouteSelectStep) Execute(ctx context.Context, req *Request) error {
	if err := s.Selector.SelectRoute(ctx, req); err != nil {
		return apiError(http.StatusServiceUnavailable, "no_available_route",
			"no healthy upstream route available for this model")
	}
	return nil
}

func (s *RouteSelectStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// RateLimitStep — enforces RPM / TPM limits per tenant and user
// ============================================================================

// RateLimiter checks and records rate limit tokens.
type RateLimiter interface {
	Check(ctx context.Context, req *Request) error
}

type RateLimitStep struct {
	Limiter RateLimiter
}

func (s *RateLimitStep) Name() string { return "rate_limit" }

func (s *RateLimitStep) Execute(ctx context.Context, req *Request) error {
	if s.Limiter == nil {
		return nil
	}
	if err := s.Limiter.Check(ctx, req); err != nil {
		return apiError(http.StatusTooManyRequests, "rate_limit_exceeded", err.Error())
	}
	return nil
}

func (s *RateLimitStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// QuotaReserveStep — atomically reserves quota on the API key
// ============================================================================

// QuotaReserver atomically reserves and releases API key quota.
type QuotaReserver interface {
	Reserve(ctx context.Context, req *Request, amount int64) error
	Release(ctx context.Context, req *Request, amount int64)
}

type QuotaReserveStep struct {
	Reserver       QuotaReserver
	EstimateTokens int64
}

func (s *QuotaReserveStep) Name() string { return "quota_reserve" }

func (s *QuotaReserveStep) Execute(ctx context.Context, req *Request) error {
	if s.Reserver == nil || req.APIKey == nil || req.APIKey.QuotaLimit == nil {
		return nil
	}
	estimate := s.EstimateTokens
	if estimate == 0 {
		estimate = 4096
	}
	if err := s.Reserver.Reserve(ctx, req, estimate); err != nil {
		return apiError(http.StatusPaymentRequired, "quota_reserve_failed", err.Error())
	}
	req.QuotaReservedAmount = estimate
	return nil
}

func (s *QuotaReserveStep) Rollback(ctx context.Context, req *Request) {
	if s.Reserver != nil && req.QuotaReservedAmount > 0 {
		s.Reserver.Release(ctx, req, req.QuotaReservedAmount)
		req.QuotaReservedAmount = 0
	}
}

// ============================================================================
// URMFreezeStep — freezes credits in URM before sending the upstream request
// ============================================================================

// BillingEstimate carries the pre-request cost estimate passed to URMBiller.Freeze,
// and the actual cost passed to URMBiller.Confirm.
type BillingEstimate struct {
	PlatformCost int64
	UserCost     int64
}

// URMBiller handles URM freeze / confirm / cancel operations.
type URMBiller interface {
	Freeze(ctx context.Context, req *Request, estimate BillingEstimate) error
	Confirm(ctx context.Context, req *Request, actual BillingEstimate) error
	Cancel(ctx context.Context, req *Request)
}

type URMFreezeStep struct {
	Biller        URMBiller
	EstimateCosts BillingEstimate
}

func (s *URMFreezeStep) Name() string { return "urm_freeze" }

func (s *URMFreezeStep) Execute(ctx context.Context, req *Request) error {
	if s.Biller == nil {
		return nil
	}
	if err := s.Biller.Freeze(ctx, req, s.EstimateCosts); err != nil {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"insufficient balance to process this request")
	}
	return nil
}

func (s *URMFreezeStep) Rollback(ctx context.Context, req *Request) {
	if s.Biller != nil && req.URMTransactionID != "" {
		s.Biller.Cancel(ctx, req)
	}
}

// ============================================================================
// URMConfirmStep — confirms actual usage with URM after request completes
// ============================================================================

type URMConfirmStep struct {
	Biller URMBiller
}

func (s *URMConfirmStep) Name() string { return "urm_confirm" }

func (s *URMConfirmStep) Execute(ctx context.Context, req *Request) error {
	if s.Biller == nil || req.URMTransactionID == "" {
		return nil
	}
	actual := BillingEstimate{
		PlatformCost: req.BillingResult.PlatformCost,
		UserCost:     req.BillingResult.UserCost,
	}
	if err := s.Biller.Confirm(ctx, req, actual); err != nil {
		slog.Warn("urm confirm failed (best-effort)", "error", err, "transaction_id", req.URMTransactionID)
	}
	return nil
}

func (s *URMConfirmStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// UsageLogStep — records the usage log regardless of success/failure
// ============================================================================

// UsageLogger persists a usage log entry.
type UsageLogger interface {
	Log(ctx context.Context, req *Request) error
}

// MetricsRecorder observes completed request metrics.
type MetricsRecorder interface {
	RecordRequest(req *Request)
}

type UsageLogStep struct {
	Logger  UsageLogger
	Metrics MetricsRecorder // optional
}

func (s *UsageLogStep) Name() string { return "usage_log" }

func (s *UsageLogStep) Execute(ctx context.Context, req *Request) error {
	if s.Logger != nil {
		if err := s.Logger.Log(ctx, req); err != nil {
			slog.WarnContext(ctx, "usage log failed (best-effort)",
				"error", err,
				"request_id", req.RequestID,
			)
		}
	}
	if s.Metrics != nil {
		s.Metrics.RecordRequest(req)
	}
	return nil
}

func (s *UsageLogStep) Rollback(_ context.Context, _ *Request) {}

// ============================================================================
// Helpers
// ============================================================================

// APIError is a structured error that maps to an HTTP response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d %s] %s", e.Status, e.Code, e.Message)
}

func apiError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
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
