package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/billingcontrol"
	corebilling "xiaodou/dai/internal/ai/core/billing"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
)

// PriceBookBiller resolves two independent ledgers from the same price-book
// format. Tenant charge uses the winning upstream resource's book and
// multiplier. User retail uses the tenant group's book and effective user
// multiplier. These multipliers are never multiplied together.
type PriceBookBiller struct {
	svc  *billingcontrol.Service
	q    *dbgen.Queries
	pool *translatingPool
}

func NewPriceBookBiller(svc *billingcontrol.Service, q *dbgen.Queries, pool *pgxpool.Pool) *PriceBookBiller {
	return &PriceBookBiller{svc: svc, q: q, pool: newTranslatingPool(pool)}
}

var _ serving.BillingSnapshotResolver = (*PriceBookBiller)(nil)

// EnsureSellable rejects models that have no complete retail and account-side
// pricing path within the tenant's own active groups.
func (b *PriceBookBiller) EnsureSellable(ctx context.Context, tenantID, modelCode, forcedGroupID string) error {
	const q = `
		SELECT EXISTS(
		  SELECT 1
		  FROM ai_groups g
		  JOIN ai_group_targets gt ON gt.group_id = g.id AND gt.status = 'active'
		  JOIN ai_upstream_models um
		    ON um.upstream_kind = gt.target_kind
		   AND um.upstream_id = gt.target_id
		   AND um.model_code = $2
		   AND um.status = 'active'
		  JOIN ai_price_book_entries e
		    ON e.price_book_id = g.retail_price_book_id
		   AND e.model_code = um.model_code
		   AND e.capability_type = um.capability_type
		  LEFT JOIN ai_upstream_accounts a
		    ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		  LEFT JOIN ai_credential_pools cp
		    ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		  JOIN ai_price_book_entries account_entry
		    ON account_entry.price_book_id = COALESCE(a.price_book_id, cp.price_book_id)
		   AND account_entry.model_code = um.model_code
		   AND account_entry.capability_type = um.capability_type
		  WHERE g.status = 'active'
		    AND g.tenant_id = $1
		    AND ($3 = '' OR g.id = $3::uuid)
		    AND (
		      (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		      OR
		      (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		    )
		)`
	var ok bool
	if err := b.pool.QueryRow(ctx, q, tenantID, modelCode, forcedGroupID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

// PrepareBilling resolves price books and all multiplier overrides for one
// candidate before upstream execution.
func (b *PriceBookBiller) PrepareBilling(ctx context.Context, req *serving.Request, cand *domain.RouteCandidate) (domain.BillingSnapshot, error) {
	subject := req.RuntimeSubject()
	if subject == nil || subject.TenantID == "" || cand == nil {
		return domain.BillingSnapshot{}, errors.New("billing subject and candidate are required")
	}
	if cand.RetailPriceBookID == "" || cand.AccountPriceBookID == "" || cand.ModelCode == "" {
		return domain.BillingSnapshot{}, domain.ErrNotFound
	}
	retailEntry, err := b.svc.GetEntry(ctx, cand.RetailPriceBookID, cand.ModelCode, string(cand.CapabilityType))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.BillingSnapshot{}, domain.ErrNotFound
		}
		return domain.BillingSnapshot{}, fmt.Errorf("load retail price entry: %w", err)
	}
	accountEntry, err := b.svc.GetEntry(ctx, cand.AccountPriceBookID, cand.ModelCode, string(cand.CapabilityType))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.BillingSnapshot{}, domain.ErrNotFound
		}
		return domain.BillingSnapshot{}, fmt.Errorf("load account price entry: %w", err)
	}
	if err := validateRuntimeTokenPricing(retailEntry, cand.CapabilityType); err != nil {
		return domain.BillingSnapshot{}, fmt.Errorf("invalid retail price entry: %w", err)
	}
	if err := validateRuntimeTokenPricing(accountEntry, cand.CapabilityType); err != nil {
		return domain.BillingSnapshot{}, fmt.Errorf("invalid account price entry: %w", err)
	}

	groupName := cand.GroupName
	if groupName == "" {
		groupName = cand.GroupID
	}
	snapshot := domain.BillingSnapshot{
		RetailEntry:                retailEntry,
		AccountEntry:               accountEntry,
		GroupName:                  groupName,
		GroupDefaultUserMultiplier: cand.GroupDefaultUserMultiplier,
		EffectiveUserMultiplier:    cand.GroupDefaultUserMultiplier,
		ServiceTier:                billingServiceTier(req),
	}

	if runtimeSubjectOwnerType(subject) == domain.OwnerUser {
		userMultiplier := cand.GroupDefaultUserMultiplier
		if cand.GroupID != "" && subject.UserID != "" {
			userGroup, err := b.q.GetUserGroup(ctx, dbgen.GetUserGroupParams{
				TenantID: subject.TenantID,
				UserID:   subject.UserID,
				GroupID:  mustParseUUID(cand.GroupID),
			})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return domain.BillingSnapshot{}, fmt.Errorf("load user group pricing: %w", err)
			}
			if err == nil && userGroup.UserMultiplierOverride.Valid {
				userMultiplier = numericToFloat(userGroup.UserMultiplierOverride)
				snapshot.UserMultiplierOverride = floatPtr(userMultiplier)
			}
		}
		snapshot.EffectiveUserMultiplier = userMultiplier
	}
	return snapshot, nil
}

func validateRuntimeTokenPricing(entry domain.PriceBookEntry, routedCapability domain.CapabilityType) error {
	if corebilling.IsTokenPricedCapability(string(routedCapability)) {
		return corebilling.ValidateTokenPriceTiers(entry.TokenPriceTiers)
	}
	return nil
}

// Calculate applies actual usage to the immutable snapshot selected for the
// winning route. It performs no database or network reads.
func (b *PriceBookBiller) Calculate(_ context.Context, req *serving.Request) (domain.BillingResult, error) {
	if req == nil || req.Candidate == nil {
		return domain.BillingResult{}, errors.New("billing candidate is required")
	}
	snapshot, ok := req.BillingSnapshots[req.Candidate.RouteID]
	if !ok {
		return domain.BillingResult{}, errors.New("billing snapshot is missing for winning route")
	}
	usage := settlementUsage(req)
	units, unitType := billableUnitsForCapability(usage, req.CapabilityType)
	result := domain.BillingResult{
		BillableUnits: units, BillableUnitType: unitType,
		ServiceTier:       snapshot.ServiceTier,
		GroupNameSnapshot: snapshot.GroupName, GroupDefaultUserMultiplier: snapshot.GroupDefaultUserMultiplier,
		UserMultiplierOverride: snapshot.UserMultiplierOverride, EffectiveUserMultiplier: snapshot.EffectiveUserMultiplier,
	}
	accountBreakdown, err := priceBreakdownForCapability(usage, snapshot.AccountEntry, req.CapabilityType)
	if err != nil {
		return domain.BillingResult{}, fmt.Errorf("select account price tier: %w", err)
	}
	accountBaseLine := costLineFromBreakdown(accountBreakdown, 1)
	tenantChargeLine := costLineFromBreakdown(
		accountBreakdown,
		req.Candidate.TenantMultiplier,
	)
	result.CatalogBaseMicro = accountBaseLine.CostMicro
	result.TenantPayableMicro = tenantChargeLine.CostMicro
	retailBreakdown, err := priceBreakdownForCapability(usage, snapshot.RetailEntry, req.CapabilityType)
	if err != nil {
		return domain.BillingResult{}, fmt.Errorf("select retail price tier: %w", err)
	}
	result.RetailBaseMicro = costLineFromBreakdown(retailBreakdown, 1).CostMicro
	var userLine *billingCostLine
	if runtimeSubjectOwnerType(req.RuntimeSubject()) == domain.OwnerUser {
		line := costLineFromBreakdown(retailBreakdown, snapshot.EffectiveUserMultiplier)
		result.UserPayableMicro = line.CostMicro
		result.UserChargedMicro = line.CostMicro
		if req.BillingSource == subscription.BillingSourceSubscription {
			result.UserChargedMicro = 0
		}
		result.APIKeyQuotaCostMicro = line.CostMicro
		userLine = &line
	} else {
		result.APIKeyQuotaCostMicro = result.TenantPayableMicro
	}
	result.BillingGroupLabel = fmt.Sprintf("%s · %.4gx", result.GroupNameSnapshot, result.EffectiveUserMultiplier)
	result.BillingBreakdownJSON = marshalBillingBreakdown(result, accountBaseLine, tenantChargeLine, userLine)
	return result, nil
}

func floatPtr(value float64) *float64 { return &value }

// ============================================================================
// Pure billing math (no DB) — extracted for unit testing.
// ============================================================================

type priceLineBreakdown struct {
	InputContextTokens     int     `json:"input_context_tokens"`
	TokenPriceTierIndex    *int    `json:"token_price_tier_index,omitempty"`
	TokenPriceTierUpper    *int    `json:"token_price_tier_up_to_input_tokens"`
	InputTokens            int     `json:"input_tokens"`
	OutputTokens           int     `json:"output_tokens"`
	CacheReadTokens        int     `json:"cache_read_tokens"`
	CacheWriteTokens       int     `json:"cache_write_tokens"`
	ReasoningTokens        int     `json:"reasoning_tokens"`
	InputUnitPer1MUSD      float64 `json:"input_unit_price_per_1m_usd"`
	OutputUnitPer1MUSD     float64 `json:"output_unit_price_per_1m_usd"`
	CacheReadUnitPer1MUSD  float64 `json:"cache_read_unit_price_per_1m_usd"`
	CacheWriteUnitPer1MUSD float64 `json:"cache_write_unit_price_per_1m_usd"`
	InputCostUSD           float64 `json:"input_cost_usd"`
	OutputCostUSD          float64 `json:"output_cost_usd"`
	CacheReadCostUSD       float64 `json:"cache_read_cost_usd"`
	CacheWriteCostUSD      float64 `json:"cache_write_cost_usd"`
	TotalUSD               float64 `json:"total_usd"`
	ImageCount             int     `json:"image_count,omitempty"`
	ImageResolution        string  `json:"image_resolution,omitempty"`
	ImageUnitPriceUSD      float64 `json:"image_unit_price_usd,omitempty"`
	ImageCostUSD           float64 `json:"image_cost_usd,omitempty"`
	VideoSeconds           float64 `json:"video_seconds,omitempty"`
	VideoResolution        string  `json:"video_resolution,omitempty"`
	VideoUnitPriceUSD      float64 `json:"video_unit_price_usd,omitempty"`
	VideoCostUSD           float64 `json:"video_cost_usd,omitempty"`
	totalExact             *big.Rat
}

type billingCostLine struct {
	RawUSD              float64            `json:"raw_usd"`
	AppliedMultiplier   float64            `json:"applied_multiplier"`
	ChargeUSDEquivalent float64            `json:"charge_usd_equivalent"`
	CostMicro           int64              `json:"cost_micro"`
	PriceLines          priceLineBreakdown `json:"price_lines,omitempty"`
}

type billingBreakdownSnapshot struct {
	Version          int                `json:"version"`
	ServiceTier      domain.ServiceTier `json:"service_tier"`
	CatalogBase      billingCostLine    `json:"catalog_base"`
	TenantPayable    billingCostLine    `json:"tenant_payable"`
	UserPayable      *billingCostLine   `json:"user_payable,omitempty"`
	UserChargedMicro int64              `json:"user_charged_micro"`
	PriceLines       priceLineBreakdown `json:"price_lines,omitempty"`
}

// pricedMicro converts one usage + price-book entry into micro-USD for one
// billing side and performs one final round-half-up operation.
func pricedMicro(u domain.TokenUsage, e domain.PriceBookEntry, multiplier float64) int64 {
	breakdown, err := priceBreakdown(u, e)
	if err != nil {
		return 0
	}
	return costLineFromBreakdown(
		breakdown,
		multiplier,
	).CostMicro
}

// priceUSD computes the raw USD cost of one call.
func priceUSD(u domain.TokenUsage, e domain.PriceBookEntry) float64 {
	breakdown, err := priceBreakdown(u, e)
	if err != nil {
		return 0
	}
	return breakdown.TotalUSD
}

func priceBreakdown(u domain.TokenUsage, e domain.PriceBookEntry) (priceLineBreakdown, error) {
	if u.ImageCount > 0 {
		price := lookupImageUSD(e.ImagePrices, e.ImageDefaultPrice, u.ImageResolution)
		exact := decimalProduct(domain.DecimalRat(price), new(big.Rat).SetInt64(int64(u.ImageCount)))
		total, _ := exact.Float64()
		return priceLineBreakdown{
			TotalUSD:          total,
			ImageCount:        u.ImageCount,
			ImageResolution:   u.ImageResolution,
			ImageUnitPriceUSD: price,
			ImageCostUSD:      total,
			totalExact:        exact,
		}, nil
	}
	if u.VideoSeconds > 0 {
		price := lookupResolutionUSD(e.VideoPrices, e.VideoDefaultPrice, u.VideoResolution)
		exact := decimalProduct(domain.DecimalRat(price), domain.DecimalRat(u.VideoSeconds))
		total, _ := exact.Float64()
		return priceLineBreakdown{
			TotalUSD:          total,
			VideoSeconds:      u.VideoSeconds,
			VideoResolution:   u.VideoResolution,
			VideoUnitPriceUSD: price,
			VideoCostUSD:      total,
			totalExact:        exact,
		}, nil
	}

	tier, tierIndex, err := corebilling.SelectTokenPriceTier(e.TokenPriceTiers, u.PromptTokens)
	if err != nil {
		return priceLineBreakdown{}, err
	}
	inputPrice := tier.InputPerToken
	outputPrice := tier.OutputPerToken
	cacheWrite := tier.CacheWritePerToken
	cacheRead := tier.CacheReadPerToken

	nonCachedIn := u.PromptTokens - u.CacheWriteTokens - u.CacheReadTokens
	cacheWriteTokens := u.CacheWriteTokens
	cacheReadTokens := u.CacheReadTokens
	if nonCachedIn < 0 {
		return priceLineBreakdown{}, fmt.Errorf(
			"cache tokens exceed prompt tokens: prompt=%d cache_write=%d cache_read=%d",
			u.PromptTokens,
			u.CacheWriteTokens,
			u.CacheReadTokens,
		)
	}

	b := priceLineBreakdown{
		InputContextTokens:     u.PromptTokens,
		TokenPriceTierIndex:    &tierIndex,
		TokenPriceTierUpper:    tier.UpToInputTokens,
		InputTokens:            nonCachedIn,
		OutputTokens:           u.CompletionTokens,
		CacheReadTokens:        cacheReadTokens,
		CacheWriteTokens:       cacheWriteTokens,
		ReasoningTokens:        u.ReasoningTokens,
		InputUnitPer1MUSD:      inputPrice * 1_000_000,
		OutputUnitPer1MUSD:     outputPrice * 1_000_000,
		CacheReadUnitPer1MUSD:  cacheRead * 1_000_000,
		CacheWriteUnitPer1MUSD: cacheWrite * 1_000_000,
		InputCostUSD:           float64(nonCachedIn) * inputPrice,
		OutputCostUSD:          float64(u.CompletionTokens) * outputPrice,
		CacheReadCostUSD:       float64(cacheReadTokens) * cacheRead,
		CacheWriteCostUSD:      float64(cacheWriteTokens) * cacheWrite,
	}
	b.totalExact = new(big.Rat)
	addDecimalTokenCost(b.totalExact, nonCachedIn, inputPrice)
	addDecimalTokenCost(b.totalExact, u.CompletionTokens, outputPrice)
	addDecimalTokenCost(b.totalExact, cacheReadTokens, cacheRead)
	addDecimalTokenCost(b.totalExact, cacheWriteTokens, cacheWrite)
	b.TotalUSD, _ = b.totalExact.Float64()
	return b, nil
}

func priceBreakdownForCapability(u domain.TokenUsage, e domain.PriceBookEntry, capability domain.CapabilityType) (priceLineBreakdown, error) {
	switch capability {
	case domain.CapabilityImage:
		price := lookupImageUSD(e.ImagePrices, e.ImageDefaultPrice, u.ImageResolution)
		exact := decimalProduct(domain.DecimalRat(price), new(big.Rat).SetInt64(int64(u.ImageCount)))
		total, _ := exact.Float64()
		return priceLineBreakdown{
			TotalUSD:          total,
			ImageCount:        u.ImageCount,
			ImageResolution:   u.ImageResolution,
			ImageUnitPriceUSD: price,
			ImageCostUSD:      total,
			totalExact:        exact,
		}, nil
	case domain.CapabilityVideo:
		price := lookupResolutionUSD(e.VideoPrices, e.VideoDefaultPrice, u.VideoResolution)
		exact := decimalProduct(domain.DecimalRat(price), domain.DecimalRat(u.VideoSeconds))
		total, _ := exact.Float64()
		return priceLineBreakdown{
			TotalUSD:          total,
			VideoSeconds:      u.VideoSeconds,
			VideoResolution:   u.VideoResolution,
			VideoUnitPriceUSD: price,
			VideoCostUSD:      total,
			totalExact:        exact,
		}, nil
	default:
		return priceBreakdown(u, e)
	}
}

func costLineFromBreakdown(b priceLineBreakdown, appliedMultiplier float64) billingCostLine {
	if appliedMultiplier < 0 {
		appliedMultiplier = 1
	}
	totalExact := b.totalExact
	if totalExact == nil {
		totalExact = domain.DecimalRat(b.TotalUSD)
	}
	microExact := new(big.Rat).Set(totalExact)
	microExact.Mul(microExact, domain.DecimalRat(appliedMultiplier))
	microExact.Mul(microExact, new(big.Rat).SetInt64(domain.MicroUSDPerUSD))
	chargeUSD := b.TotalUSD * appliedMultiplier
	return billingCostLine{
		RawUSD:              b.TotalUSD,
		AppliedMultiplier:   appliedMultiplier,
		ChargeUSDEquivalent: chargeUSD,
		CostMicro:           domain.RoundHalfUpRatToInt64(microExact),
		PriceLines:          b,
	}
}

func decimalProduct(values ...*big.Rat) *big.Rat {
	product := new(big.Rat).SetInt64(1)
	for _, value := range values {
		if value == nil {
			return new(big.Rat)
		}
		product.Mul(product, value)
	}
	return product
}

func addDecimalTokenCost(total *big.Rat, tokens int, price float64) {
	if total == nil || tokens <= 0 || price <= 0 {
		return
	}
	total.Add(total, decimalProduct(
		new(big.Rat).SetInt64(int64(tokens)),
		domain.DecimalRat(price),
	))
}

func billingServiceTier(req *serving.Request) domain.ServiceTier {
	if req != nil && req.ServiceTier != "" {
		return req.ServiceTier
	}
	return domain.ServiceTierStandard
}

func marshalBillingBreakdown(
	result domain.BillingResult,
	accountBase, tenantCharge billingCostLine,
	userRetail *billingCostLine,
) []byte {
	priceLines := priceLineBreakdown{}
	if userRetail != nil {
		priceLines = userRetail.PriceLines
	} else {
		priceLines = accountBase.PriceLines
	}
	raw, err := json.Marshal(billingBreakdownSnapshot{
		// Version 5 drops the platform-payable line: platform cost is no longer
		// tracked in this system. Replay tooling keys off this.
		Version:          5,
		ServiceTier:      result.ServiceTier,
		CatalogBase:      accountBase,
		TenantPayable:    tenantCharge,
		UserPayable:      userRetail,
		UserChargedMicro: result.UserChargedMicro,
		PriceLines:       priceLines,
	})
	if err != nil {
		return nil
	}
	return raw
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

func billableUnitsForCapability(u domain.TokenUsage, capability domain.CapabilityType) (int64, string) {
	switch capability {
	case domain.CapabilityImage:
		return int64(u.ImageCount), "image"
	case domain.CapabilityVideo:
		return int64(u.VideoSeconds), "second"
	default:
		return billableUnits(u)
	}
}

func settlementUsage(req *serving.Request) domain.TokenUsage {
	usage := req.TokenUsage
	// Provider usage payloads occasionally report overlapping cache counters.
	// Keep one deterministic snapshot for billing and usage records: cache-read
	// wins, cache-write consumes the remaining prompt tokens, and negatives are
	// treated as absent. The raw priceBreakdown helper remains strict for config
	// validation; runtime settlement is the normalization boundary.
	if usage.PromptTokens < 0 {
		usage.PromptTokens = 0
	}
	if usage.CacheReadTokens < 0 {
		usage.CacheReadTokens = 0
	}
	if usage.CacheWriteTokens < 0 {
		usage.CacheWriteTokens = 0
	}
	if usage.CacheReadTokens > usage.PromptTokens {
		usage.CacheReadTokens = usage.PromptTokens
	}
	remaining := usage.PromptTokens - usage.CacheReadTokens
	if usage.CacheWriteTokens > remaining {
		usage.CacheWriteTokens = remaining
	}
	if req.RequestStatus != domain.RequestFailed {
		req.TokenUsage = usage
		return usage
	}
	switch req.CapabilityType {
	case domain.CapabilityImage:
		usage.ImageCount = 0
	case domain.CapabilityVideo:
		usage.VideoSeconds = 0
	}
	req.TokenUsage = usage
	return usage
}

// lookupImageUSD classifies OpenAI Image 2 sizes into fixed price tiers.
// Unrecognised sizes use the explicit default price.
func lookupImageUSD(prices []domain.ResolutionUSDPrice, defaultPrice float64, size string) float64 {
	tier, ok := corebilling.ClassifyOpenAIImagePriceTier(size)
	if !ok {
		return defaultPrice
	}
	return lookupResolutionUSD(prices, defaultPrice, tier)
}

// lookupResolutionUSD returns the exact price when present; otherwise it falls
// back to the explicit default price. Video pricing continues to use literal
// resolution matching.
func lookupResolutionUSD(prices []domain.ResolutionUSDPrice, defaultPrice float64, resolution string) float64 {
	if resolution != "" {
		for _, p := range prices {
			if p.Resolution == resolution {
				return p.Price
			}
		}
	}
	return defaultPrice
}
