package routing

import "context"

// StickyBinding describes which upstream target a conversation is pinned to.
type StickyBinding struct {
	// TargetKind is "deployment" or "credential".
	TargetKind string

	// Deployment route fields (valid when TargetKind == "deployment").
	DeploymentID string
	EndpointID   string

	// Pool route field (valid when TargetKind == "credential").
	CredentialID string

	// RouteID identifies which model route this binding belongs to, so
	// the reader can verify the bound route is still a valid candidate.
	RouteID string
}

// StickyStore reads and writes conversation → upstream target bindings.
// Implementations must be safe for concurrent use.
type StickyStore interface {
	// GetBinding returns the stored binding for a conversation, or (nil, nil)
	// when none is found.
	GetBinding(ctx context.Context, tenantID, identity, model, convID string) (*StickyBinding, error)
	// SetBinding persists a binding (typically 24 h TTL).
	SetBinding(ctx context.Context, tenantID, identity, model, convID string, b *StickyBinding) error
	// DeleteBinding removes a stale binding (called when the target becomes
	// unhealthy or the credential is invalidated).
	DeleteBinding(ctx context.Context, tenantID, identity, model, convID string) error
}
