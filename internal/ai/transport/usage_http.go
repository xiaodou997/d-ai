package transport

import "github.com/danielgtaylor/huma/v2"
import "xiaodou/dai/internal/ai/domain"

// UsageHTTPDeps is the dependency boundary for management usage query
// routes. Tenant, end-user and workspace routes receive shared query ports
// through their own explicit modules.
type UsageHTTPDeps struct {
	Records                    domain.RequestRecordRepository
	Auth                       HTTPAuthDeps
	UsageQueries               UsageQueryReader
	IdentityProvider           IdentityProvider
	IdentityEnrichmentFailures IdentityEnrichmentFailureObserver
}

// RegisterUsage owns the platform-admin authenticated management usage route
// group.
func RegisterUsage(api huma.API, d UsageHTTPDeps) {
	registerRequestRecords(api, d)
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerUsage(management, d)
}
