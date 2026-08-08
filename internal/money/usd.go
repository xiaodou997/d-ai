package money

import (
	"fmt"
	"math"
	"math/big"
)

const (
	CurrencyUSD        = "USD"
	MicrosPerUSD int64 = 1_000_000
	MaxWholeUSD        = (1<<63 - 1) / MicrosPerUSD
)

// USDToMicros is intended for control-plane input and display boundaries.
// Runtime billing uses exact rational arithmetic and rounds only once.
func USDToMicros(value float64) (int64, error) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value > float64(math.MaxInt64)/float64(MicrosPerUSD) {
		return 0, fmt.Errorf("USD amount is outside the supported range")
	}
	return int64(math.Round(value * float64(MicrosPerUSD))), nil
}

func WholeUSDToMicros(value int64) (int64, error) {
	if value < 0 || value > MaxWholeUSD {
		return 0, fmt.Errorf("USD amount is outside the supported range")
	}
	return value * MicrosPerUSD, nil
}

func MicrosToUSD(value int64) float64 {
	return float64(value) / float64(MicrosPerUSD)
}

// ApplyBasisPointsCeil returns ceil(amount * basisPoints / 10,000) without
// overflowing intermediate int64 multiplication.
func ApplyBasisPointsCeil(amount int64, basisPoints int) (int64, error) {
	if amount < 0 || basisPoints < 0 || basisPoints > 10_000 {
		return 0, fmt.Errorf("invalid amount or basis points")
	}
	product := new(big.Int).Mul(big.NewInt(amount), big.NewInt(int64(basisPoints)))
	product.Add(product, big.NewInt(9_999))
	product.Quo(product, big.NewInt(10_000))
	if !product.IsInt64() {
		return 0, fmt.Errorf("basis point result overflows int64")
	}
	return product.Int64(), nil
}
