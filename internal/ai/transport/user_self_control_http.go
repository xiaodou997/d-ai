package transport

import "github.com/danielgtaylor/huma/v2"

// UserSelfControlHTTPDeps is the dependency boundary for end-user API-key
// and API-key limit-policy controls. Ownership remains claims-scoped inside
// the handlers, while the module owns the userType=4 authentication group.
type UserSelfControlHTTPDeps struct {
	Auth            HTTPAuthDeps
	APIKeys         APIKeyReader
	APIKeyWriter    APIKeyWriter
	APIKeyLifecycle APIKeyLifecycleManager
	APIKeySecrets   APIKeySecretManager
	Groups          CommercialGroupCatalog
	LimitPolicies   CommercialLimitPolicyManager
}

// RegisterUserSelfControl owns end-user API-key and limit-policy routes.
func RegisterUserSelfControl(api huma.API, d UserSelfControlHTTPDeps) {
	user := huma.NewGroup(api)
	user.UseMiddleware(endUserSensitiveAuth(api, d.Auth))
	registerUserSelfAPIKeys(user, d)
	registerUserSelfLimits(user, d)
}
