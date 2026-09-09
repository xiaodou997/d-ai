package transport

import (
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func TestOverviewSummaryUsesSuccessAndCacheRateDenominators(t *testing.T) {
	summary := overviewSummaryFromDomain(domain.DashboardSummary{
		TotalRequests: 10, SuccessfulRequests: 8, FailedRequests: 2,
		TotalPromptTokens: 800, CacheReadTokens: 200,
		TotalTenantPayableMicro: 10_000_000, TotalCatalogBaseMicro: 4_000_000,
	})
	if summary.SuccessRate == nil || *summary.SuccessRate != 80 {
		t.Fatalf("success rate = %#v", summary.SuccessRate)
	}
	if summary.CacheRate == nil || *summary.CacheRate != 20 {
		t.Fatalf("cache rate = %#v", summary.CacheRate)
	}
	if summary.GrossMarginUSD != 6 || summary.GrossMarginRate == nil || *summary.GrossMarginRate != 60 {
		t.Fatalf("margin = %#v/%#v", summary.GrossMarginUSD, summary.GrossMarginRate)
	}
}

func TestOverviewRatesAreNilWithoutDenominator(t *testing.T) {
	summary := overviewSummaryFromDomain(domain.DashboardSummary{})
	if summary.SuccessRate != nil || summary.CacheRate != nil || summary.GrossMarginRate != nil {
		t.Fatalf("zero-denominator rates = %#v", summary)
	}
}

func TestPreviousOverviewWindowKeepsPeriodLength(t *testing.T) {
	from := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	previousFrom, previousTo := previousOverviewWindow(&from, &to)
	if previousFrom == nil || previousTo == nil || previousTo.Sub(*previousFrom) != 24*time.Hour {
		t.Fatalf("previous window = %v/%v", previousFrom, previousTo)
	}
	if !previousTo.Before(from) {
		t.Fatalf("previous window overlaps current: %v < %v", previousTo, from)
	}
}

func TestOverviewAccountRatesUseInputAndCacheReadTokens(t *testing.T) {
	account := overviewAccountFromDomain(domain.UsageUpstreamSummaryRow{
		TargetKind: "direct_upstream", TargetID: "account-1", RequestCount: 10, SuccessCount: 9,
		TotalPromptTokens: 800, CacheReadTokens: 200,
	})
	if account.SuccessRate == nil || *account.SuccessRate != 90 || account.CacheRate == nil || *account.CacheRate != 20 {
		t.Fatalf("account rates = %#v/%#v", account.SuccessRate, account.CacheRate)
	}
}
