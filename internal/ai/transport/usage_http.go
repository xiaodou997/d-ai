package transport

import "github.com/danielgtaylor/huma/v2"

// UsageHTTPDeps is the dependency boundary for management usage query
// routes. Tenant, end-user and workspace routes continue to receive the
// shared query ports through the core AI dependencies.
type UsageHTTPDeps struct {
	Auth                       HTTPAuthDeps
	UsageQueries               UsageQueryReader
	IdentityProvider           IdentityProvider
	IdentityEnrichmentFailures IdentityEnrichmentFailureObserver
}

// RegisterUsage owns the platform-admin authenticated management usage route
// group.
func RegisterUsage(api huma.API, d UsageHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerUsage(management, d)
}
