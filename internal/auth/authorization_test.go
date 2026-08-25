package auth

import "testing"

func TestActorCapabilitiesRespectRoleAndScope(t *testing.T) {
	tests := []struct {
		name  string
		actor Actor
		cap   Capability
		want  bool
	}{
		{"super admin", Actor{UserType: 1}, CapabilitySuperAdmin, true},
		{"platform admin includes super", Actor{UserType: 1}, CapabilityPlatformAdmin, true},
		{"platform admin", Actor{UserType: 2}, CapabilityPlatformAdmin, true},
		{"tenant requires scope", Actor{UserType: 3}, CapabilityTenantSelf, false},
		{"tenant scoped", Actor{UserType: 3, TenantID: "t1"}, CapabilityTenantSelf, true},
		{"customer scoped", Actor{UserType: 4, TenantID: "t1"}, CapabilityCustomerSelf, true},
		{"customer cannot tenant", Actor{UserType: 4, TenantID: "t1"}, CapabilityTenantSelf, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.actor.Has(tt.cap); got != tt.want {
				t.Fatalf("Has(%q) = %v, want %v", tt.cap, got, tt.want)
			}
		})
	}
}
