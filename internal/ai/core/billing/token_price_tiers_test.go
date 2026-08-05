package billing

import "testing"

func tokenLimit(value int) *int {
	return &value
}

func TestSelectTokenPriceTierUsesWholeRequestInputContext(t *testing.T) {
	tiers := []TokenPriceTier{
		{UpToInputTokens: tokenLimit(200_000), InputPerToken: 1, OutputPerToken: 2},
		{UpToInputTokens: tokenLimit(272_000), InputPerToken: 3, OutputPerToken: 4},
		{UpToInputTokens: nil, InputPerToken: 5, OutputPerToken: 6},
	}

	tests := []struct {
		name         string
		promptTokens int
		wantIndex    int
	}{
		{name: "below first threshold", promptTokens: 199_999, wantIndex: 0},
		{name: "at first threshold", promptTokens: 200_000, wantIndex: 0},
		{name: "above first threshold", promptTokens: 200_001, wantIndex: 1},
		{name: "at second threshold", promptTokens: 272_000, wantIndex: 1},
		{name: "unbounded tier", promptTokens: 272_001, wantIndex: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, index, err := SelectTokenPriceTier(tiers, tt.promptTokens)
			if err != nil {
				t.Fatalf("select tier: %v", err)
			}
			if index != tt.wantIndex {
				t.Fatalf("index = %d, want %d", index, tt.wantIndex)
			}
			if tier != tiers[tt.wantIndex] {
				t.Fatalf("tier = %#v, want %#v", tier, tiers[tt.wantIndex])
			}
		})
	}
}

func TestValidateTokenPriceTiersRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name  string
		tiers []TokenPriceTier
	}{
		{name: "empty"},
		{name: "missing terminal tier", tiers: []TokenPriceTier{{UpToInputTokens: tokenLimit(200_000)}}},
		{name: "unbounded tier before end", tiers: []TokenPriceTier{{}, {}}},
		{name: "unordered thresholds", tiers: []TokenPriceTier{{UpToInputTokens: tokenLimit(200_000)}, {UpToInputTokens: tokenLimit(128_000)}, {}}},
		{name: "zero threshold", tiers: []TokenPriceTier{{UpToInputTokens: tokenLimit(0)}, {}}},
		{name: "negative price", tiers: []TokenPriceTier{{InputPerToken: -1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTokenPriceTiers(tt.tiers); err == nil {
				t.Fatal("expected invalid tiers to fail")
			}
		})
	}
}

func TestValidateTokenPriceTiersAcceptsFreePrices(t *testing.T) {
	if err := ValidateTokenPriceTiers([]TokenPriceTier{{}}); err != nil {
		t.Fatalf("zero prices represent free billing and must be valid: %v", err)
	}
}

func TestIsTokenPricedCapability(t *testing.T) {
	for _, capability := range []string{"chat", "embedding", "rerank"} {
		if !IsTokenPricedCapability(capability) {
			t.Fatalf("%q should require token pricing", capability)
		}
	}
	for _, capability := range []string{"image", "video", "audio_tts", "audio_stt"} {
		if IsTokenPricedCapability(capability) {
			t.Fatalf("%q should not require token pricing", capability)
		}
	}
}
