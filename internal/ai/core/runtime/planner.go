package runtime

import (
	"context"
	"errors"
	"strings"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

var ErrNoDispatchOption = errors.New("no dispatch option")

// CommercialDispatcher resolves commercial dispatch options for a runtime
// caller before upstream binding resolution.
type CommercialDispatcher interface {
	ResolveDispatch(
		ctx context.Context,
		subject identity.Subject,
		capability catalog.Capability,
		clientSurface surface.ID,
		requestedModel string,
	) ([]commercial.DispatchResolution, error)
}

// DispatchPlan is the runtime-kernel side view of the commercial dispatch
// result. A later phase will turn one option into a concrete upstream binding.
type DispatchPlan struct {
	RequestID      string
	RequestedModel string
	ClientSurface  surface.ID
	Options        []commercial.DispatchResolution
}

// Planner is the first runtime-kernel entrypoint wired to the rebuilt
// commercial layer.
type Planner struct {
	Dispatcher CommercialDispatcher
}

func NewPlanner(dispatcher CommercialDispatcher) *Planner {
	return &Planner{Dispatcher: dispatcher}
}

func (p *Planner) Plan(ctx context.Context, subject identity.Subject, req Request) (DispatchPlan, error) {
	if p == nil || p.Dispatcher == nil {
		return DispatchPlan{}, errors.New("commercial dispatcher is not configured")
	}
	requestedModel := strings.TrimSpace(req.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(req.ResolvedModelID)
	}
	if requestedModel == "" {
		return DispatchPlan{}, errors.New("requested model is required")
	}
	if !surface.IsKnown(req.ClientSurface) {
		return DispatchPlan{}, errors.New("client surface is required")
	}
	if groupID := strings.TrimSpace(req.ForcedGroupID); groupID != "" {
		subject.ForcedGroupID = groupID
	}
	options, err := p.Dispatcher.ResolveDispatch(ctx, subject, req.Capability, req.ClientSurface, requestedModel)
	if err != nil {
		return DispatchPlan{}, err
	}
	if len(options) == 0 {
		return DispatchPlan{}, ErrNoDispatchOption
	}
	return DispatchPlan{
		RequestID:      req.RequestID,
		RequestedModel: requestedModel,
		ClientSurface:  req.ClientSurface,
		Options:        options,
	}, nil
}
