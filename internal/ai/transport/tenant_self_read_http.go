package transport

import "github.com/danielgtaylor/huma/v2"

// TenantSelfReadHTTPDeps is the dependency boundary for tenant-scoped
// dashboard and usage read projections. Tenant controls and workspace routes
// are registered by separate modules.
type TenantSelfReadHTTPDeps struct {
	Auth             HTTPAuthDeps
	DashboardQueries DashboardQueryReader
	UsageQueries     UsageQueryReader
}

// RegisterTenantSelfRead owns the tenant-user authenticated dashboard and
// usage read routes.
func RegisterTenantSelfRead(api huma.API, d TenantSelfReadHTTPDeps) {
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, d.Auth))
	registerTenantSelf(tenant, d)
}
