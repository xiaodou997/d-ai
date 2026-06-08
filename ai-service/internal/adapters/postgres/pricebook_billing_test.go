package postgres

import (
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

func chatEntry() domain.PriceBookEntry {
	return domain.PriceBookEntry{
		InputPerToken:     0.000003, // $3 / 1M
		OutputPerToken:    0.000006, // $6 / 1M
		CacheReadPerToken: 0.0000003,
		ReasoningPerToken: 0.000012,
	}
}

func TestSellMicro_TokenCacheDisabled(t *testing.T) {
	// 1000 prompt + 500 completion, no cache split, ×1, rate 7.
	// usd = 1000*3e-6 + 500*6e-6 = 0.006 ; micro = 0.006*7*10000 = 420
	u := domain.TokenUsage{PromptTokens: 1000, CompletionTokens: 500}
	got := sellMicro(u, chatEntry(), false, 1, 7)
	if got != 420 {
		t.Fatalf("got %d, want 420", got)
	}
}

func TestSellMicro_CacheDisabledIgnoresCacheTokens(t *testing.T) {
	// cacheRead set but cacheEnabled=false → whole prompt at input price.
	// usd = 1000*3e-6 + 200*6e-6 = 0.0042 ; micro = 0.0042*7*10000 = 294
	u := domain.TokenUsage{PromptTokens: 1000, CacheReadTokens: 400, CompletionTokens: 200, ReasoningTokens: 50}
	got := sellMicro(u, chatEntry(), false, 1, 7)
	if got != 294 {
		t.Fatalf("got %d, want 294", got)
	}
}

func TestSellMicro_CacheEnabledSplits(t *testing.T) {
	// cacheEnabled=true uses cache_read + reasoning prices.
	// nonCachedIn=600, cacheRead=400, nonReasonOut=150, reasoning=50
	// usd = 600*3e-6 + 400*3e-7 + 150*6e-6 + 50*1.2e-5
	//     = 0.0018 + 0.00012 + 0.0009 + 0.0006 = 0.00342
	// micro = 0.00342*7*10000 = 239.4 → floor 239
	u := domain.TokenUsage{PromptTokens: 1000, CacheReadTokens: 400, CompletionTokens: 200, ReasoningTokens: 50}
	got := sellMicro(u, chatEntry(), true, 1, 7)
	if got != 239 {
		t.Fatalf("got %d, want 239", got)
	}
}

func TestSellMicro_CacheEnabledZeroPricesFallBack(t *testing.T) {
	// entry with no cache/reasoning prices → fall back to input/output even when enabled.
	e := domain.PriceBookEntry{InputPerToken: 0.000003, OutputPerToken: 0.000006}
	u := domain.TokenUsage{PromptTokens: 1000, CacheReadTokens: 400, CompletionTokens: 200, ReasoningTokens: 50}
	enabled := sellMicro(u, e, true, 1, 7)
	disabled := sellMicro(u, e, false, 1, 7)
	if enabled != disabled {
		t.Fatalf("zero cache prices should match disabled: enabled=%d disabled=%d", enabled, disabled)
	}
}

func TestSellMicro_Multiplier(t *testing.T) {
	u := domain.TokenUsage{PromptTokens: 1000, CompletionTokens: 500}
	full := sellMicro(u, chatEntry(), false, 1, 7)
	half := sellMicro(u, chatEntry(), false, 0.5, 7)
	if half != full/2 {
		t.Fatalf("half=%d, want %d", half, full/2)
	}
}

func TestSellMicro_Image(t *testing.T) {
	e := domain.PriceBookEntry{ImagePrices: []domain.ResolutionUSDPrice{
		{Resolution: "1024x1024", Price: 0.04},
		{Resolution: "512x512", Price: 0.01},
	}}
	u := domain.TokenUsage{ImageCount: 2, ImageResolution: "1024x1024"}
	// usd = 0.04*2 = 0.08 ; micro = 0.08*7*10000 = 5600
	if got := sellMicro(u, e, false, 1, 7); got != 5600 {
		t.Fatalf("got %d, want 5600", got)
	}
	// unknown resolution → lowest price (0.01)
	u2 := domain.TokenUsage{ImageCount: 1, ImageResolution: "999x999"}
	if got := sellMicro(u2, e, false, 1, 7); got != 700 {
		t.Fatalf("fallback got %d, want 700", got)
	}
}

func TestSellMicro_FloorsFraction(t *testing.T) {
	// tiny usage flooring below 1 micro → 0
	e := domain.PriceBookEntry{InputPerToken: 0.000003}
	u := domain.TokenUsage{PromptTokens: 1}
	// usd=3e-6 ; micro=3e-6*7*10000=0.21 → floor 0
	if got := sellMicro(u, e, false, 1, 7); got != 0 {
		t.Fatalf("got %d, want 0", got)
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
