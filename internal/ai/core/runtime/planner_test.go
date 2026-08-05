package runtime

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

func TestPlannerPlanBuildsDispatchPlan(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{
			{
				Group: commercial.AccessibleGroup{
					Group: commercial.Group{ID: "g1", Name: "Main"},
				},
				RequestedModel:  "opus-4.7",
				ResolvedModelID: "gpt-5.4",
			},
		},
	}
	planner := NewPlanner(dispatcher)

	plan, err := planner.Plan(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		RequestID:      "req-1",
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.AnthropicMessages,
		RequestedModel: "opus-4.7",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.RequestID != "req-1" {
		t.Fatalf("request id = %q", plan.RequestID)
	}
	if len(plan.Options) != 1 || plan.Options[0].ResolvedModelID != "gpt-5.4" {
		t.Fatalf("options = %#v", plan.Options)
	}
}

func TestPlannerPlanReturnsNoDispatchOption(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(&runtimeDispatcherStub{})
	_, err := planner.Plan(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "gpt-5",
	})
	if err != ErrNoDispatchOption {
		t.Fatalf("expected ErrNoDispatchOption, got %v", err)
	}
}

func TestPlannerPlanAppliesForcedGroupOnly(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{
			{
				Group: commercial.AccessibleGroup{
					Group: commercial.Group{ID: "group-agent", Name: "Agent group"},
				},
				RequestedModel:  "gpt-agent",
				ResolvedModelID: "gpt-agent",
			},
		},
	}
	planner := NewPlanner(dispatcher)

	_, err := planner.Plan(context.Background(), identity.Subject{TenantID: "tenant-1", GroupID: "other-group"}, Request{
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "gpt-agent",
		ForcedGroupID:  "group-agent",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if dispatcher.lastSubject.ForcedGroupID != "group-agent" {
		t.Fatalf("subject forced group id = %q", dispatcher.lastSubject.ForcedGroupID)
	}
	if dispatcher.lastSubject.GroupID != "other-group" {
		t.Fatalf("subject group id should be untouched, got %q", dispatcher.lastSubject.GroupID)
	}
}

func TestPlannerPlanDoesNotForcePlainRequestGroup(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{
			{
				Group: commercial.AccessibleGroup{
					Group: commercial.Group{ID: "group-visible", Name: "Visible group"},
				},
				RequestedModel:  "gpt",
				ResolvedModelID: "gpt",
			},
		},
	}
	planner := NewPlanner(dispatcher)

	_, err := planner.Plan(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "gpt",
		GroupID:        "client-supplied-group",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if dispatcher.lastSubject.ForcedGroupID != "" {
		t.Fatalf("plain group id must not force bypass, got %q", dispatcher.lastSubject.ForcedGroupID)
	}
}

type runtimeDispatcherStub struct {
	options     []commercial.DispatchResolution
	err         error
	lastSubject identity.Subject
}

func (s *runtimeDispatcherStub) ResolveDispatch(
	ctx context.Context,
	subject identity.Subject,
	capability catalog.Capability,
	clientSurface surface.ID,
	requestedModel string,
) ([]commercial.DispatchResolution, error) {
	_, _, _ = ctx, capability, clientSurface
	_ = requestedModel
	s.lastSubject = subject
	return append([]commercial.DispatchResolution(nil), s.options...), s.err
}
