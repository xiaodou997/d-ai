package auth

// Capability is a backend authorization capability. UI visibility may use
// the same names, but handlers must always enforce these server-side.
type Capability string

const (
	CapabilitySuperAdmin    Capability = "super_admin"
	CapabilityPlatformAdmin Capability = "platform_admin"
	CapabilityTenantSelf    Capability = "tenant_self"
	CapabilityCustomerSelf  Capability = "customer_self"
)

// UserID and TenantID keep identity values distinct from arbitrary strings at
// the authorization boundary. Transport and persistence adapters may still
// convert them to wire strings explicitly at their edges.
type UserID string
type TenantID string

// UserType is the persisted portal role. Keep the numeric values aligned with
// the identity schema and JWT claims until those wire contracts are versioned.
// It intentionally remains an int so malformed out-of-range JWT claims cannot
// wrap into a privileged role during policy evaluation.
type UserType int

const (
	UserTypeSuperAdmin    UserType = 1
	UserTypePlatformAdmin UserType = 2
	UserTypeTenant        UserType = 3
	UserTypeCustomer      UserType = 4
)

// TenantScope is the only tenant boundary an actor may carry. An empty tenant
// means a global scope (used by platform administrators); tenant-scoped roles
// are admitted only when their TenantID is non-empty.
type TenantScope struct {
	TenantID TenantID
}

func (s TenantScope) IsGlobal() bool { return s.TenantID == "" }

func (s TenantScope) Allows(tenantID TenantID) bool {
	return tenantID != "" && s.TenantID == tenantID
}

// ResourceOwnership is the common ownership reference for tenant and
// end-user resources. A user resource must carry both IDs; a tenant resource
// may leave UserID empty.
type ResourceOwnership struct {
	TenantID TenantID
	UserID   UserID
}

func NewResourceOwnership(tenantID, userID string) ResourceOwnership {
	return ResourceOwnership{TenantID: TenantID(tenantID), UserID: UserID(userID)}
}

func (r ResourceOwnership) IsTenantResource() bool {
	return r.TenantID != "" && r.UserID == ""
}

func (r ResourceOwnership) IsUserResource() bool {
	return r.TenantID != "" && r.UserID != ""
}

// Actor is the normalized identity/scope used by policy checks and commands.
type Actor struct {
	UserID   UserID
	TenantID TenantID
	UserType UserType
}

func NewActor(userID, tenantID string, userType int) Actor {
	return Actor{UserID: UserID(userID), TenantID: TenantID(tenantID), UserType: UserType(userType)}
}

func ActorFromClaims(claims *Claims) Actor {
	if claims == nil {
		return Actor{}
	}
	return NewActor(claims.UserID, claims.TenantID, claims.UserType)
}

func (a Actor) Scope() TenantScope { return TenantScope{TenantID: a.TenantID} }

func (a Actor) Ownership() ResourceOwnership {
	return ResourceOwnership{TenantID: a.TenantID, UserID: a.UserID}
}

func (a Actor) RequiresTenantScope() bool {
	return a.UserType == UserTypeTenant || a.UserType == UserTypeCustomer
}

func (a Actor) Has(capability Capability) bool {
	switch capability {
	case CapabilitySuperAdmin:
		return a.UserType == UserTypeSuperAdmin
	case CapabilityPlatformAdmin:
		return a.UserType == UserTypeSuperAdmin || a.UserType == UserTypePlatformAdmin
	case CapabilityTenantSelf:
		return a.UserType == UserTypeTenant && a.TenantID != ""
	case CapabilityCustomerSelf:
		return a.UserType == UserTypeCustomer && a.TenantID != ""
	default:
		return false
	}
}

// CanAccessTenant applies the resource scope carried by an actor. Platform
// administrators are global; tenant-scoped actors may only access their own
// non-empty tenant.
func (a Actor) CanAccessTenant(tenantID TenantID) bool {
	if tenantID == "" {
		return false
	}
	if a.Has(CapabilityPlatformAdmin) {
		return true
	}
	return (a.Has(CapabilityTenantSelf) || a.Has(CapabilityCustomerSelf)) && a.Scope().Allows(tenantID)
}

// CanAccessUser extends tenant scope with customer ownership. Tenant users
// can access users within their tenant; customers can access only themselves.
func (a Actor) CanAccessUser(tenantID TenantID, userID UserID) bool {
	if !a.CanAccessTenant(tenantID) {
		return false
	}
	if a.UserType == UserTypeCustomer {
		return userID != "" && a.UserID == userID
	}
	return true
}

// Owns applies the same tenant/user ownership rule to a typed resource
// reference. It is intentionally stricter than capability admission: a
// caller may have the right role but still not own this object.
func (a Actor) Owns(resource ResourceOwnership) bool {
	if !resource.IsTenantResource() && !resource.IsUserResource() {
		return false
	}
	if resource.UserID == "" {
		return a.CanAccessTenant(resource.TenantID)
	}
	return a.CanAccessUser(resource.TenantID, resource.UserID)
}
