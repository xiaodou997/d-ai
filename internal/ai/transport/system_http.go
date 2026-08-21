package transport

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/serving"
)

// SystemHTTPDeps is the complete dependency boundary for system status and
// route-weight management endpoints.
type SystemHTTPDeps struct {
	Auth           HTTPAuthDeps
	DatabaseHealth ComponentHealthProbe
	RedisHealth    ComponentHealthProbe
	Health         routing.HealthTracker
	Weights        ScoreWeightsStore
}

// ComponentHealthProbe is the minimal infrastructure check needed by the
// system status endpoint. Client-specific command types stay in adapters.
type ComponentHealthProbe interface {
	Check(ctx context.Context) error
}

// ScoreWeightsStore is the minimal port required by the system endpoints.
// PostgreSQL-backed caching and persistence belong to the adapter package;
// transport only needs to read and update effective weights.
type ScoreWeightsStore interface {
	Get(ctx context.Context, scope string) serving.ScoreWeights
	Upsert(ctx context.Context, scope string, weights serving.ScoreWeights) error
}

// RegisterSystem owns the platform-admin authenticated system route group.
func RegisterSystem(api huma.API, d SystemHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerSystem(management, d)
}
