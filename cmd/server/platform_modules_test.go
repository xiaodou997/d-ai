package main

import (
	"strings"
	"testing"

	"xiaodou/dai/internal/auth"
	cleanup "xiaodou/dai/internal/cleanup"
	payment "xiaodou/dai/internal/payment/service"
	"xiaodou/dai/internal/scheduler"
	tenant "xiaodou/dai/internal/tenant"
	user "xiaodou/dai/internal/user"
)

func TestConfigureSecretKeyringRequiresConfig(t *testing.T) {
	if err := configureSecretKeyring(nil); err == nil {
		t.Fatal("expected nil configuration to be rejected")
	}
}

func TestPlatformModulesLifecycleIsSafeForEmptyBundle(t *testing.T) {
	modules := &platformModules{}
	modules.Start()
	modules.Start()
	modules.Stop()
	modules.Stop()
}

func TestValidatePlatformAssemblyRejectsIncompleteBundle(t *testing.T) {
	err := validatePlatformAssembly(&platformModules{})
	if err == nil {
		t.Fatal("expected incomplete platform assembly to be rejected")
	}
	for _, dependency := range []string{"jwt", "tenant_lifecycle", "admin_account_lifecycle", "scheduler"} {
		if !strings.Contains(err.Error(), dependency) {
			t.Fatalf("validation error %q does not mention %q", err, dependency)
		}
	}
}

func TestValidatePlatformAssemblyRejectsNilBundle(t *testing.T) {
	if err := validatePlatformAssembly(nil); err == nil {
		t.Fatal("expected nil platform assembly to be rejected")
	}
}

func TestValidatePlatformAssemblyAcceptsCompleteBundle(t *testing.T) {
	modules := &platformModules{
		JWT:                   &auth.JWTService{},
		Sessions:              &auth.SessionService{},
		Security:              &auth.AccountSecurityService{},
		TenantLifecycle:       &tenant.AdminTenantLifecycleService{},
		AdminAccountLifecycle: &user.AdminAccountLifecycleService{},
		AdminEndUserLifecycle: &user.AdminEndUserLifecycleService{},
		Payment:               &payment.PaymentService{},
		DataCleanup:           &cleanup.Service{},
		banReconciler:         &auth.BanReconciler{},
		sched:                 &scheduler.Scheduler{},
	}
	if err := validatePlatformAssembly(modules); err != nil {
		t.Fatalf("complete platform assembly rejected: %v", err)
	}
}
