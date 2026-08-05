package billing

import (
	"errors"
	"fmt"
	"math"
)

// TokenPriceTier prices an entire token request after selecting a tier from
// its total input context. A nil upper bound is the required terminal tier.
type TokenPriceTier struct {
	UpToInputTokens    *int    `json:"up_to_input_tokens"`
	InputPerToken      float64 `json:"input_per_token"`
	OutputPerToken     float64 `json:"output_per_token"`
	CacheWritePerToken float64 `json:"cache_write_per_token"`
	CacheReadPerToken  float64 `json:"cache_read_per_token"`
}

// IsTokenPricedCapability identifies capabilities that require token tiers.
func IsTokenPricedCapability(capability string) bool {
	switch capability {
	case "chat", "embedding", "rerank":
		return true
	default:
		return false
	}
}

// ValidateTokenPriceTiers validates the persisted tier order and prices.
func ValidateTokenPriceTiers(tiers []TokenPriceTier) error {
	if len(tiers) == 0 {
		return errors.New("token price tiers are required")
	}
	previousLimit := 0
	for index, tier := range tiers {
		if err := validateTierPrices(index, tier); err != nil {
			return err
		}
		if tier.UpToInputTokens == nil {
			if index != len(tiers)-1 {
				return fmt.Errorf("token price tier %d is unbounded before the final tier", index)
			}
			continue
		}
		limit := *tier.UpToInputTokens
		if limit <= 0 {
			return fmt.Errorf("token price tier %d upper bound must be positive", index)
		}
		if limit <= previousLimit {
			return fmt.Errorf("token price tier %d upper bound must be strictly increasing", index)
		}
		previousLimit = limit
	}
	if tiers[len(tiers)-1].UpToInputTokens != nil {
		return errors.New("token price tiers require an unbounded final tier")
	}
	return nil
}

// SelectTokenPriceTier returns the first tier whose inclusive input-context
// upper bound contains promptTokens.
func SelectTokenPriceTier(tiers []TokenPriceTier, promptTokens int) (TokenPriceTier, int, error) {
	if err := ValidateTokenPriceTiers(tiers); err != nil {
		return TokenPriceTier{}, -1, err
	}
	if promptTokens < 0 {
		return TokenPriceTier{}, -1, errors.New("prompt tokens must be non-negative")
	}
	for index, tier := range tiers {
		if tier.UpToInputTokens == nil || promptTokens <= *tier.UpToInputTokens {
			return tier, index, nil
		}
	}
	return TokenPriceTier{}, -1, errors.New("no token price tier matched")
}

func validateTierPrices(index int, tier TokenPriceTier) error {
	prices := []struct {
		name  string
		value float64
	}{
		{name: "input_per_token", value: tier.InputPerToken},
		{name: "output_per_token", value: tier.OutputPerToken},
		{name: "cache_write_per_token", value: tier.CacheWritePerToken},
		{name: "cache_read_per_token", value: tier.CacheReadPerToken},
	}
	for _, price := range prices {
		if price.value < 0 || math.IsNaN(price.value) || math.IsInf(price.value, 0) {
			return fmt.Errorf("token price tier %d %s must be a finite non-negative number", index, price.name)
		}
	}
	return nil
}
