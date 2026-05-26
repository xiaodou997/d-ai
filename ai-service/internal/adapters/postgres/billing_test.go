package postgres

import (
	"testing"

	"xiaodou/uni-ai-api/internal/domain"
)

// TestCalculateBilling_MicroPrecision verifies that small token counts on
// cheap models no longer round up to 1 credit. With micro-credit precision,
// 300 tokens × 1 credit/M tokens should be 3 micro (= 0.0003 credit), not 1.
func TestCalculateBilling_MicroPrecision(t *testing.T) {
	cases := []struct {
		name        string
		usage       domain.TokenUsage
		pricing     domain.ModelPricing
		wantPlatMic int64 // expected PlatformCost in micro-credits
	}{
		{
			name:  "300 prompt tokens × 1 credit/M (= 10000 micro/M) = 3 micro",
			usage: domain.TokenUsage{PromptTokens: 300},
			pricing: domain.ModelPricing{
				InputPer1M: 10000, // 1 credit per 1M tokens, in micro
			},
			wantPlatMic: 3, // 300 * 10000 / 1_000_000 = 3 micro
		},
		{
			name: "1000 prompt + 500 completion at typical pricing",
			usage: domain.TokenUsage{
				PromptTokens:     1000,
				CompletionTokens: 500,
			},
			pricing: domain.ModelPricing{
				InputPer1M:  1_500_000, // 150 credits/M (= 1.5 元/M)
				OutputPer1M: 6_000_000, // 600 credits/M (= 6 元/M)
			},
			wantPlatMic: 1000*1_500_000/1_000_000 + 500*6_000_000/1_000_000, // 1500 + 3000 = 4500 micro
		},
		{
			name: "zero usage stays zero (no more floor-to-1)",
			usage: domain.TokenUsage{
				PromptTokens: 0,
			},
			pricing: domain.ModelPricing{
				InputPer1M: 1_000_000,
			},
			wantPlatMic: 0,
		},
		{
			name: "sub-1-credit usage no longer inflated",
			usage: domain.TokenUsage{
				PromptTokens: 50, // 50 tokens
			},
			pricing: domain.ModelPricing{
				InputPer1M: 100_000, // 10 credits/M (cheap model, 0.1 元/M)
			},
			// 50 * 100000 / 1_000_000 = 5 micro = 0.0005 credit
			// 旧代码会被 floor 到 1 整积分 (= 10000 micro)，水分 2000 倍
			wantPlatMic: 5,
		},
		{
			name: "cache read tokens are NOT billed separately (subset of PromptTokens)",
			usage: domain.TokenUsage{
				PromptTokens:     100,
				CacheReadTokens:  200,
			},
			pricing: domain.ModelPricing{
				InputPer1M: 1_000_000, // 100 credits/M
			},
			// CacheReadTokens is a subset of PromptTokens, so only PromptTokens is billed.
			// 100 * 1_000_000 / 1_000_000 = 100 micro
			wantPlatMic: 100,
		},
		{
			name: "reasoning tokens are NOT billed separately (subset of CompletionTokens)",
			usage: domain.TokenUsage{
				PromptTokens:     13,
				CompletionTokens: 170,
				ReasoningTokens:  64,
			},
			pricing: domain.ModelPricing{
				InputPer1M:  1_120_000, // 112 credits/M (e.g. gpt-5.4-mini input)
				OutputPer1M: 6_800_000, // 680 credits/M (e.g. gpt-5.4-mini output)
			},
			// ReasoningTokens is a subset of CompletionTokens, not billed separately.
			// prompt: 13 * 1_120_000 / 1_000_000 = 14 micro
			// completion: 170 * 6_800_000 / 1_000_000 = 1156 micro
			// total: 14 + 1156 = 1170 micro (= 0.117 credits)
			wantPlatMic: 1170,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CalculateBilling(c.usage, c.pricing)
			if got.PlatformCostMicro != c.wantPlatMic {
				t.Errorf("PlatformCostMicro = %d micro, want %d micro (= %.4f credits, want %.4f credits)",
					got.PlatformCostMicro, c.wantPlatMic,
					domain.MicroToCreditsFloat(got.PlatformCostMicro),
					domain.MicroToCreditsFloat(c.wantPlatMic))
			}
			// Platform / User / APIKeyQuota Micro 应该相等（token 计费）
			if got.UserCostMicro != got.PlatformCostMicro {
				t.Errorf("UserCostMicro (%d) != PlatformCostMicro (%d)", got.UserCostMicro, got.PlatformCostMicro)
			}
			if got.APIKeyQuotaCostMicro != got.PlatformCostMicro {
				t.Errorf("APIKeyQuotaCostMicro (%d) != PlatformCostMicro (%d)", got.APIKeyQuotaCostMicro, got.PlatformCostMicro)
			}
		})
	}
}

// TestMicroCreditConversions verifies floor/ceil/float helpers.
func TestMicroCreditConversions(t *testing.T) {
	cases := []struct {
		micro     int64
		wantFloor int64
		wantCeil  int64
		wantFloat float64
	}{
		{0, 0, 0, 0.0},
		{1, 0, 1, 0.0001},
		{9999, 0, 1, 0.9999},
		{10000, 1, 1, 1.0},
		{10001, 1, 2, 1.0001},
		{30, 0, 1, 0.003},      // 0.003 credit (was rounded to 1 in old logic)
		{123456, 12, 13, 12.3456},
	}
	for _, c := range cases {
		if f := domain.MicroToCreditsFloor(c.micro); f != c.wantFloor {
			t.Errorf("Floor(%d) = %d, want %d", c.micro, f, c.wantFloor)
		}
		if ce := domain.MicroToCreditsCeil(c.micro); ce != c.wantCeil {
			t.Errorf("Ceil(%d) = %d, want %d", c.micro, ce, c.wantCeil)
		}
		if fl := domain.MicroToCreditsFloat(c.micro); fl != c.wantFloat {
			t.Errorf("Float(%d) = %v, want %v", c.micro, fl, c.wantFloat)
		}
	}
}
