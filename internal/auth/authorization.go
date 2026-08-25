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

// Actor is the normalized identity/scope used by policy checks and commands.
type Actor struct {
	UserID   string
	TenantID string
	UserType int
}

func (a Actor) RequiresTenantScope() bool {
	return a.UserType == 3 || a.UserType == 4
}

func (a Actor) Has(capability Capability) bool {
	switch capability {
	case CapabilitySuperAdmin:
		return a.UserType == 1
	case CapabilityPlatformAdmin:
		return a.UserType == 1 || a.UserType == 2
	case CapabilityTenantSelf:
		return a.UserType == 3 && a.TenantID != ""
	case CapabilityCustomerSelf:
		return a.UserType == 4 && a.TenantID != ""
	default:
		return false
	}
}

// CanAccessTenant applies the resource scope carried by an actor. Platform
// administrators are global; tenant-scoped actors may only access their own
// non-empty tenant.
func (a Actor) CanAccessTenant(tenantID string) bool {
	if tenantID == "" {
		return false
	}
	if a.Has(CapabilityPlatformAdmin) {
		return true
	}
	return (a.Has(CapabilityTenantSelf) || a.Has(CapabilityCustomerSelf)) && a.TenantID == tenantID
}

// CanAccessUser extends tenant scope with customer ownership. Tenant users
// can access users within their tenant; customers can access only themselves.
func (a Actor) CanAccessUser(tenantID, userID string) bool {
	if !a.CanAccessTenant(tenantID) {
		return false
	}
	if a.UserType == 4 {
		return userID != "" && a.UserID == userID
	}
	return true
}
