// Package urm implements the serving.URMBiller interface using the URM settlement API.
package urm

import (
	"context"
	"go.uber.org/zap"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/serving"
	"xiaodou/uni-ai-api/internal/urm"
)

// urmClient is the subset of urm.Client used by Biller.
type urmClient interface {
	Freeze(ctx context.Context, req urm.FreezeRequest) (*urm.FreezeResponse, error)
	Confirm(ctx context.Context, req urm.ConfirmRequest) (*urm.ConfirmResponse, error)
	Cancel(ctx context.Context, transactionID string) error
}

// PricingResolver resolves the effective model price for a request.
type PricingResolver interface {
	ResolvePricing(ctx context.Context, req *serving.Request) (domain.ModelPricing, error)
}

// BillingCalculator computes billing amounts from token usage and pricing.
type BillingCalculator func(usage domain.TokenUsage, pricing domain.ModelPricing) domain.BillingResult

// Biller implements serving.URMBiller.
//
// Freeze: looks up current pricing, estimates cost from defaultTokenEstimate,
// calls URM freeze to hold credits, stores the transaction ID on the request.
//
// Confirm: looks up pricing again with actual token usage, computes real costs,
// writes them to req.BillingResult, then calls URM confirm with actual amounts.
//
// Cancel: cancels the URM transaction on pipeline rollback.
type Biller struct {
	client               urmClient
	pricing              PricingResolver
	calculate            BillingCalculator
	defaultTokenEstimate int // tokens to assume when freezing before actual usage is known
	logger               *zap.Logger
}

func NewBiller(
	client urmClient,
	pricing PricingResolver,
	calculate BillingCalculator,
	defaultTokenEstimate int,
	logger *zap.Logger,
) *Biller {
	if defaultTokenEstimate <= 0 {
		defaultTokenEstimate = 4096
	}
	if logger == nil {
		logger = zap.L()
	}
	return &Biller{
		client:               client,
		pricing:              pricing,
		calculate:            calculate,
		defaultTokenEstimate: defaultTokenEstimate,
		logger:               logger,
	}
}

// Freeze holds estimated credits in URM before the upstream request executes.
// If the passed-in estimate is non-zero it is used directly; otherwise the biller
// computes an estimate from current pricing × defaultTokenEstimate.
//
// estimate values are in micro-credits; we ceil-convert to integer credits at
// the URM boundary so the hold always covers the estimated cost.
func (b *Biller) Freeze(ctx context.Context, req *serving.Request, estimate serving.BillingEstimate) error {
	identity := req.RuntimeIdentity()
	if identity == nil {
		return nil
	}

	tenantMicro := estimate.PlatformCost
	userMicro := estimate.UserCost

	if tenantMicro == 0 {
		if pricing, err := b.pricing.ResolvePricing(ctx, req); err == nil {
			usage := domain.TokenUsage{PromptTokens: b.defaultTokenEstimate}
			billing := b.calculate(usage, pricing)
			tenantMicro = billing.PlatformCost
			userMicro = billing.UserCost
		}
		// If pricing lookup fails, fall through with 0 — URM will still create
		// a transaction but no credits will be held until Confirm.
	}

	if identity.OwnerType == domain.OwnerTenant {
		userMicro = 0
	}

	if tenantMicro == 0 && userMicro == 0 {
		return nil
	}

	resp, err := b.client.Freeze(ctx, urm.FreezeRequest{
		IdempotencyKey: req.RequestID,
		TenantID:       identity.TenantID,
		UserID:         identity.UserID,
		Description:    "ai-gateway: " + req.ModelCode,
		TenantAmount:   domain.MicroToCreditsCeil(tenantMicro),
		UserAmount:     domain.MicroToCreditsCeil(userMicro),
	})
	if err != nil {
		return err
	}
	req.URMTransactionID = resp.EventID
	return nil
}

// Confirm computes the actual billing from real token usage, writes it to
// req.BillingResult (so UsageLogger can skip recomputing), then calls URM confirm.
//
// BillingResult cost fields are in micro-credits; we floor-convert at the URM
// boundary. Sub-1-credit remainders are dropped in Phase 0 (e.g. 0.03 credit
// of consumption deducts 0 credit from URM). Phase 1 (分账层) will carry the
// remainder forward in a local ledger and settle it on aggregation.
func (b *Biller) Confirm(ctx context.Context, req *serving.Request, _ serving.BillingEstimate) error {
	if req.URMTransactionID == "" {
		return nil
	}

	pricing, err := b.pricing.ResolvePricing(ctx, req)
	if err == nil {
		billing := b.calculate(req.TokenUsage, pricing)
		req.BillingResult = billing
		req.BillingResolved = true
	}

	actualTenantMicro := req.BillingResult.PlatformCost
	actualUserMicro := req.BillingResult.UserCost
	if identity := req.RuntimeIdentity(); identity != nil && identity.OwnerType == domain.OwnerTenant {
		actualUserMicro = 0
	}

	_, err = b.client.Confirm(ctx, urm.ConfirmRequest{
		EventID:            req.URMTransactionID,
		ActualTenantAmount: domain.MicroToCreditsFloor(actualTenantMicro),
		ActualUserAmount:   domain.MicroToCreditsFloor(actualUserMicro),
	})
	if err != nil {
		b.logger.Error("urm confirm failed",
			zap.Error(err),
			zap.String("transaction_id", req.URMTransactionID),
			zap.String("request_id", req.RequestID),
		)
	}
	return err
}

// Cancel rolls back the URM freeze on pipeline failure. Best-effort.
func (b *Biller) Cancel(ctx context.Context, req *serving.Request) {
	if req.URMTransactionID == "" {
		return
	}
	if err := b.client.Cancel(ctx, req.URMTransactionID); err != nil {
		b.logger.Warn("urm cancel failed",
			zap.Error(err),
			zap.String("transaction_id", req.URMTransactionID),
			zap.String("request_id", req.RequestID),
		)
	}
}
