package domain

import "math"

// RoundUpCurrency2 normalizes a positive price to 2 decimal places using
// upward rounding. This keeps admin-facing USD prices stable when values are
// converted between per-token and per-1M units and avoids float tails like
// 0.09999999999999999.
func RoundUpCurrency2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	if v <= 0 {
		return 0
	}
	scaled := v * 100
	nearest := math.Round(scaled)
	if math.Abs(scaled-nearest) < 1e-9 {
		return nearest / 100
	}
	return math.Ceil(scaled-1e-9) / 100
}
