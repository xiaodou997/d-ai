package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/serving"
)

// UsageLogger implements serving.UsageLogger.
// It creates the usage log row, upserts the hourly rollup, confirms quota, and
// computes billing amounts using a three-tier price lookup:
//
//	1. ai_tenant_user_prices (user-specific override)
//	2. ai_tenant_model_price_overrides (tenant override)
//	3. ai_model_prices (base price)
type UsageLogger struct {
	q        *dbgen.Queries
	prices   *PriceResolver
	payloads *PayloadStore // optional; nil = payload persistence disabled
}

func NewUsageLogger(q *dbgen.Queries) *UsageLogger {
	return &UsageLogger{q: q, prices: NewPriceResolver(q)}
}

// SetPayloadStore enables payload persistence for failed/sampled requests.
func (l *UsageLogger) SetPayloadStore(ps *PayloadStore) {
	l.payloads = ps
}

// Log records a usage entry regardless of request success/failure.
// Billing amounts are written to req.BillingResult; if URMBiller.Confirm already
// computed them (req.BillingResolved == true) the pricing lookup is skipped.
func (l *UsageLogger) Log(ctx context.Context, req *serving.Request) error {
	if req.APIKey == nil || req.Candidate == nil {
		return nil
	}

	if !req.BillingResolved {
		pricing, err := l.prices.ResolvePricing(ctx, req)
		if err != nil {
			pricing = domain.ModelPricing{}
		}
		req.BillingResult = CalculateBilling(req.TokenUsage, pricing)
	}

	billing := req.BillingResult

	// Fire-and-forget quota confirmation. Note: QuotaReserved is the exact
	// amount we put aside in QuotaReserveStep (an estimate), not the actual
	// billing cost — releasing the actual cost would leave the reservation
	// counter drifting upward over time.
	apiKeyID := mustParseUUID(req.APIKey.KeyID)
	reservedAmount := req.QuotaReservedAmount
	go func() {
		defer recoverGoroutine("quota confirm", req.RequestID)
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = l.q.ConfirmAPIKeyQuotaUsage(bgCtx, dbgen.ConfirmAPIKeyQuotaUsageParams{
			ID:            apiKeyID,
			QuotaUsed:     billing.APIKeyQuotaCost,
			QuotaReserved: reservedAmount,
		})
	}()

	logID, err := l.createUsageLog(ctx, req, billing)
	if err != nil {
		return fmt.Errorf("create usage log: %w", err)
	}

	// Async payload persistence (failed + sampled). clientBody is not available
	// here; pass a best-effort copy from Envelope (may be nil for non-chat).
	if l.payloads != nil {
		var clientBody []byte
		if req.Envelope != nil {
			clientBody = req.Envelope.ClientBody
		}
		go func() {
			defer recoverGoroutine("payload save", req.RequestID)
			bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			l.payloads.Save(bgCtx, logID, req, clientBody)
		}()
	}

	// Async rollup — best effort
	go func() {
		defer recoverGoroutine("usage rollup", req.RequestID)
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = l.upsertRollup(bgCtx, req, billing)
	}()

	return nil
}

// recoverGoroutine is the panic firewall for fire-and-forget goroutines spawned
// by the usage logger. Without it a panic from any DB call (e.g. pool closed
// during shutdown) would unwind the entire process. The context.Background()
// chains above also mean these goroutines must die on their own — we cap them
// with a 10s timeout so they can't outlive shutdown indefinitely.
func recoverGoroutine(label, requestID string) {
	if rec := recover(); rec != nil {
		slog.Error("usage logger goroutine panic",
			"label", label,
			"panic", fmt.Sprint(rec),
			"request_id", requestID,
		)
	}
}

// ============================================================================
// DB writes
// ============================================================================

func (l *UsageLogger) createUsageLog(ctx context.Context, req *serving.Request, billing domain.BillingResult) (pgtype.UUID, error) {
	c := req.Candidate
	key := req.APIKey
	usage := req.TokenUsage

	modelRouteUUID := mustParseUUID(c.RouteID)
	apiKeyUUID := mustParseUUID(key.KeyID)
	modelUUID := mustParseUUID(c.ModelID)

	// Pool routes have no deployment/endpoint; deployment routes have no pool.
	deploymentUUID := mustParseUUID(c.DeploymentID)
	endpointUUID := mustParseUUID(c.EndpointID)
	poolUUID := mustParseUUID(c.PoolID)
	var credUUID pgtype.UUID
	if req.SelectedCredential != nil {
		credUUID = mustParseUUID(req.SelectedCredential.ID)
	}

	// Pool routes use PoolUpstreamModel; deployment routes use UpstreamModel.
	upstreamModel := c.UpstreamModel
	if c.IsPoolRoute() {
		upstreamModel = c.PoolUpstreamModel
	}

	params := dbgen.CreateUsageLogParams{
		RequestID:            req.RequestID,
		TraceID:              nullableText(req.TraceID),
		ApiKeyID:             apiKeyUUID,
		KeyOwnerType:         string(key.OwnerType),
		TenantID:             key.TenantID,
		UserID:               nullableText(key.UserID),
		ModelID:              modelUUID,
		ModelCode:            req.ModelCode,
		ModelRouteID:         modelRouteUUID,
		UpstreamDeploymentID: deploymentUUID,
		EndpointID:           endpointUUID,
		CredentialPoolID:     poolUUID,
		OauthCredentialID:    credUUID,
		ProviderCode:         nullableText(c.ProviderCode),
		UpstreamModel:        nullableText(upstreamModel),
		ProviderFormat:       nullableText(string(c.Protocol)),
		Stream:               req.IsStream,
		PromptTokens:         int32(usage.PromptTokens),
		CompletionTokens:     int32(usage.CompletionTokens),
		CacheWriteTokens:     int32(usage.CacheWriteTokens),
		CacheReadTokens:      int32(usage.CacheReadTokens),
		ReasoningTokens:      int32(usage.ReasoningTokens),
		TotalTokens:          int32(usage.TotalTokens()),
		BillableUnitType:     billing.BillableUnitType,
		BillableUnits:        billing.BillableUnits,
		ProviderCost:         billing.ProviderCost,
		PlatformCost:         billing.PlatformCost,
		UserCost:             billing.UserCost,
		ApiKeyQuotaCost:      billing.APIKeyQuotaCost,
		UrmTransactionID:     nullableText(req.URMTransactionID),
		BillingStatus:        billingStatus(req),
		RequestStatus:        string(req.RequestStatus),
		HttpStatus:           nullableInt4(req.HTTPStatus),
		UpstreamStatus:       nullableInt4(req.UpstreamStatus),
		LatencyMs:            nullableInt4(req.LatencyMs),
		FirstTokenLatencyMs:  nullableInt4(req.FirstTokenMs),
		ErrorCode:            nullableText(req.ErrorCode),
		ErrorMessage:         nullableText(req.ErrorMessage),
		UsageEstimated:  req.TokenCountSource == "estimated",
		UsageSource:     tokenCountSource(req.TokenCountSource),
		AttemptsCount:   int32(len(req.Attempts)),
		FinalRouteID:    mustParseUUID(c.RouteID),
		ClientProtocol:  string(req.ClientProtocol),
		Resolution:      nullableText(resolution(req)),
	}

	return l.q.CreateUsageLog(ctx, params)
}

func (l *UsageLogger) upsertRollup(ctx context.Context, req *serving.Request, billing domain.BillingResult) error {
	key := req.APIKey
	usage := req.TokenUsage

	return l.q.UpsertUsageRollupHourly(ctx, dbgen.UpsertUsageRollupHourlyParams{
		TenantID:         key.TenantID,
		UserID:           nullableText(key.UserID),
		ApiKeyID:         mustParseUUID(key.KeyID),
		ModelCode:        req.ModelCode,
		ProviderCode:     nullableText(req.Candidate.ProviderCode),
		RequestStatus:    string(req.RequestStatus),
		BillableUnitType: billing.BillableUnitType,
		PromptTokens:     int64(usage.PromptTokens),
		CompletionTokens: int64(usage.CompletionTokens),
		CacheWriteTokens: int64(usage.CacheWriteTokens),
		CacheReadTokens:  int64(usage.CacheReadTokens),
		ReasoningTokens:  int64(usage.ReasoningTokens),
		TotalTokens:      int64(usage.TotalTokens()),
		BillableUnits:    billing.BillableUnits,
		ProviderCost:     billing.ProviderCost,
		PlatformCost:     billing.PlatformCost,
		UserCost:         billing.UserCost,
		ApiKeyQuotaCost:  billing.APIKeyQuotaCost,
		LatencyMs:        nullableInt4(req.LatencyMs),
	})
}

// tokenCountSource normalises the source label; defaults to "upstream" when empty
// (e.g. failed requests where Execute never reached the estimation path).
func tokenCountSource(s string) string {
	if s == "" {
		return "upstream"
	}
	return s
}

// resolution returns the request resolution for image/video billing audit,
// or empty string for token-based requests.
func resolution(req *serving.Request) string {
	if req.TokenUsage.ImageResolution != "" {
		return req.TokenUsage.ImageResolution
	}
	return req.TokenUsage.VideoResolution
}

// billingStatus returns the appropriate billing_status value for the usage log.
func billingStatus(req *serving.Request) string {
	if req.URMTransactionID != "" {
		return string(domain.BillingConfirmed)
	}
	if req.BillingResult.PlatformCost == 0 {
		return string(domain.BillingFree)
	}
	return string(domain.BillingPending)
}

