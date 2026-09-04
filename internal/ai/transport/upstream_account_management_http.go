package transport

import (
	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/routing"
)

// UpstreamAccountManagementHTTPDeps is the complete dependency boundary for
// direct upstream-account CRUD and portable transfer workflows. Connectivity,
// model discovery and explicit binding management are separate HTTP modules.
type UpstreamAccountManagementHTTPDeps struct {
	Auth            HTTPAuthDeps
	Accounts        UpstreamAccountCatalog
	AccountManager  UpstreamAccountManager
	EndpointManager UpstreamAccountEndpointManager
	AccountReader   UpstreamAccountReader
	ProviderSecrets ProviderSecretCodec
	ModelBindings   UpstreamModelBindingStore
	PriceBooks      PriceBookReader
	AdminAudit      AdminAuditRecorder
	RuntimeHealth   routing.HealthTracker
}

// RegisterUpstreamAccountManagement owns the platform-admin authenticated
// direct upstream-account CRUD and transfer route group.
func RegisterUpstreamAccountManagement(api huma.API, d UpstreamAccountManagementHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerUpstreamAccounts(management, d)
}
