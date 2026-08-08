package service

import (
	"testing"

	"xiaodou/dai/internal/payment"
)

func topupTestParams(packages []payment.TopupPackage) payment.TopupParams {
	return payment.TopupParams{
		FeeRateBp: 160, MinMicroUSD: payment.TopupMinAmountMicroUSD,
		MaxMicroUSD: payment.TopupMaxAmountMicroUSD, Packages: packages,
	}
}

func TestCalculateTopupSnapshotPackageUsesServerSnapshot(t *testing.T) {
	params := topupTestParams([]payment.TopupPackage{
		{ID: "p20", Name: "$20 基础包", PaymentAmountMicroUSD: 20_000_000, GiftAmountMicroUSD: 2_000_000, Enabled: true},
	})

	got, err := calculateTopupSnapshot(params, 0, "p20")
	if err != nil {
		t.Fatalf("calculateTopupSnapshot returned error: %v", err)
	}
	if got.PaymentAmountMinor != 2_000 || got.GrossAmountMicroUSD != 20_000_000 || got.GiftAmountMicroUSD != 2_000_000 || got.CreditedAmountMicroUSD != 22_000_000 || got.FeeAmountMicroUSD != 0 {
		t.Fatalf("package snapshot mismatch: %+v", got)
	}
}

func TestCalculateTopupSnapshotRejectsPackageAmountMismatch(t *testing.T) {
	params := topupTestParams([]payment.TopupPackage{
		{ID: "p20", Name: "$20 基础包", PaymentAmountMicroUSD: 20_000_000, Enabled: true},
	})
	if _, err := calculateTopupSnapshot(params, 10_000_000, "p20"); err == nil {
		t.Fatal("expected amount mismatch to be rejected")
	}
}

func TestCalculateTopupSnapshotCustomDeductsFee(t *testing.T) {
	got, err := calculateTopupSnapshot(topupTestParams(nil), 10_000_000, "")
	if err != nil {
		t.Fatalf("calculateTopupSnapshot returned error: %v", err)
	}
	if got.PaymentAmountMinor != 1_000 || got.FeeAmountMicroUSD != 160_000 || got.CreditedAmountMicroUSD != 9_840_000 {
		t.Fatalf("custom snapshot mismatch: %+v", got)
	}
}

func TestCalculateTopupSnapshotRejectsDisabledPackage(t *testing.T) {
	params := topupTestParams([]payment.TopupPackage{
		{ID: "p20", Name: "$20 基础包", PaymentAmountMicroUSD: 20_000_000, Enabled: false},
	})
	if _, err := calculateTopupSnapshot(params, 20_000_000, "p20"); err == nil {
		t.Fatal("expected disabled package to be rejected")
	}
}

func TestValidatePackagesRejectsInvalidDefinitions(t *testing.T) {
	cases := []struct {
		name     string
		packages []payment.TopupPackage
	}{
		{"empty id", []payment.TopupPackage{{Name: "$10", PaymentAmountMicroUSD: 10_000_000, Enabled: true}}},
		{"duplicate id", []payment.TopupPackage{{ID: "p10", Name: "$10", PaymentAmountMicroUSD: 10_000_000}, {ID: "p10", Name: "$10 gift", PaymentAmountMicroUSD: 10_000_000}}},
		{"empty name", []payment.TopupPackage{{ID: "p10", PaymentAmountMicroUSD: 10_000_000}}},
		{"invalid amount", []payment.TopupPackage{{ID: "p10", Name: "$10", PaymentAmountMicroUSD: 9_999_999}}},
		{"negative gift", []payment.TopupPackage{{ID: "p10", Name: "$10", PaymentAmountMicroUSD: 10_000_000, GiftAmountMicroUSD: -1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePackages(tc.packages); err == nil {
				t.Fatal("expected package definition to be rejected")
			}
		})
	}
}

func TestEnabledTopupPackagesFiltersDisabledPackages(t *testing.T) {
	packages := []payment.TopupPackage{
		{ID: "p10", Name: "$10", PaymentAmountMicroUSD: 10_000_000, Enabled: true},
		{ID: "hidden", Name: "hidden", PaymentAmountMicroUSD: 10_000_000, Enabled: false},
	}
	got := enabledTopupPackages(packages)
	if len(got) != 1 || got[0].ID != "p10" {
		t.Fatalf("unexpected visible packages: %+v", got)
	}
}
