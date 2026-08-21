package transport

import "github.com/danielgtaylor/huma/v2"

// ModelBindingHTTPDeps is the dependency boundary for account and pool model
// binding management routes. Discovery, connectivity and transfer workflows
// keep using their shared ports from the core AI dependencies.
type ModelBindingHTTPDeps struct {
	Auth          HTTPAuthDeps
	AccountReader UpstreamAccountReader
	PoolReader    OAuthPoolReader
	ModelBindings UpstreamModelBindingStore
}

// RegisterModelBindings owns the platform-admin authenticated model-binding
// route groups for direct upstream accounts and OAuth pools.
func RegisterModelBindings(api huma.API, d ModelBindingHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerUpstreamModelBindings(management, d)
}
