package transport

import "github.com/danielgtaylor/huma/v2"

// UpstreamAccessManagementHTTPDeps is the dependency boundary for platform
// administrators managing a tenant's upstream-resource access policy.
type UpstreamAccessManagementHTTPDeps struct {
	Auth           HTTPAuthDeps
	UpstreamAccess UpstreamAccessManager
}

// RegisterUpstreamAccessManagement owns the platform-admin authenticated
// tenant upstream-access policy routes.
func RegisterUpstreamAccessManagement(api huma.API, d UpstreamAccessManagementHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerTenantUpstreamAccess(management, d)
}
