package transport

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/routing"
)

// SystemHTTPDeps is the dependency boundary for system status.
type SystemHTTPDeps struct {
	Auth           HTTPAuthDeps
	DatabaseHealth ComponentHealthProbe
	RedisHealth    ComponentHealthProbe
	Health         routing.HealthTracker
}

// ComponentHealthProbe is the minimal infrastructure check needed by the
// system status endpoint. Client-specific command types stay in adapters.
type ComponentHealthProbe interface {
	Check(ctx context.Context) error
}

// RegisterSystem owns the platform-admin authenticated system route group.
func RegisterSystem(api huma.API, d SystemHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerSystem(management, d)
}
