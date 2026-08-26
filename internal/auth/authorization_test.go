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

func TestActorTenantScopeRequirement(t *testing.T) {
	if (Actor{UserType: 1}).RequiresTenantScope() || (Actor{UserType: 2}).RequiresTenantScope() {
		t.Fatal("administrative actors must not require a tenant scope")
	}
	if !(Actor{UserType: 3}).RequiresTenantScope() || !(Actor{UserType: 4}).RequiresTenantScope() {
		t.Fatal("tenant and customer actors must require a tenant scope")
	}
}

func TestActorUsesTypedScopeAndOwnershipReferences(t *testing.T) {
	actor := NewActor("tenant-user", "tenant-a", int(UserTypeTenant))
	if actor.Scope().IsGlobal() || !actor.Scope().Allows(TenantID("tenant-a")) || actor.Scope().Allows(TenantID("tenant-b")) {
		t.Fatalf("tenant scope = %#v, want only tenant-a", actor.Scope())
	}
	if got := actor.Ownership(); got != (ResourceOwnership{TenantID: TenantID("tenant-a"), UserID: UserID("tenant-user")}) {
		t.Fatalf("actor ownership = %#v", got)
	}
	if !actor.Owns(NewResourceOwnership("tenant-a", "end-user")) || actor.Owns(NewResourceOwnership("tenant-b", "end-user")) {
		t.Fatal("tenant actor ownership crossed tenant boundary")
	}

	global := NewActor("admin", "", int(UserTypePlatformAdmin))
	if !global.Scope().IsGlobal() || !global.Owns(NewResourceOwnership("tenant-b", "end-user")) {
		t.Fatal("global platform actor did not own a global resource reference")
	}
	if NewResourceOwnership("", "user").IsUserResource() || NewResourceOwnership("tenant-a", "").IsUserResource() {
		t.Fatal("invalid resource ownership reference was accepted")
	}
}

func TestActorFromClaimsPreservesInvalidRoleWithoutWraparound(t *testing.T) {
	actor := ActorFromClaims(&Claims{UserID: "malformed", TenantID: "tenant-a", UserType: 257})
	if actor.UserType != UserType(257) || actor.Has(CapabilitySuperAdmin) || actor.Has(CapabilityPlatformAdmin) {
		t.Fatalf("malformed role was normalized to a privileged role: %#v", actor)
	}
}
