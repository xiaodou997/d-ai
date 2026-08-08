package money

import "testing"

func TestUSDMicroConversions(t *testing.T) {
	micro, err := USDToMicros(12.345678)
	if err != nil || micro != 12_345_678 {
		t.Fatalf("USDToMicros() = %d, %v", micro, err)
	}
	if got := MicrosToUSD(micro); got != 12.345678 {
		t.Fatalf("MicrosToUSD() = %v", got)
	}
}

func TestApplyBasisPointsCeil(t *testing.T) {
	got, err := ApplyBasisPointsCeil(1_000_001, 160)
	if err != nil || got != 16_001 {
		t.Fatalf("ApplyBasisPointsCeil() = %d, %v", got, err)
	}
}
