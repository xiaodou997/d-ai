package transport

import (
	"testing"

	"xiaodou/dai/internal/auth"
)

func TestLoginRoleDecisionsUseSharedActorPolicy(t *testing.T) {
	roleCases := []struct {
		name           string
		userType       int
		tenantID       string
		wantTenantGate bool
		wantAdminAudit bool
		wantMFA        bool
	}{
		{name: "super admin", userType: 1, wantAdminAudit: true, wantMFA: true},
		{name: "platform admin", userType: 2, wantAdminAudit: true, wantMFA: true},
		{name: "tenant", userType: 3, tenantID: "tenant-a", wantTenantGate: true},
		{name: "customer", userType: 4, tenantID: "tenant-a", wantTenantGate: true},
		{name: "unknown", userType: 99, tenantID: "tenant-a"},
	}
	for _, tt := range roleCases {
		t.Run(tt.name, func(t *testing.T) {
			actor := auth.NewActor("user", tt.tenantID, tt.userType)
			if got := actor.RequiresTenantScope(); got != tt.wantTenantGate {
				t.Fatalf("RequiresTenantScope() = %v, want %v", got, tt.wantTenantGate)
			}
			if got := principalType(tt.userType); (got == "admin") != tt.wantAdminAudit {
				t.Fatalf("principalType() = %q, want admin=%v", got, tt.wantAdminAudit)
			}
			principal := loginPrincipal{UserID: "user", TenantID: tt.tenantID, UserType: tt.userType, MFAEnabled: true}
			if got := requiresAdministrativeMFA(principal); got != tt.wantMFA {
				t.Fatalf("requiresAdministrativeMFA() = %v, want %v", got, tt.wantMFA)
			}
		})
	}
}
