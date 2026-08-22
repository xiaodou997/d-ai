package transport

import "github.com/danielgtaylor/huma/v2"

// TenantSelfControlHTTPDeps is the dependency boundary for tenant-owned API
// keys and tenant-scoped concurrency policies. Dashboard and usage reads stay
// in the tenant self-service core because they share query projections with
// workspace and user surfaces.
type TenantSelfControlHTTPDeps struct {
	Auth            HTTPAuthDeps
	APIKeys         APIKeyReader
	APIKeyWriter    APIKeyWriter
	APIKeyLifecycle APIKeyLifecycleManager
	APIKeySecrets   APIKeySecretManager
	Groups          CommercialGroupCatalog
	LimitPolicies   CommercialLimitPolicyManager
	TenantEndUsers  TenantEndUserVerifier
}

// RegisterTenantSelfControl owns the tenant-user authenticated API-key and
// limit-policy management routes.
func RegisterTenantSelfControl(api huma.API, d TenantSelfControlHTTPDeps) {
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, d.Auth))
	registerTenantSelfAPIKeys(tenant, d)
	registerTenantSelfAPIKeyWrites(tenant, d)
	registerTenantSelfLimits(tenant, d)
}
