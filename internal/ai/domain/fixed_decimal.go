package domain

import (
	"math"
	"math/big"
	"strconv"
)

// DecimalRat converts a finite float through its canonical decimal text. This
// avoids carrying binary floating-point artifacts into final billing math while
// the control-plane DTOs are migrated incrementally to decimal strings.
func DecimalRat(v float64) *big.Rat {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return new(big.Rat)
	}
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	if !ok {
		return new(big.Rat)
	}
	return r
}

// RoundHalfUpRatToInt64 rounds a non-negative rational to the nearest integer,
// with exact halves rounded up. Negative values are clamped to zero and values
// above int64 are saturated so malformed configuration cannot wrap a charge.
func RoundHalfUpRatToInt64(v *big.Rat) int64 {
	if v == nil || v.Sign() <= 0 {
		return 0
	}
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(v.Num(), v.Denom(), rem)
	if new(big.Int).Lsh(rem, 1).Cmp(v.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return math.MaxInt64
	}
	return q.Int64()
}

// ScaleInt64HalfUp applies decimal multipliers and rounds exactly once.
func ScaleInt64HalfUp(value int64, multipliers ...float64) int64 {
	if value <= 0 {
		return 0
	}
	r := new(big.Rat).SetInt64(value)
	for _, multiplier := range multipliers {
		if multiplier <= 0 {
			return 0
		}
		r.Mul(r, DecimalRat(multiplier))
	}
	return RoundHalfUpRatToInt64(r)
}
