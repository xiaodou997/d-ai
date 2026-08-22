package transport

import "github.com/danielgtaylor/huma/v2"

// DashboardHTTPDeps is the dependency boundary for management dashboard
// routes. Tenant self-service and workspace routes receive the shared
// DashboardQueryReader through their own explicit modules.
type DashboardHTTPDeps struct {
	Auth                       HTTPAuthDeps
	DashboardQueries           DashboardQueryReader
	IdentityProvider           IdentityProvider
	IdentityEnrichmentFailures IdentityEnrichmentFailureObserver
}

// RegisterDashboard owns the platform-admin authenticated management
// dashboard route group.
func RegisterDashboard(api huma.API, d DashboardHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerDashboard(management, d)
}
