package transport

import "github.com/danielgtaylor/huma/v2"

// APIKeyManagementHTTPDeps is the dependency boundary for platform-managed
// tenant and end-user API-key routes.
type APIKeyManagementHTTPDeps struct {
	Auth            HTTPAuthDeps
	APIKeys         APIKeyReader
	APIKeyWriter    APIKeyWriter
	APIKeyLifecycle APIKeyLifecycleManager
	APIKeySecrets   APIKeySecretManager
	Groups          CommercialGroupCatalog
	LimitPolicies   CommercialLimitPolicyManager
}

// RegisterAPIKeyManagement owns the platform-admin authenticated API-key
// management routes. Tenant self-service keys are registered separately.
func RegisterAPIKeyManagement(api huma.API, d APIKeyManagementHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerAPIKeys(management, d)
}
