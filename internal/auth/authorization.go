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
