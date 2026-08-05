package service

import (
	"testing"

	"xiaodou/dai/internal/payment"
)

func TestCalculateTopupSnapshotPackageUsesServerSnapshot(t *testing.T) {
	params := payment.TopupParams{
		CreditsPerCNY: 100,
		FeeRateBp:     160,
		Min:           payment.TopupMinAmountFen,
		Max:           payment.TopupMaxAmountFen,
		Packages: []payment.TopupPackage{
			{ID: "p20", Name: "20 元基础包", Amount: 2000, Credits: 2200, Enabled: true, SortOrder: 10},
		},
	}

	got, err := calculateTopupSnapshot(params, 0, "p20")
	if err != nil {
		t.Fatalf("calculateTopupSnapshot returned error: %v", err)
	}
	if got.AmountFen != 2000 || got.CreditAmount != 2200 || got.FeeCredits != 0 {
		t.Fatalf("package snapshot mismatch: amount=%d credits=%d feeCredits=%d", got.AmountFen, got.CreditAmount, got.FeeCredits)
	}
}

func TestCalculateTopupSnapshotRejectsPackageAmountMismatch(t *testing.T) {
	params := payment.TopupParams{
		CreditsPerCNY: 100,
		FeeRateBp:     160,
		Min:           payment.TopupMinAmountFen,
		Max:           payment.TopupMaxAmountFen,
		Packages: []payment.TopupPackage{
			{ID: "p20", Name: "20 元基础包", Amount: 2000, Credits: 2200, Enabled: true, SortOrder: 10},
		},
	}

	if _, err := calculateTopupSnapshot(params, 1000, "p20"); err == nil {
		t.Fatal("expected amount mismatch to be rejected")
	}
}

func TestCalculateTopupSnapshotRejectsDisabledPackage(t *testing.T) {
	params := payment.TopupParams{
		CreditsPerCNY: 100,
		FeeRateBp:     160,
		Min:           payment.TopupMinAmountFen,
		Max:           payment.TopupMaxAmountFen,
		Packages: []payment.TopupPackage{
			{ID: "p20", Name: "20 元基础包", Amount: 2000, Credits: 2200, Enabled: false, SortOrder: 10},
		},
	}

	if _, err := calculateTopupSnapshot(params, 2000, "p20"); err == nil {
		t.Fatal("expected disabled package to be rejected")
	}
}

func TestValidatePackagesRejectsInvalidDefinitions(t *testing.T) {
	cases := []struct {
		name     string
		packages []payment.TopupPackage
	}{
		{
			name:     "empty id",
			packages: []payment.TopupPackage{{ID: "", Name: "10 元体验包", Amount: 1000, Credits: 1000, Enabled: true}},
		},
		{
			name: "duplicate id",
			packages: []payment.TopupPackage{
				{ID: "p10", Name: "10 元体验包", Amount: 1000, Credits: 1000, Enabled: true},
				{ID: "p10", Name: "10 元活动包", Amount: 1000, Credits: 1200, Enabled: true},
			},
		},
		{
			name:     "empty name",
			packages: []payment.TopupPackage{{ID: "p10", Name: "", Amount: 1000, Credits: 1000, Enabled: true}},
		},
		{
			name:     "invalid amount",
			packages: []payment.TopupPackage{{ID: "p10", Name: "10 元体验包", Amount: 999, Credits: 1000, Enabled: true}},
		},
		{
			name:     "invalid credits",
			packages: []payment.TopupPackage{{ID: "p10", Name: "10 元体验包", Amount: 1000, Credits: 0, Enabled: true}},
		},
		{
			name:     "credits too large",
			packages: []payment.TopupPackage{{ID: "p10", Name: "10 元体验包", Amount: 1000, Credits: payment.MaxPackageCredits + 1, Enabled: true}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePackages(tc.packages); err == nil {
				t.Fatal("expected package definition to be rejected")
			}
		})
	}
}

func TestValidateSettingsRejectsLargeExchangeRate(t *testing.T) {
	global := payment.DefaultGlobalSettings()
	global.CreditsPerCNY = payment.MaxCreditsPerCNY + 1
	if err := validateGlobalSettings(global); err == nil {
		t.Fatal("expected global exchange rate to be rejected")
	}

	tenant := payment.DefaultTenantSettings(payment.DefaultGlobalSettings())
	tenant.UserCreditsPerCNY = payment.MaxCreditsPerCNY + 1
	if err := validateTenantSettings(tenant); err == nil {
		t.Fatal("expected tenant exchange rate to be rejected")
	}
}

func TestCreditsFromAmountRejectsOverflow(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)
	if _, err := creditsFromAmount(maxInt64, 2); err == nil {
		t.Fatal("expected overflowing credit calculation to be rejected")
	}
}

func TestEnabledTopupPackagesFiltersDisabledPackages(t *testing.T) {
	packages := []payment.TopupPackage{
		{ID: "p10", Name: "10 元体验包", Amount: 1000, Credits: 1000, Enabled: true},
		{ID: "hidden", Name: "隐藏活动包", Amount: 1000, Credits: 999999, Enabled: false},
	}

	got := enabledTopupPackages(packages)
	if len(got) != 1 || got[0].ID != "p10" {
		t.Fatalf("unexpected visible packages: %+v", got)
	}
}
