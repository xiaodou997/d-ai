package transport

import "github.com/danielgtaylor/huma/v2"

// OverviewHTTPDeps is the dependency boundary for the platform-admin
// operating overview snapshot. It intentionally composes the existing
// aggregate query ports so the HTTP contract is a single consistent read.
type OverviewHTTPDeps struct {
	Auth                       HTTPAuthDeps
	DashboardQueries           DashboardQueryReader
	UsageQueries               UsageQueryReader
	IdentityProvider           IdentityProvider
	IdentityEnrichmentFailures IdentityEnrichmentFailureObserver
}

// RegisterOverview owns the platform-admin authenticated overview snapshot.
func RegisterOverview(api huma.API, d OverviewHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerOverview(management, d)
}
