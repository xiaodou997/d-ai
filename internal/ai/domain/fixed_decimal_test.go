package domain

import (
	"math"
	"math/big"
	"testing"
)

func TestRoundHalfUpRatToInt64(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{value: "0.49", want: 0},
		{value: "0.5", want: 1},
		{value: "1.5", want: 2},
		{value: "239.4", want: 239},
		{value: "239.5", want: 240},
	} {
		r, ok := new(big.Rat).SetString(tc.value)
		if !ok {
			t.Fatalf("parse %q", tc.value)
		}
		if got := RoundHalfUpRatToInt64(r); got != tc.want {
			t.Fatalf("round %s = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestScaleInt64HalfUp(t *testing.T) {
	if got := ScaleInt64HalfUp(1001, 0.4); got != 400 {
		t.Fatalf("1001 * 0.4 = %d, want 400", got)
	}
	if got := ScaleInt64HalfUp(5, 0.5); got != 3 {
		t.Fatalf("5 * 0.5 = %d, want 3", got)
	}
	if got := RoundHalfUpRatToInt64(new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 80))); got != math.MaxInt64 {
		t.Fatalf("overflow = %d, want max int64", got)
	}
}

func TestWholeUSDToMicroRejectsOverflow(t *testing.T) {
	if got, ok := WholeUSDToMicro(MaxWholeUSD); !ok || got != MaxWholeUSD*MicroUSDPerUSD {
		t.Fatalf("max conversion = %d, %v", got, ok)
	}
	if _, ok := WholeUSDToMicro(MaxWholeUSD + 1); ok {
		t.Fatal("overflowing whole-USD amount should be rejected")
	}
	if _, ok := WholeUSDToMicro(-1); ok {
		t.Fatal("negative whole-USD amount should be rejected")
	}
}
