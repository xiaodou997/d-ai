package billing

import (
	"math"
	"testing"
)

func TestCreditMicroConversions(t *testing.T) {
	micro, err := CreditsToMicro(3)
	if err != nil || micro != 30_000 {
		t.Fatalf("CreditsToMicro(3) = %d, %v", micro, err)
	}
	if got := MicroToWholeCredits(23_999); got != 2 {
		t.Fatalf("MicroToWholeCredits = %d", got)
	}
	if got := MicroToCredits(12_345); got != 1.2345 {
		t.Fatalf("MicroToCredits = %v", got)
	}
	if _, err := CreditsToMicro(math.MaxInt64); err == nil {
		t.Fatal("expected overflow error")
	}
}
