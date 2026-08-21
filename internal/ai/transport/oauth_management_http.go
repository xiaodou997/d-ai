package transport

import "github.com/danielgtaylor/huma/v2"

// OAuthManagementHTTPDeps is the complete dependency boundary for OAuth pool
// and credential management routes. Serving and background token refresh keep
// their runtime implementations outside this HTTP module.
type OAuthManagementHTTPDeps struct {
	Auth              HTTPAuthDeps
	CredentialCreator OAuthCredentialCreator
	CredentialReader  OAuthCredentialReader
	CredentialWriter  OAuthCredentialWriter
	PoolReader        OAuthPoolReader
	PoolWriter        OAuthPoolWriter
	PoolHealthReader  OAuthPoolHealthReader
	TokenRefresher    OAuthTokenRefresher
	ClientCatalog     ClientCatalogResolver
	ModelBindings     UpstreamModelBindingStore
}

// RegisterOAuthManagement owns the platform-admin authenticated OAuth pool
// and credential management route group.
func RegisterOAuthManagement(api huma.API, d OAuthManagementHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerOAuthPools(management, d)
}
