package postgres

import (
	"math"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestEncodeTokenPriceTiersReturnsMarshalError(t *testing.T) {
	_, err := encodeTokenPriceTiers([]domain.TokenPriceTier{{InputPerToken: math.Inf(1)}})
	if err == nil {
		t.Fatal("expected non-JSON token price to fail encoding")
	}
}

func TestDecodeTokenPriceTiersReturnsUnmarshalError(t *testing.T) {
	_, err := decodeTokenPriceTiers([]byte(`{"input_per_token":`))
	if err == nil {
		t.Fatal("expected malformed token price tiers to fail decoding")
	}
}
