package runtime

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

func TestInvokeExpanderExpandAppTarget(t *testing.T) {
	t.Parallel()

	expander := NewInvokeExpander(&invokeRuntimeResolverStub{
		invocation: application.RuntimeInvocation{
			InvokeKey: application.InvokeKey{
				ID:         "ik-2",
				OwnerScope: identity.ScopeUser,
				TenantID:   "tenant-a",
				UserID:     "user-a",
				Status:     application.StatusActive,
				AppID:      "app-1",
			},
			BoundModelID: "gpt-agent",
			App: &application.RuntimeApp{
				App: application.App{
					ID:            "app-1",
					OwnerScope:    identity.ScopeTenant,
					OwnerTenantID: "tenant-a",
					BoundModelID:  "gpt-agent",
					GroupID:       "group-agent",
				},
			},
		},
	})

	got, err := expander.ExpandByKeyHash(context.Background(), "hash-2", Request{
		RequestID:     "req-2",
		Capability:    catalog.CapabilityChat,
		ClientSurface: surface.AnthropicMessages,
	})
	if err != nil {
		t.Fatalf("ExpandByKeyHash: %v", err)
	}
	if got.Request.AppID != "app-1" {
		t.Fatalf("app id = %q", got.Request.AppID)
	}
	if got.Request.GroupID != "group-agent" {
		t.Fatalf("group id = %q", got.Request.GroupID)
	}
	if got.Request.ForcedGroupID != "group-agent" || got.Subject.ForcedGroupID != "group-agent" {
		t.Fatalf("forced group id request=%q subject=%q", got.Request.ForcedGroupID, got.Subject.ForcedGroupID)
	}
	if got.Subject.AppOwnerType != string(identity.ScopeTenant) || got.Subject.AppOwnerTenantID != "tenant-a" {
		t.Fatalf("app owner snapshot = %#v", got.Subject)
	}
	if got.Subject.Scope != identity.ScopeUser || got.Subject.UserID != "user-a" {
		t.Fatalf("subject = %#v", got.Subject)
	}
}

func TestInvokeExpanderPropagatesResolverError(t *testing.T) {
	t.Parallel()

	expander := NewInvokeExpander(&invokeRuntimeResolverStub{err: errors.New("boom")})
	_, err := expander.ExpandByKeyHash(context.Background(), "hash", Request{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected resolver error, got %v", err)
	}
}

func TestInvokeExpanderExpandByKeyID(t *testing.T) {
	t.Parallel()

	resolver := &invokeRuntimeResolverStub{invocation: application.RuntimeInvocation{
		InvokeKey: application.InvokeKey{
			ID: "invoke-1", OwnerScope: identity.ScopeUser, TenantID: "tenant-a", UserID: "user-a",
		},
		BoundModelID: "gpt-image-1",
		App: &application.RuntimeApp{App: application.App{
			ID: "app-1", AppType: application.AppTypeImageGenerationAgent,
			BoundModelID: "gpt-image-1", GroupID: "group-image",
		}},
	}}
	expander := NewInvokeExpander(resolver)

	got, err := expander.ExpandByKeyID(context.Background(), identity.ScopeUser, "tenant-a", "user-a", "invoke-1", Request{})
	if err != nil {
		t.Fatalf("ExpandByKeyID: %v", err)
	}
	if resolver.keyID != "invoke-1" || resolver.ownerScope != identity.ScopeUser || resolver.tenantID != "tenant-a" || resolver.userID != "user-a" {
		t.Fatalf("resolver lookup = scope %q tenant %q user %q id %q", resolver.ownerScope, resolver.tenantID, resolver.userID, resolver.keyID)
	}
	if got.Subject.InvokeKeyID != "invoke-1" || got.Subject.ForcedGroupID != "group-image" || got.App.App.AppType != application.AppTypeImageGenerationAgent {
		t.Fatalf("expanded invocation = %+v", got)
	}
}

type invokeRuntimeResolverStub struct {
	invocation application.RuntimeInvocation
	err        error
	ownerScope identity.Scope
	tenantID   string
	userID     string
	keyID      string
}

func (s *invokeRuntimeResolverStub) ResolveRuntimeInvocationByKeyHash(context.Context, string) (application.RuntimeInvocation, error) {
	if s.err != nil {
		return application.RuntimeInvocation{}, s.err
	}
	return s.invocation, nil
}

func (s *invokeRuntimeResolverStub) ResolveRuntimeInvocationByID(_ context.Context, ownerScope identity.Scope, tenantID, userID, keyID string) (application.RuntimeInvocation, error) {
	s.ownerScope, s.tenantID, s.userID, s.keyID = ownerScope, tenantID, userID, keyID
	if s.err != nil {
		return application.RuntimeInvocation{}, s.err
	}
	return s.invocation, nil
}
