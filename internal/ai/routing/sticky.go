package routing

import "context"

// StickyBinding describes which upstream target a conversation is pinned to.
type StickyBinding struct {
	// TargetKind is "account" or "credential".
	TargetKind string

	// Account route field (valid when TargetKind == "account"): the upstream
	// account id (= RouteCandidate.EndpointID).
	EndpointID string

	// Pool route field (valid when TargetKind == "credential").
	CredentialID string

	// RouteID identifies which group→target binding this came from, so the
	// reader can verify the bound target is still a valid candidate.
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
