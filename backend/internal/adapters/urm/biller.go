// Package urm implements the serving.URMBiller interface using the URM settlement API.
package urm

import (
	"context"
	"log/slog"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/serving"
	"uni-ai-api/backend/internal/urm"
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
	logger               *slog.Logger
}

func NewBiller(
	client urmClient,
	pricing PricingResolver,
	calculate BillingCalculator,
	defaultTokenEstimate int,
	logger *slog.Logger,
) *Biller {
	if defaultTokenEstimate <= 0 {
		defaultTokenEstimate = 4096
	}
	if logger == nil {
		logger = slog.Default()
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
func (b *Biller) Freeze(ctx context.Context, req *serving.Request, estimate serving.BillingEstimate) error {
	if req.APIKey == nil {
		return nil
	}

	tenantAmount := estimate.PlatformCost
	userAmount := estimate.UserCost

	if tenantAmount == 0 {
		if pricing, err := b.pricing.ResolvePricing(ctx, req); err == nil {
			usage := domain.TokenUsage{PromptTokens: b.defaultTokenEstimate}
			billing := b.calculate(usage, pricing)
			tenantAmount = billing.PlatformCost
			userAmount = billing.UserCost
		}
		// If pricing lookup fails, fall through with 0 — URM will still create
		// a transaction but no credits will be held until Confirm.
	}

	if req.APIKey.OwnerType == domain.OwnerTenant {
		userAmount = 0
	}

	if tenantAmount == 0 && userAmount == 0 {
		return nil
	}

	resp, err := b.client.Freeze(ctx, urm.FreezeRequest{
		IdempotencyKey: req.RequestID,
		TenantID:       req.APIKey.TenantID,
		UserID:         req.APIKey.UserID,
		Description:    "ai-gateway: " + req.ModelCode,
		TenantAmount:   tenantAmount,
		UserAmount:     userAmount,
	})
	if err != nil {
		return err
	}
	req.URMTransactionID = resp.EventID
	return nil
}

// Confirm computes the actual billing from real token usage, writes it to
// req.BillingResult (so UsageLogger can skip recomputing), then calls URM confirm.
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

	actualTenant := req.BillingResult.PlatformCost
	actualUser := req.BillingResult.UserCost
	if req.APIKey != nil && req.APIKey.OwnerType == domain.OwnerTenant {
		actualUser = 0
	}

	_, err = b.client.Confirm(ctx, urm.ConfirmRequest{
		EventID:            req.URMTransactionID,
		ActualTenantAmount: actualTenant,
		ActualUserAmount:   actualUser,
	})
	if err != nil {
		b.logger.Error("urm confirm failed",
			"error", err,
			"transaction_id", req.URMTransactionID,
			"request_id", req.RequestID,
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
			"error", err,
			"transaction_id", req.URMTransactionID,
			"request_id", req.RequestID,
		)
	}
}
