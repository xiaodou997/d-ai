package payment

import "testing"

func TestNormalizePackagesDoesNotRepairInvalidPaymentConfig(t *testing.T) {
	packages := normalizePackages([]TopupPackage{
		{ID: " ", Name: " ", Amount: 1, Credits: 0, Enabled: true},
	})

	if len(packages) != 1 {
		t.Fatalf("unexpected package count: %d", len(packages))
	}
	if packages[0].ID != "" || packages[0].Name != "" || packages[0].Amount != 1 || packages[0].Credits != 0 {
		t.Fatalf("normalizePackages repaired invalid config: %+v", packages[0])
	}
	if err := validatePackages(packages); err == nil {
		t.Fatal("expected invalid package config to be rejected")
	}
}

func TestValidateSettingsRejectsOutOfRangeValues(t *testing.T) {
	global := DefaultGlobalSettings()
	global.CreditsPerCNY = MaxCreditsPerCNY + 1
	if err := validateGlobalSettings(global); err == nil {
		t.Fatal("expected invalid global credits per CNY to be rejected")
	}

	tenant := DefaultTenantSettings(DefaultGlobalSettings())
	tenant.UserTopupPackages[0].Credits = MaxPackageCredits + 1
	if err := validateTenantSettings(tenant); err == nil {
		t.Fatal("expected invalid package credits to be rejected")
	}
}
