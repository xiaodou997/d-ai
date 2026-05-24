package credits

import (
	"math"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/uni-ai-api/internal/domain"
)

const MicroPerCredit = domain.MicroCreditsPerCredit

func MicroToCredits(micro int64) float64 {
	return float64(micro) / float64(MicroPerCredit)
}

func CreditsToMicro(value float64) int64 {
	return int64(math.Round(value * float64(MicroPerCredit)))
}

func WholeCreditsToMicro(value int64) int64 {
	return value * MicroPerCredit
}

func MicroToWholeCredits(micro int64) int64 {
	return micro / MicroPerCredit
}

func WholeCreditsPtrToInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: WholeCreditsToMicro(*value), Valid: true}
}

func Int8ToWholeCreditsPtr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	credits := MicroToWholeCredits(value.Int64)
	return &credits
}
