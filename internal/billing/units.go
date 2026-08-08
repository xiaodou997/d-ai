package billing

import (
	"xiaodou/dai/internal/money"
)

const MicroUSDPerUSD int64 = money.MicrosPerUSD

func WholeUSDToMicro(usd int64) (int64, error) {
	return money.WholeUSDToMicros(usd)
}

func MicroToWholeUSD(micro int64) int64 {
	return micro / MicroUSDPerUSD
}

func MicroToUSD(micro int64) float64 {
	return money.MicrosToUSD(micro)
}
