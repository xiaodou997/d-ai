package serving

import (
	"context"
	"errors"
	"net/http"

	"xiaodou/unihub/ai-service/internal/domain"
)

// SellPriceChecker verifies that a chargeable request actually has a configured
// sell price (fail-closed billing). Implemented by the postgres PriceBookBiller.
type SellPriceChecker interface {
	// EnsureSellable returns domain.ErrNotFound when the tenant has no sell
	// binding or the model has no price book entry.
	EnsureSellable(ctx context.Context, tenantID, modelCode string) error
}

// BillingGuardStep rejects a request before any upstream call when no outbound
// sell price is configured for (tenant, model). Internal/untenanted calls
// (no TenantID) are not billed and pass through untouched.
type BillingGuardStep struct {
	Checker SellPriceChecker
}

func (s *BillingGuardStep) Name() string { return "billing_guard" }

func (s *BillingGuardStep) Execute(ctx context.Context, req *Request) error {
	if s.Checker == nil {
		return nil
	}
	id := req.RuntimeIdentity()
	if id == nil || id.TenantID == "" {
		return nil // not a billed request
	}
	if err := s.Checker.EnsureSellable(ctx, id.TenantID, req.ModelCode); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return apiError(http.StatusPaymentRequired, "no_price_configured",
				"no sell price configured for this model")
		}
		return apiError(http.StatusInternalServerError, "billing_guard_failed", err.Error())
	}
	return nil
}

func (s *BillingGuardStep) Rollback(_ context.Context, _ *Request) {}
