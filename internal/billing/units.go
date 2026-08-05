package billing

import (
	"fmt"
	"math"
)

const MicroCreditsPerCredit int64 = 10_000

func CreditsToMicro(credits int64) (int64, error) {
	if credits < 0 {
		return 0, fmt.Errorf("credits must not be negative")
	}
	if credits > math.MaxInt64/MicroCreditsPerCredit {
		return 0, fmt.Errorf("credit amount overflows microcredit storage")
	}
	return credits * MicroCreditsPerCredit, nil
}

func MicroToWholeCredits(micro int64) int64 {
	return micro / MicroCreditsPerCredit
}

func MicroToCredits(micro int64) float64 {
	return float64(micro) / float64(MicroCreditsPerCredit)
}
