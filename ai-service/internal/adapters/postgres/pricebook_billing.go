package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/service/pricebook"
	"xiaodou/unihub/ai-service/internal/serving"
)

// rateCacheTTL bounds how long a fetched credits_per_usd value is reused before
// re-reading ai_settings. Short enough that an admin rate change takes effect
// quickly; long enough to keep the per-request DB load negligible.
const rateCacheTTL = 30 * time.Second

// PriceBookBiller computes per-request micro-credit billing from the unified
// Price Book model:
//
//   - PlatformCostMicro = tenant sell binding (entry × sell_multiplier × rate)
//   - UserCostMicro     = cascade (entry × sell_multiplier × user_multiplier × rate)
//   - APIKeyQuotaCostMicro = the cost charged to the API key's own owner
//
// ProviderCostMicro (upstream USD cost) is wired in P4 once the deployment cost
// binding is loaded onto the route candidate; it is 0 here.
type PriceBookBiller struct {
	svc    *pricebook.Service
	logger *zap.Logger

	mu      sync.RWMutex
	rate    float64
	rateExp time.Time
}

func NewPriceBookBiller(svc *pricebook.Service, logger *zap.Logger) *PriceBookBiller {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PriceBookBiller{svc: svc, logger: logger}
}

var _ serving.SellPriceChecker = (*PriceBookBiller)(nil)

// EnsureSellable is the fail-closed pre-flight check: a tenant must have a sell
// binding and the requested model must have a price book entry, else the request
// is rejected before any upstream call. Returns domain.ErrNotFound when either
// is missing.
func (b *PriceBookBiller) EnsureSellable(ctx context.Context, tenantID, modelCode string) error {
	binding, err := b.svc.GetTenantSellBinding(ctx, tenantID)
	if err != nil {
		return err
	}
	if _, err := b.svc.GetEntry(ctx, binding.PriceBookID, modelCode); err != nil {
		return err
	}
	return nil
}

// Calculate resolves the three cost lines for one completed request. On any
// missing-price condition it logs a warning and returns zeros for the affected
// lines (the fail-closed reject already happened pre-flight in BillingGuardStep;
// reaching here with no price means a race or an unbilled internal call).
func (b *PriceBookBiller) Calculate(ctx context.Context, req *serving.Request) domain.BillingResult {
	id := req.RuntimeIdentity()
	if id == nil || id.TenantID == "" || req.Candidate == nil {
		return domain.BillingResult{}
	}

	units, unitType := billableUnits(req.TokenUsage)
	result := domain.BillingResult{BillableUnits: units, BillableUnitType: unitType}

	rate := b.creditsPerUSD(ctx)

	// Provider (upstream) cost — independent of sell pricing, for profit
	// accounting. Computed first so it is recorded even if sell pricing is
	// missing (which should not happen for billed traffic past BillingGuard).
	result.ProviderCostMicro = b.providerCostMicro(ctx, req, rate)

	binding, err := b.svc.GetTenantSellBinding(ctx, id.TenantID)
	if err != nil {
		b.warnNoPrice("tenant sell binding", id.TenantID, req.ModelCode, err)
		return result
	}
	entry, err := b.svc.GetEntry(ctx, binding.PriceBookID, req.ModelCode)
	if err != nil {
		b.warnNoPrice("price book entry", id.TenantID, req.ModelCode, err)
		return result
	}

	result.PlatformCostMicro = sellMicro(req.TokenUsage, entry, binding.CacheBillingEnabled, binding.SellMultiplier, rate)

	if id.OwnerType == domain.OwnerUser {
		userMult := 1.0
		userCache := false
		if ub, err := b.svc.GetUserSellBinding(ctx, id.TenantID); err == nil {
			userMult = ub.UserMultiplier
			userCache = ub.CacheBillingEnabled
		} else if !errors.Is(err, domain.ErrNotFound) {
			b.logger.Warn("user sell binding lookup failed",
				zap.String("tenant_id", id.TenantID), zap.Error(err))
		}
		result.UserCostMicro = sellMicro(req.TokenUsage, entry, userCache, binding.SellMultiplier*userMult, rate)
	}

	// The API key's local quota is debited in the key owner's own currency:
	// a user key spends the user price; a tenant key spends the platform price.
	if id.OwnerType == domain.OwnerUser {
		result.APIKeyQuotaCostMicro = result.UserCostMicro
	} else {
		result.APIKeyQuotaCostMicro = result.PlatformCostMicro
	}
	return result
}

// providerCostMicro computes the upstream USD cost (in micro-credits via the
// same rate) from the route candidate's cost binding. Returns 0 when the
// deployment has no price book bound. Cache/reasoning prices always apply on the
// cost side (provider charges its own cache rates; 0 falls back to input/output).
func (b *PriceBookBiller) providerCostMicro(ctx context.Context, req *serving.Request, rate float64) int64 {
	c := req.Candidate
	if c == nil || c.PriceBookID == "" || c.UpstreamModel == "" {
		return 0
	}
	entry, err := b.svc.GetEntry(ctx, c.PriceBookID, c.UpstreamModel)
	if err != nil {
		b.warnNoPrice("upstream cost entry", req.RuntimeIdentity().TenantID, c.UpstreamModel, err)
		return 0
	}
	return sellMicro(req.TokenUsage, entry, true, c.CostMultiplier, rate)
}

func (b *PriceBookBiller) warnNoPrice(what, tenantID, modelCode string, err error) {
	b.logger.Warn("billing: "+what+" missing, charging 0",
		zap.String("tenant_id", tenantID),
		zap.String("model_code", modelCode),
		zap.Error(err))
}

// creditsPerUSD returns the cached USD→credit rate, refreshing past the TTL.
func (b *PriceBookBiller) creditsPerUSD(ctx context.Context) float64 {
	b.mu.RLock()
	if time.Now().Before(b.rateExp) && b.rate > 0 {
		r := b.rate
		b.mu.RUnlock()
		return r
	}
	b.mu.RUnlock()

	r, err := b.svc.GetCreditsPerUSD(ctx)
	if err != nil || r <= 0 {
		r = domain.DefaultCreditsPerUSD
	}
	b.mu.Lock()
	b.rate = r
	b.rateExp = time.Now().Add(rateCacheTTL)
	b.mu.Unlock()
	return r
}

// ============================================================================
// Pure billing math (no DB) — extracted for unit testing.
// ============================================================================

// sellMicro converts one usage + price-book entry into micro-credits for one
// billing side, applying the multiplier and USD→credit rate, then flooring.
func sellMicro(u domain.TokenUsage, e domain.PriceBookEntry, cacheEnabled bool, multiplier, creditsPerUSD float64) int64 {
	usd := sellUSD(u, e, cacheEnabled)
	micro := usd * multiplier * creditsPerUSD * float64(domain.MicroCreditsPerCredit)
	if micro <= 0 {
		return 0
	}
	return int64(micro) // floor toward zero
}

// sellUSD computes the raw USD cost of one call. Image/video take precedence
// over token billing (mirrors the capability-by-usage convention); audio is not
// yet wired into TokenUsage and bills 0.
func sellUSD(u domain.TokenUsage, e domain.PriceBookEntry, cacheEnabled bool) float64 {
	if u.ImageCount > 0 {
		return lookupResolutionUSD(e.ImagePrices, u.ImageResolution) * float64(u.ImageCount)
	}
	if u.VideoSeconds > 0 {
		return lookupResolutionUSD(e.VideoPrices, u.VideoResolution) * u.VideoSeconds
	}

	inputPrice := e.InputPerToken
	outputPrice := e.OutputPerToken
	cacheWrite := inputPrice
	cacheRead := inputPrice
	reasoning := outputPrice
	if cacheEnabled {
		cacheWrite = e.EffectiveCacheWritePerToken()
		cacheRead = e.EffectiveCacheReadPerToken()
		reasoning = e.EffectiveReasoningPerToken()
	}

	// Cache tokens ⊆ prompt; reasoning ⊆ completion — split to avoid double count.
	nonCachedIn := u.PromptTokens - u.CacheWriteTokens - u.CacheReadTokens
	if nonCachedIn < 0 {
		nonCachedIn = 0
	}
	nonReasonOut := u.CompletionTokens - u.ReasoningTokens
	if nonReasonOut < 0 {
		nonReasonOut = 0
	}

	return float64(nonCachedIn)*inputPrice +
		float64(u.CacheWriteTokens)*cacheWrite +
		float64(u.CacheReadTokens)*cacheRead +
		float64(nonReasonOut)*outputPrice +
		float64(u.ReasoningTokens)*reasoning
}

// billableUnits reports the headline quantity + unit for the usage row.
func billableUnits(u domain.TokenUsage) (int64, string) {
	if u.ImageCount > 0 {
		return int64(u.ImageCount), "image"
	}
	if u.VideoSeconds > 0 {
		return int64(u.VideoSeconds), "second"
	}
	return int64(u.TotalTokens()), "token"
}

// lookupResolutionUSD returns the price matching resolution, or the lowest price
// in the list when no exact match is found (mirrors UI "from X" rendering).
func lookupResolutionUSD(prices []domain.ResolutionUSDPrice, resolution string) float64 {
	if len(prices) == 0 {
		return 0
	}
	if resolution != "" {
		for _, p := range prices {
			if p.Resolution == resolution {
				return p.Price
			}
		}
	}
	min := prices[0].Price
	for _, p := range prices[1:] {
		if p.Price < min {
			min = p.Price
		}
	}
	return min
}
