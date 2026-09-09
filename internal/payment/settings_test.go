package payment

import "testing"

func TestNormalizePackagesDoesNotRepairInvalidPaymentConfig(t *testing.T) {
	packages := normalizePackages([]TopupPackage{
		{ID: " ", Name: " ", PaymentAmountMicroUSD: 1, Enabled: true},
	})

	if len(packages) != 1 {
		t.Fatalf("unexpected package count: %d", len(packages))
	}
	if packages[0].ID != "" || packages[0].Name != "" || packages[0].PaymentAmountMicroUSD != 1 {
		t.Fatalf("normalizePackages repaired invalid config: %+v", packages[0])
	}
	if err := validatePackages(packages, 0); err == nil {
		t.Fatal("expected invalid package config to be rejected")
	}
}

func TestValidateSettingsRejectsOutOfRangeValues(t *testing.T) {
	global := DefaultGlobalSettings()
	global.TenantCustomTopupFeeBp = 10_001
	if err := validateGlobalSettings(global); err == nil {
		t.Fatal("expected invalid fee rate to be rejected")
	}

	tenant := DefaultTenantSettings(DefaultGlobalSettings())
	tenant.UserTopupPackages[0].PaymentAmountMicroUSD = MaxPackageAmountMicroUSD + 1
	if err := validateTenantSettings(tenant); err == nil {
		t.Fatal("expected overflowing package amount to be rejected")
	}
}

func TestValidatePackagesChecksFeeInclusiveCentPrecision(t *testing.T) {
	packages := []TopupPackage{{
		ID: "p10", Name: "$10", PaymentAmountMicroUSD: 10_000_001, FeeEnabled: true, Enabled: true,
	}}
	if err := validatePackages(packages, 160); err == nil {
		t.Fatal("expected fee-inclusive package amount with sub-cent payment to be rejected")
	}
}
