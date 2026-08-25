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
		{"tenant owns tenant", Actor{UserType: 3, TenantID: "t1"}, CapabilityTenantSelf, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.actor.Has(tt.cap); got != tt.want {
				t.Fatalf("Has(%q) = %v, want %v", tt.cap, got, tt.want)
			}
		})
	}
}

func TestActorResourceOwnershipRejectsCrossTenantAccess(t *testing.T) {
	tenant := Actor{UserID: "tenant-user", TenantID: "t1", UserType: 3}
	if !tenant.CanAccessTenant("t1") || tenant.CanAccessTenant("t2") {
		t.Fatal("tenant actor scope is not enforced")
	}
	if !tenant.CanAccessUser("t1", "end-user") || tenant.CanAccessUser("t2", "end-user") {
		t.Fatal("tenant user ownership is not enforced")
	}
	customer := Actor{UserID: "end-1", TenantID: "t1", UserType: 4}
	if customer.CanAccessUser("t1", "end-2") || !customer.CanAccessUser("t1", "end-1") {
		t.Fatal("customer user ownership is not enforced")
	}
	admin := Actor{UserID: "admin", UserType: 2}
	if !admin.CanAccessTenant("t2") || !admin.CanAccessUser("t2", "end-user") {
		t.Fatal("platform admin should be global")
	}
}
