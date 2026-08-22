package transport

import (
	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/workspace"
)

// WorkspaceHTTPDeps is the shared dependency boundary for tenant and
// end-user workspace routes. The module owns both auth groups while handlers
// keep scope derived from JWT claims.
type WorkspaceHTTPDeps struct {
	TenantAuth        HTTPAuthDeps
	UserAuth          HTTPAuthDeps
	WorkspaceOverview workspace.OverviewReader
	WorkspaceModels   workspace.ChatModelReader
	WorkspaceSessions workspace.ChatSessionReader
	WorkspaceManager  workspace.ChatSessionManager
	WorkspaceImages   workspace.ImageJobReader
	DashboardQueries  DashboardQueryReader
	UsageQueries      UsageQueryReader
}

// RegisterWorkspace owns tenant and end-user authenticated workspace routes.
func RegisterWorkspace(api huma.API, d WorkspaceHTTPDeps) {
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, d.TenantAuth))
	registerTenantSelfWorkspace(tenant, d)

	user := huma.NewGroup(api)
	user.UseMiddleware(endUserAuth(api, d.UserAuth))
	registerUserSelfWorkspace(user, d)
}
