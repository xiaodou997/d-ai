package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/billingcontrol"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

type billingModelRepository struct {
	billingcontrol.Repository
}

func (*billingModelRepository) GetEntry(_ context.Context, priceBookID, modelCode, capabilityType string) (domain.PriceBookEntry, error) {
	entry := chatEntry()
	entry.PriceBookID = priceBookID
	entry.ModelCode = modelCode
	entry.CapabilityType = capabilityType
	switch {
	case priceBookID == "sell-book" && modelCode == "gpt-5.4":
		return entry, nil
	case priceBookID == "cost-book" && modelCode == "gpt-5.4":
		return entry, nil
	default:
		return domain.PriceBookEntry{}, domain.ErrNotFound
	}
}

func chatEntry() domain.PriceBookEntry {
	return domain.PriceBookEntry{
		TokenPriceTiers: []domain.TokenPriceTier{{
			InputPerToken:     0.000003, // $3 / 1M
			OutputPerToken:    0.000006, // $6 / 1M
			CacheReadPerToken: 0.0000003,
		}},
	}
}

func mustPriceBreakdown(t *testing.T, usage domain.TokenUsage, entry domain.PriceBookEntry) priceLineBreakdown {
	t.Helper()
	breakdown, err := priceBreakdown(usage, entry)
	if err != nil {
		t.Fatalf("price breakdown: %v", err)
	}
	return breakdown
}

func TestPrepareBillingUsesPublicModelCodeForBothLedgers(t *testing.T) {
	svc := billingcontrol.New(&billingModelRepository{}, nil)
	biller := NewPriceBookBiller(svc, nil, nil)
	candidate := &domain.RouteCandidate{
		RouteID:                    "route-1",
		RequestedModel:             "claude-opus-4-1",
		ModelCode:                  "gpt-5.4",
		UpstreamModel:              "gpt-5.4-upstream",
		CapabilityType:             domain.CapabilityChat,
		RetailPriceBookID:          "sell-book",
		AccountPriceBookID:         "cost-book",
		GroupDefaultUserMultiplier: 1,
		TenantMultiplier:           1,
	}
	req := &serving.Request{
		Subject: &coreidentity.Subject{Scope: coreidentity.ScopeTenant, TenantID: "tenant-1"},
	}

	snapshot, err := biller.PrepareBilling(context.Background(), req, candidate)
	if err != nil {
		t.Fatalf("prepare billing: %v", err)
	}
	if snapshot.RetailEntry.ModelCode != "gpt-5.4" {
		t.Fatalf("retail model = %q, want gpt-5.4", snapshot.RetailEntry.ModelCode)
	}
	if snapshot.AccountEntry.ModelCode != "gpt-5.4" {
		t.Fatalf("account model = %q, want gpt-5.4", snapshot.AccountEntry.ModelCode)
	}
}

func TestSellMicro_TokensWithoutCache(t *testing.T) {
	// USD = 1000*3e-6 + 500*6e-6 = 0.006; micro-USD = 6000.
	u := domain.TokenUsage{PromptTokens: 1000, CompletionTokens: 500}
	got := pricedMicro(u, chatEntry(), 1)
	if got != 6000 {
		t.Fatalf("got %d, want 6000", got)
	}
}

func TestSellMicro_AlwaysSplitsCacheTokens(t *testing.T) {
	// Cache usage always follows the price-book cache rates.
	// usd = 600*3e-6 + 400*3e-7 + 200*6e-6 = 0.00312
	// micro-USD = 3120.
	u := domain.TokenUsage{PromptTokens: 1000, CacheReadTokens: 400, CompletionTokens: 200, ReasoningTokens: 50}
	got := pricedMicro(u, chatEntry(), 1)
	if got != 3120 {
		t.Fatalf("got %d, want 3120", got)
	}
}

func TestPriceBreakdownRejectsCacheTokensExceedingPrompt(t *testing.T) {
	_, err := priceBreakdown(domain.TokenUsage{
		PromptTokens:    100,
		CacheReadTokens: 101,
	}, chatEntry())
	if err == nil {
		t.Fatal("expected invalid cache token usage to fail")
	}
}

func TestCalculateNormalizesOverlappingCacheUsageOnce(t *testing.T) {
	candidate := &domain.RouteCandidate{RouteID: "route-cache", TenantMultiplier: 1, GroupDefaultUserMultiplier: 1}
	req := &serving.Request{
		Subject:    &coreidentity.Subject{Scope: coreidentity.ScopeTenant, TenantID: "tenant-cache"},
		Candidate:  candidate,
		TokenUsage: domain.TokenUsage{PromptTokens: 100, CompletionTokens: 10, CacheReadTokens: 80, CacheWriteTokens: 50},
		BillingSnapshots: map[string]domain.BillingSnapshot{candidate.RouteID: {RetailEntry: chatEntry(), AccountEntry: chatEntry(),
			GroupDefaultUserMultiplier: 1, EffectiveUserMultiplier: 1,
		}},
	}
	if _, err := (&PriceBookBiller{}).Calculate(context.Background(), req); err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if req.TokenUsage.CacheReadTokens != 80 || req.TokenUsage.CacheWriteTokens != 20 {
		t.Fatalf("normalized cache usage = read %d/write %d, want 80/20", req.TokenUsage.CacheReadTokens, req.TokenUsage.CacheWriteTokens)
	}
}

func TestSellMicro_ExplicitZeroCachePricesAreFree(t *testing.T) {
	e := domain.PriceBookEntry{TokenPriceTiers: []domain.TokenPriceTier{{InputPerToken: 0.000003, OutputPerToken: 0.000006}}}
	u := domain.TokenUsage{PromptTokens: 1000, CacheReadTokens: 400, CompletionTokens: 200, ReasoningTokens: 50}
	if got := pricedMicro(u, e, 1); got != 3000 {
		t.Fatalf("got %d, want 3000", got)
	}
}

func TestSellMicro_Multiplier(t *testing.T) {
	u := domain.TokenUsage{PromptTokens: 1000, CompletionTokens: 500}
	full := pricedMicro(u, chatEntry(), 1)
	half := pricedMicro(u, chatEntry(), 0.5)
	if half != full/2 {
		t.Fatalf("half=%d, want %d", half, full/2)
	}
}

func TestCostLineAppliesOnlyCommercialMultiplier(t *testing.T) {
	u := domain.TokenUsage{PromptTokens: 1000, CacheReadTokens: 400, CompletionTokens: 200, ReasoningTokens: 50}
	breakdown := mustPriceBreakdown(t, u, chatEntry())

	base := costLineFromBreakdown(breakdown, 1)
	doubled := costLineFromBreakdown(breakdown, 2)

	if base.CostMicro != 3120 || doubled.CostMicro != 6240 {
		t.Fatalf("base/doubled=%d/%d, want 3120/6240", base.CostMicro, doubled.CostMicro)
	}
	if doubled.ChargeUSDEquivalent != base.ChargeUSDEquivalent*2 {
		t.Fatalf("charge usd=%f, want %f", doubled.ChargeUSDEquivalent, base.ChargeUSDEquivalent*2)
	}
}

func TestMarshalBillingBreakdown_ServiceTier(t *testing.T) {
	line := costLineFromBreakdown(mustPriceBreakdown(t, domain.TokenUsage{PromptTokens: 1000}, chatEntry()), 1)
	raw := marshalBillingBreakdown(domain.BillingResult{
		ServiceTier: domain.ServiceTierFast,
	}, line, line, &line)

	var got struct {
		ServiceTier string          `json:"service_tier"`
		CatalogBase billingCostLine `json:"catalog_base"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal breakdown: %v", err)
	}
	if got.ServiceTier != "fast" {
		t.Fatalf("service tier = %s, want fast", got.ServiceTier)
	}
	if strings.Contains(string(raw), "service_tier_multiplier") {
		t.Fatalf("billing breakdown must not contain service tier pricing: %s", raw)
	}
	if got.CatalogBase.CostMicro != line.CostMicro {
		t.Fatalf("upstream reference cost = %d, want %d", got.CatalogBase.CostMicro, line.CostMicro)
	}
}

// RetailBaseMicro uses multiplier 1 and is independent from either commercial multiplier.
func TestBaseCostUsesMultiplierOne(t *testing.T) {
	u := domain.TokenUsage{PromptTokens: 1000, CompletionTokens: 500}
	breakdown := mustPriceBreakdown(t, u, chatEntry())

	base := costLineFromBreakdown(breakdown, 1).CostMicro
	if base != 6000 {
		t.Fatalf("base=%d, want 6000", base)
	}
	charge := costLineFromBreakdown(breakdown, 3).CostMicro
	if charge != base*3 {
		t.Fatalf("charge=%d, want base*3=%d", charge, base*3)
	}
}

func TestCalculateUsesPreparedSnapshotWithoutRuntimeDependencies(t *testing.T) {
	candidate := &domain.RouteCandidate{
		RouteID:                    "route-1",
		TenantMultiplier:           0.5,
		GroupDefaultUserMultiplier: 3,
	}
	req := &serving.Request{
		Subject:    &coreidentity.Subject{Scope: coreidentity.ScopeUser, TenantID: "tenant-1", UserID: "user-1"},
		Candidate:  candidate,
		TokenUsage: domain.TokenUsage{PromptTokens: 1000, CompletionTokens: 500},
		BillingSnapshots: map[string]domain.BillingSnapshot{
			candidate.RouteID: {
				RetailEntry:                chatEntry(),
				AccountEntry:               chatEntry(),
				GroupName:                  "standard",
				GroupDefaultUserMultiplier: 3,
				EffectiveUserMultiplier:    1.5,
				ServiceTier:                domain.ServiceTierStandard,
			},
		},
	}

	// A zero-value biller has no DB, price-book service, or logger. Successful
	// calculation proves the completion path only consumes the prepared snapshot.
	result, err := (&PriceBookBiller{}).Calculate(context.Background(), req)
	if err != nil {
		t.Fatalf("calculate prepared snapshot: %v", err)
	}
	if result.RetailBaseMicro != 6000 || result.CatalogBaseMicro != 6000 {
		t.Fatalf("retail/upstream reference = %d/%d, want 6000/6000", result.RetailBaseMicro, result.CatalogBaseMicro)
	}
	if result.TenantPayableMicro != 3000 || result.UserPayableMicro != 9000 || result.UserChargedMicro != 9000 || result.APIKeyQuotaCostMicro != 9000 {
		t.Fatalf("tenant/user/debit/quota = %d/%d/%d/%d, want 3000/9000/9000/9000",
			result.TenantPayableMicro, result.UserPayableMicro, result.UserChargedMicro, result.APIKeyQuotaCostMicro)
	}
}

func TestCalculateRejectsMissingPreparedSnapshot(t *testing.T) {
	req := &serving.Request{Candidate: &domain.RouteCandidate{RouteID: "route-1"}}
	if _, err := (&PriceBookBiller{}).Calculate(context.Background(), req); err == nil {
		t.Fatal("expected missing billing snapshot to fail closed")
	}
}

func TestValidateRuntimeTokenPricingUsesRoutedCapability(t *testing.T) {
	mislabeled := domain.PriceBookEntry{CapabilityType: "image"}
	if err := validateRuntimeTokenPricing(mislabeled, domain.CapabilityChat); err == nil {
		t.Fatal("chat route with missing token tiers must fail before upstream execution")
	}
	if err := validateRuntimeTokenPricing(mislabeled, domain.CapabilityImage); err != nil {
		t.Fatalf("image route should not require token tiers: %v", err)
	}
}

func TestCalculateSelectsAccountAndRetailTiersIndependently(t *testing.T) {
	accountLimit := 1000
	retailLimit := 2000
	accountEntry := domain.PriceBookEntry{TokenPriceTiers: []domain.TokenPriceTier{
		{UpToInputTokens: &accountLimit, InputPerToken: 0.000001, OutputPerToken: 0.000002},
		{InputPerToken: 0.000010, OutputPerToken: 0.000020},
	}}
	retailEntry := domain.PriceBookEntry{TokenPriceTiers: []domain.TokenPriceTier{
		{UpToInputTokens: &retailLimit, InputPerToken: 0.000003, OutputPerToken: 0.000004},
		{InputPerToken: 0.000030, OutputPerToken: 0.000040},
	}}
	candidate := &domain.RouteCandidate{RouteID: "route-1", TenantMultiplier: 1, GroupDefaultUserMultiplier: 1}
	req := &serving.Request{
		Subject:    &coreidentity.Subject{Scope: coreidentity.ScopeUser, TenantID: "tenant-1", UserID: "user-1"},
		Candidate:  candidate,
		TokenUsage: domain.TokenUsage{PromptTokens: 1500, CompletionTokens: 100},
		BillingSnapshots: map[string]domain.BillingSnapshot{candidate.RouteID: {RetailEntry: retailEntry, AccountEntry: accountEntry,
			GroupDefaultUserMultiplier: 1, EffectiveUserMultiplier: 1,
			ServiceTier: domain.ServiceTierStandard,
		}},
	}
	result, err := (&PriceBookBiller{}).Calculate(context.Background(), req)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	var breakdown billingBreakdownSnapshot
	if err := json.Unmarshal(result.BillingBreakdownJSON, &breakdown); err != nil {
		t.Fatalf("unmarshal breakdown: %v", err)
	}
	if breakdown.Version != 5 || breakdown.CatalogBase.PriceLines.TokenPriceTierIndex == nil || *breakdown.CatalogBase.PriceLines.TokenPriceTierIndex != 1 {
		t.Fatalf("account breakdown = %#v", breakdown.CatalogBase.PriceLines)
	}
	if breakdown.TenantPayable.PriceLines.TokenPriceTierIndex == nil || *breakdown.TenantPayable.PriceLines.TokenPriceTierIndex != 1 {
		t.Fatalf("tenant charge breakdown = %#v", breakdown.TenantPayable)
	}
	if breakdown.CatalogBase.PriceLines.InputContextTokens != 1500 || breakdown.UserPayable == nil || breakdown.UserPayable.PriceLines.TokenPriceTierUpper == nil || *breakdown.UserPayable.PriceLines.TokenPriceTierUpper != 2000 {
		t.Fatalf("tier context metadata missing: %#v", breakdown)
	}
}

func TestSellMicro_Image(t *testing.T) {
	e := domain.PriceBookEntry{ImageDefaultPrice: 0.01, ImagePrices: []domain.ResolutionUSDPrice{
		{Resolution: "1k", Price: 0.02},
		{Resolution: "2k", Price: 0.04},
		{Resolution: "4k", Price: 0.08},
	}}
	u := domain.TokenUsage{ImageCount: 2, ImageResolution: "1536x1024"}
	// USD = 0.04*2 = 0.08; micro-USD = 80000.
	if got := pricedMicro(u, e, 1); got != 80000 {
		t.Fatalf("got %d, want 80000", got)
	}
	// unknown resolution → explicit default price (0.01)
	u2 := domain.TokenUsage{ImageCount: 1, ImageResolution: "not-a-size"}
	if got := pricedMicro(u2, e, 1); got != 10000 {
		t.Fatalf("fallback got %d, want 10000", got)
	}
	if got := pricedMicro(domain.TokenUsage{ImageCount: 1, ImageResolution: "auto"}, e, 1); got != 80000 {
		t.Fatalf("auto got %d, want 80000", got)
	}
}

func TestPriceBreakdownImageOmitsTokenTierMetadata(t *testing.T) {
	breakdown := mustPriceBreakdown(t, domain.TokenUsage{
		ImageCount:      1,
		ImageResolution: "1024x1024",
	}, domain.PriceBookEntry{ImageDefaultPrice: 0.01})
	raw, err := json.Marshal(breakdown)
	if err != nil {
		t.Fatalf("marshal image breakdown: %v", err)
	}
	if strings.Contains(string(raw), "token_price_tier_index") {
		t.Fatalf("image breakdown contains token tier index: %s", raw)
	}
}

func TestPriceBreakdownTerminalTierRecordsNullUpperBound(t *testing.T) {
	breakdown := mustPriceBreakdown(t, domain.TokenUsage{PromptTokens: 200_001}, domain.PriceBookEntry{
		TokenPriceTiers: []domain.TokenPriceTier{{}},
	})
	raw, err := json.Marshal(breakdown)
	if err != nil {
		t.Fatalf("marshal token breakdown: %v", err)
	}
	if !strings.Contains(string(raw), `"token_price_tier_up_to_input_tokens":null`) {
		t.Fatalf("terminal tier upper bound is not recorded as null: %s", raw)
	}
}

func TestSellMicro_RoundsHalfUp(t *testing.T) {
	// Three micro-USD is represented exactly.
	e := domain.PriceBookEntry{TokenPriceTiers: []domain.TokenPriceTier{{InputPerToken: 0.000003}}}
	u := domain.TokenUsage{PromptTokens: 1}
	if got := pricedMicro(u, e, 1); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	// Half a micro-USD rounds up.
	e.TokenPriceTiers[0].InputPerToken = 0.0000005
	if got := pricedMicro(u, e, 1); got != 1 {
		t.Fatalf("half-up got %d, want 1", got)
	}
}

func TestBillableUnits(t *testing.T) {
	if n, typ := billableUnits(domain.TokenUsage{PromptTokens: 10, CompletionTokens: 5}); n != 15 || typ != "token" {
		t.Fatalf("token: got %d/%s", n, typ)
	}
	if n, typ := billableUnits(domain.TokenUsage{ImageCount: 3}); n != 3 || typ != "image" {
		t.Fatalf("image: got %d/%s", n, typ)
	}
	if n, typ := billableUnits(domain.TokenUsage{VideoSeconds: 12.7}); n != 12 || typ != "second" {
		t.Fatalf("video: got %d/%s", n, typ)
	}
}

// 失败请求的计费口径（已确认的业务规则，改动前请先确认是有意为之）：
//   - 已调用到上游 → 按上游返回的 token 照常计费，上游收了我们就收；
//   - 完全没调用到上游（attempts == 0）→ 全额置零；
//   - 图片/视频这类「交付物」计量在失败时置零，与 token 不同。
//
// 以下三个测试锁住这三条。
//
// A request that reached an upstream is billed for the tokens that upstream
// reported, even when the request ultimately failed: the cost was really
// incurred and the platform does not absorb it.
func TestFailedRequestStillBillsUpstreamTokens(t *testing.T) {
	req := &serving.Request{
		RequestStatus: domain.RequestFailed,
		TokenUsage:    domain.TokenUsage{PromptTokens: 100, CompletionTokens: 40},
	}
	usage := settlementUsage(req)
	if usage.PromptTokens != 100 || usage.CompletionTokens != 40 {
		t.Fatalf("failed request usage = %+v, want tokens preserved", usage)
	}
}

// Delivered-asset units are different: an image that was never produced is not
// a billable unit the way consumed tokens are.
func TestFailedRequestDropsUndeliveredAssetUnits(t *testing.T) {
	for _, tc := range []struct {
		name       string
		capability domain.CapabilityType
		usage      domain.TokenUsage
		check      func(domain.TokenUsage) bool
	}{
		{
			name: "image", capability: domain.CapabilityImage,
			usage: domain.TokenUsage{ImageCount: 3},
			check: func(u domain.TokenUsage) bool { return u.ImageCount == 0 },
		},
		{
			name: "video", capability: domain.CapabilityVideo,
			usage: domain.TokenUsage{VideoSeconds: 12},
			check: func(u domain.TokenUsage) bool { return u.VideoSeconds == 0 },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &serving.Request{
				RequestStatus:  domain.RequestFailed,
				CapabilityType: tc.capability,
				TokenUsage:     tc.usage,
			}
			if got := settlementUsage(req); !tc.check(got) {
				t.Fatalf("failed %s request usage = %+v, want asset units zeroed", tc.name, got)
			}
		})
	}
}

// Never reaching an upstream costs nothing at all.
func TestUnattemptedRequestIsFree(t *testing.T) {
	zeroed := unattemptedBilling(domain.BillingResult{
		CatalogBaseMicro: 5, TenantPayableMicro: 7, RetailBaseMicro: 6,
		UserPayableMicro: 9, UserChargedMicro: 9, APIKeyQuotaCostMicro: 11,
		BillableUnits: 30,
	})
	if zeroed.TenantPayableMicro != 0 || zeroed.UserChargedMicro != 0 ||
		zeroed.APIKeyQuotaCostMicro != 0 || zeroed.BillableUnits != 0 ||
		zeroed.CatalogBaseMicro != 0 || zeroed.RetailBaseMicro != 0 || zeroed.UserPayableMicro != 0 {
		t.Fatalf("unattempted billing = %+v, want all amounts zero", zeroed)
	}
}
