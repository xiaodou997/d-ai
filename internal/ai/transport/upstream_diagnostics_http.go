package transport

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// UpstreamDiagnosticsHTTPDeps is the dependency boundary for upstream model
// discovery, model capability inference and account connectivity tests.
type UpstreamDiagnosticsHTTPDeps struct {
	Auth              HTTPAuthDeps
	AccountReader     UpstreamAccountReader
	ModelBindings     UpstreamModelBindingStore
	ProviderSecrets   ProviderSecretCodec
	HTTPClient        HTTPDoer
	AccountHealth     UpstreamAccountHealthWriter
	ModelCapabilities ModelCapabilityResolver
}

// HTTPDoer is the only outbound HTTP capability required by upstream
// diagnostics. Connection pooling, redirects and transport timeouts remain
// owned by the composition root's concrete client.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RegisterUpstreamDiagnostics owns the platform-admin authenticated upstream
// discovery and connectivity route group.
func RegisterUpstreamDiagnostics(api huma.API, d UpstreamDiagnosticsHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerUpstreamDiscovery(management, d)
	registerUpstreamAccountTest(management, d)
}
