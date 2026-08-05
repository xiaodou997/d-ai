package runtime

import (
	"context"
	"errors"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/identity"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
)

var ErrNoRouteCandidates = errors.New("no executable route candidates")
var ErrNoAllowedGroup = errors.New("no allowed group in dispatch plan")

// NoRouteError carries why every candidate target was rejected.
//
// Without it, "every upstream was ruled out" and "the master key cannot decrypt
// this account's credential" collapse into the same bare ErrNoRouteCandidates,
// and the server keeps no record of which it was — the caller sees a 503 and
// operators have nothing to go on. The rejection reasons are computed anyway
// (see Resolver.inspect); this type just stops them from being thrown away.
//
// It unwraps to ErrNoRouteCandidates, so existing errors.Is checks and the
// client-facing error mapping are unaffected.
type NoRouteError struct {
	Rejections []RejectedTarget
}

func (e *NoRouteError) Error() string { return ErrNoRouteCandidates.Error() }

func (e *NoRouteError) Unwrap() error { return ErrNoRouteCandidates }

// Resolver is the first runtime-kernel execution-preparation service. It takes
// a request through commercial planning and resolves all usable upstream
// targets in failover order.
type Resolver struct {
	Planner *Planner
	Binder  coreupstream.RuntimeBindingResolver
}

type pendingBinding struct {
	groupRank  int
	option     commercial.DispatchResolution
	target     commercial.GroupTarget
	bindingReq coreupstream.RuntimeBindingRequest
}

type inspectionResult struct {
	inspection   RouteInspection
	firstBindErr error
}

func NewResolver(planner *Planner, binder coreupstream.RuntimeBindingResolver) *Resolver {
	return &Resolver{Planner: planner, Binder: binder}
}

func (r *Resolver) Resolve(ctx context.Context, subject identity.Subject, req Request) (RoutePlan, error) {
	result, err := r.inspect(ctx, subject, req)
	if err != nil {
		return RoutePlan{}, err
	}
	if result.firstBindErr != nil && len(result.inspection.Candidates) == 0 {
		return RoutePlan{}, result.firstBindErr
	}
	if len(result.inspection.Candidates) == 0 {
		return RoutePlan{}, &NoRouteError{Rejections: result.inspection.RejectedCandidates}
	}
	return RoutePlan{
		RequestID:  result.inspection.RequestID,
		Candidates: result.inspection.Candidates,
	}, nil
}

// Inspect runs the exact commercial planner and runtime binder used by Resolve
// but retains safe rejection reasons so a preview can explain an empty route
// without turning a valid configuration state into an internal server error.
func (r *Resolver) Inspect(ctx context.Context, subject identity.Subject, req Request) (RouteInspection, error) {
	result, err := r.inspect(ctx, subject, req)
	if err != nil {
		return RouteInspection{}, err
	}
	if result.firstBindErr != nil && len(result.inspection.Candidates) == 0 {
		return RouteInspection{}, result.firstBindErr
	}
	return result.inspection, nil
}

func (r *Resolver) inspect(ctx context.Context, subject identity.Subject, req Request) (inspectionResult, error) {
	if r == nil || r.Planner == nil || r.Binder == nil {
		return inspectionResult{}, errors.New("runtime resolver is not fully configured")
	}
	plan, err := r.Planner.Plan(ctx, subject, req)
	if err != nil {
		return inspectionResult{}, err
	}
	inspection := RouteInspection{RequestID: req.RequestID, Candidates: make([]PlannedTarget, 0), RejectedCandidates: make([]RejectedTarget, 0)}
	allowed := stringSet(req.AllowedGroupIDs)
	matchedAllowedGroup := len(allowed) == 0
	pending := make([]pendingBinding, 0)
	for groupRank, option := range plan.Options {
		if len(allowed) > 0 {
			if _, ok := allowed[option.Group.Group.ID]; !ok {
				continue
			}
			matchedAllowedGroup = true
		}
		if len(option.Targets) == 0 {
			inspection.RejectedCandidates = append(inspection.RejectedCandidates, RejectedTarget{
				GroupRank:   groupRank,
				Group:       option.Group,
				ModelID:     option.ResolvedModelID,
				MatchedRule: option.MatchedRule,
				Code:        RejectionNoActiveTarget,
				Detail:      "no active target is linked to this group",
			})
			continue
		}
		for _, target := range option.Targets {
			bindingReq, bindReqErr := buildRuntimeBindingRequest(subject.TenantID, req, option, target)
			if bindReqErr != nil {
				inspection.RejectedCandidates = append(inspection.RejectedCandidates, RejectedTarget{
					RouteID: target.ID, GroupRank: groupRank, Group: option.Group, Target: target,
					ModelID: option.ResolvedModelID, MatchedRule: option.MatchedRule,
					Code: RejectionBindingInvalid, Detail: "target configuration is invalid",
				})
				continue
			}
			pending = append(pending, pendingBinding{
				groupRank: groupRank, option: option, target: target, bindingReq: bindingReq,
			})
		}
	}
	if !matchedAllowedGroup {
		return inspectionResult{}, ErrNoAllowedGroup
	}
	bindings := r.resolveBindings(ctx, pending)
	var firstBindErr error
	for index, item := range pending {
		bindingResult := bindings[index]
		if bindingResult.err == nil {
			inspection.Candidates = append(inspection.Candidates, PlannedTarget{
				RouteID:     item.target.ID,
				GroupRank:   item.groupRank,
				Group:       item.option.Group,
				Target:      item.target,
				ModelID:     item.option.ResolvedModelID,
				MatchedRule: item.option.MatchedRule,
				Binding:     bindingResult.binding,
			})
			continue
		}
		if rejection, ok := coreupstream.RuntimeBindingRejectionFromError(bindingResult.err); ok {
			inspection.RejectedCandidates = append(inspection.RejectedCandidates, RejectedTarget{
				RouteID: item.target.ID, GroupRank: item.groupRank, Group: item.option.Group,
				Target: item.target, ModelID: item.option.ResolvedModelID, MatchedRule: item.option.MatchedRule,
				Code: RejectionCode(rejection.Code), Detail: rejection.Detail,
			})
		}
		if !errors.Is(bindingResult.err, coreupstream.ErrNoRuntimeBinding) && firstBindErr == nil {
			firstBindErr = bindingResult.err
		}
	}
	return inspectionResult{inspection: inspection, firstBindErr: firstBindErr}, nil
}

func (r *Resolver) resolveBindings(ctx context.Context, pending []pendingBinding) []bindingResolution {
	requests := make([]coreupstream.RuntimeBindingRequest, len(pending))
	for index, item := range pending {
		requests[index] = item.bindingReq
	}
	if batch, ok := r.Binder.(interface {
		resolveRuntimeBindings(context.Context, []coreupstream.RuntimeBindingRequest) []bindingResolution
	}); ok {
		return batch.resolveRuntimeBindings(ctx, requests)
	}
	results := make([]bindingResolution, len(requests))
	for index, req := range requests {
		results[index].binding, results[index].err = r.Binder.ResolveRuntimeBinding(ctx, req)
	}
	return results
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func buildRuntimeBindingRequest(tenantID string, req Request, option commercial.DispatchResolution, target commercial.GroupTarget) (coreupstream.RuntimeBindingRequest, error) {
	mode, err := commercialTargetMode(target.TargetKind)
	if err != nil {
		return coreupstream.RuntimeBindingRequest{}, err
	}
	return coreupstream.RuntimeBindingRequest{
		TenantID:                tenantID,
		Capability:              req.Capability,
		ClientSurface:           req.ClientSurface,
		RequestedModel:          option.RequestedModel,
		ResolvedModelID:         option.ResolvedModelID,
		Stream:                  req.Stream,
		AllowProtocolConversion: option.Group.Group.AllowProtocolConversion,
		TargetMode:              mode,
		TargetID:                target.TargetID,
		Priority:                target.Priority,
	}, nil
}

func commercialTargetMode(kind commercial.TargetKind) (coreupstream.AccessMode, error) {
	switch kind {
	case commercial.TargetKindDirectUpstream:
		return coreupstream.AccessModeDirect, nil
	case commercial.TargetKindOAuthPool:
		return coreupstream.AccessModeOAuthPool, nil
	default:
		return "", ErrNoRouteCandidates
	}
}
