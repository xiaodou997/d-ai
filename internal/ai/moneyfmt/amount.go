package moneyfmt

import (
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/dai/internal/money"
)

func MicroToUSD(micro int64) float64 {
	return money.MicrosToUSD(micro)
}

func WholeUSDToMicro(value int64) int64 {
	return value * money.MicrosPerUSD
}

func MicroToWholeUSD(micro int64) int64 {
	return micro / money.MicrosPerUSD
}

func WholeUSDPtrToInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: WholeUSDToMicro(*value), Valid: true}
}

func Int8ToWholeUSDPtr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	usd := MicroToWholeUSD(value.Int64)
	return &usd
}
