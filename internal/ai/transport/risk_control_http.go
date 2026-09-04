package transport

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/promptaudit"
)

// RiskControlHTTPDeps is the complete dependency boundary for risk-control
// management routes.
type RiskControlHTTPDeps struct {
	Auth                HTTPAuthDeps
	ProviderSecrets     ProviderSecretCodec
	RiskControlConfig   RiskControlConfigStore
	RiskControlDetector RiskControlDetector
	RiskControlLogs     RiskControlLogReader
	RiskEvents          RiskEventManager
	PromptAuditConfig   PromptAuditConfigStore
	PromptAuditProbe    PromptAuditProber
	PromptAuditEvents   PromptAuditEventReader
	PromptAuditRuntime  PromptAuditRuntimeReader
}

type PromptAuditConfigStore interface {
	Get(context.Context) (promptaudit.Config, error)
	Update(context.Context, promptaudit.Config) error
}

type PromptAuditProber interface {
	Probe(context.Context, promptaudit.Endpoint, string) (*promptaudit.Result, error)
}

type PromptAuditEventReader interface {
	ListPromptAuditEvents(context.Context, promptaudit.EventFilter) (promptaudit.EventPage, error)
	DeletePromptAuditEvent(context.Context, string) (bool, error)
}

type PromptAuditRuntimeReader interface {
	Runtime(context.Context) promptaudit.Runtime
}

// RiskControlConfigStore owns the mutable moderation configuration.
type RiskControlConfigStore interface {
	Get(ctx context.Context) (domain.RiskControlConfig, error)
	Update(ctx context.Context, config domain.RiskControlConfig) error
}

// RiskControlDetector executes a non-persisting moderation check for the
// management test endpoint. Implementations may still use an in-memory cache.
type RiskControlDetector interface {
	Detect(ctx context.Context, config domain.RiskControlConfig, text string) domain.RiskControlDetection
}

// RiskControlLogReader exposes paginated moderation audit history.
type RiskControlLogReader interface {
	List(ctx context.Context, filter domain.ContentModerationLogFilter, limit, offset int32) (domain.ContentModerationLogPage, error)
}

// RiskEventManager exposes the human-review queue and its state transition.
type RiskEventManager interface {
	List(ctx context.Context, filter domain.RiskEventFilter, limit, offset int32) (domain.RiskEventPage, error)
	Resolve(ctx context.Context, id, status, resolvedBy, note string) (domain.RiskEvent, error)
}

// RegisterRiskControl owns the platform-admin authenticated risk-control
// route group.
func RegisterRiskControl(api huma.API, d RiskControlHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerRiskControl(management, d)
	registerPromptAudit(management, d)
}
