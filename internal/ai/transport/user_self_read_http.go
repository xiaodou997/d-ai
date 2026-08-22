package transport

import "github.com/danielgtaylor/huma/v2"

// UserSelfReadHTTPDeps is the dependency boundary for end-user group,
// model-grant and usage projections. All returned data is scoped from JWT
// claims by the handlers; this module only owns the userType=4 read group.
type UserSelfReadHTTPDeps struct {
	Auth          HTTPAuthDeps
	Groups        CommercialGroupCatalog
	ModelCatalog  ModelCatalogReader
	UserUsageLogs UserUsageLogReader
	UsageQueries  UsageQueryReader
}

// RegisterUserSelfRead owns end-user group, model-grant and usage routes.
func RegisterUserSelfRead(api huma.API, d UserSelfReadHTTPDeps) {
	user := huma.NewGroup(api)
	user.UseMiddleware(endUserAuth(api, d.Auth))
	registerUserSelfGroups(user, d)
	registerUserSelfModelGrants(user, d)
	registerUserSelfUsage(user, d)
}
