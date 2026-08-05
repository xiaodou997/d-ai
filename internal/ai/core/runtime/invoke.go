package runtime

import (
	"context"
	"errors"
	"strings"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/core/identity"
)

// InvokeExpansion is the resolved invoke-key entrypoint state before runtime
// planning starts.
type InvokeExpansion struct {
	Subject    identity.Subject
	Request    Request
	InvokeKey  application.InvokeKey
	App        *application.RuntimeApp
	BoundModel string
}

// InvocationResolver resolves invoke keys and their optional app targets from
// the application layer.
type InvocationResolver interface {
	ResolveRuntimeInvocationByKeyHash(ctx context.Context, keyHash string) (application.RuntimeInvocation, error)
}

// InvokeExpander bridges the application layer to the runtime kernel request
// descriptor.
type InvokeExpander struct {
	Resolver InvocationResolver
}

func NewInvokeExpander(resolver InvocationResolver) *InvokeExpander {
	return &InvokeExpander{Resolver: resolver}
}

func (e *InvokeExpander) ExpandByKeyHash(ctx context.Context, keyHash string, req Request) (InvokeExpansion, error) {
	if e == nil || e.Resolver == nil {
		return InvokeExpansion{}, errors.New("invoke resolver is not configured")
	}
	keyHash = strings.TrimSpace(keyHash)
	if keyHash == "" {
		return InvokeExpansion{}, errors.New("invoke key hash is required")
	}
	invocation, err := e.Resolver.ResolveRuntimeInvocationByKeyHash(ctx, keyHash)
	if err != nil {
		return InvokeExpansion{}, err
	}
	return expandInvocation(invocation, req)
}

// ExpandByKeyID reloads a persisted invoke-key reference. It is intentionally
// separate from the hash path so queued work never stores or reconstructs the
// caller's plaintext rk_ credential.
func (e *InvokeExpander) ExpandByKeyID(
	ctx context.Context,
	ownerScope identity.Scope,
	tenantID, userID, keyID string,
	req Request,
) (InvokeExpansion, error) {
	if e == nil || e.Resolver == nil {
		return InvokeExpansion{}, errors.New("invoke resolver is not configured")
	}
	resolver, ok := e.Resolver.(application.RuntimeIDResolver)
	if !ok {
		return InvokeExpansion{}, errors.New("invoke resolver does not support lookup by id")
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return InvokeExpansion{}, errors.New("invoke key id is required")
	}
	invocation, err := resolver.ResolveRuntimeInvocationByID(ctx, ownerScope, tenantID, userID, keyID)
	if err != nil {
		return InvokeExpansion{}, err
	}
	return expandInvocation(invocation, req)
}

func expandInvocation(invocation application.RuntimeInvocation, req Request) (InvokeExpansion, error) {
	if strings.TrimSpace(invocation.BoundModelID) == "" {
		return InvokeExpansion{}, errors.New("invoke target has no bound model")
	}
	req.InvokeKeyID = invocation.InvokeKey.ID
	req.ResolvedModelID = invocation.BoundModelID
	if strings.TrimSpace(req.RequestedModel) == "" {
		req.RequestedModel = invocation.BoundModelID
	}
	subject := invocation.Subject()
	if invocation.App != nil {
		req.AppID = invocation.App.App.ID
		req.GroupID = strings.TrimSpace(invocation.App.App.GroupID)
		req.ForcedGroupID = req.GroupID
		subject.ForcedGroupID = req.ForcedGroupID
		subject.AppID = invocation.App.App.ID
		subject.AppName = invocation.App.App.Name
		subject.AppOwnerType = string(invocation.App.App.OwnerScope)
		subject.AppOwnerTenantID = invocation.App.App.OwnerTenantID
		subject.AppOwnerUserID = invocation.App.App.OwnerUserID
	}
	return InvokeExpansion{
		Subject:    subject,
		Request:    req,
		InvokeKey:  invocation.InvokeKey,
		App:        invocation.App,
		BoundModel: invocation.BoundModelID,
	}, nil
}
