package billing

import "testing"

func TestUSDMicroConversions(t *testing.T) {
	micro, err := WholeUSDToMicro(3)
	if err != nil || micro != 3_000_000 {
		t.Fatalf("WholeUSDToMicro(3) = %d, %v", micro, err)
	}
	if got := MicroToWholeUSD(2_399_999); got != 2 {
		t.Fatalf("MicroToWholeUSD = %d", got)
	}
	if got := MicroToUSD(1_234_500); got != 1.2345 {
		t.Fatalf("MicroToUSD = %v", got)
	}
}
