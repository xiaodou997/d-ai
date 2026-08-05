package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/commercial"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/runtimecompat"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
)

type routePlanner interface {
	Resolve(context.Context, coreidentity.Subject, coreruntime.Request) (coreruntime.RoutePlan, error)
}

// RuntimeRouteSelector is the only adapter from commercial route planning to
// serving candidates. It runs after subscription gating inside the pipeline.
type RuntimeRouteSelector struct {
	planner routePlanner
	logger  *zap.Logger
}

func NewRuntimeRouteSelector(planner routePlanner, logger *zap.Logger) *RuntimeRouteSelector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RuntimeRouteSelector{planner: planner, logger: logger}
}

func (s *RuntimeRouteSelector) SelectCandidates(ctx context.Context, req *serving.Request) ([]*domain.RouteCandidate, error) {
	if s == nil || s.planner == nil {
		return nil, errors.New("runtime route planner is not configured")
	}
	subject := req.RuntimeSubject()
	if subject == nil {
		return nil, errors.New("missing runtime subject")
	}
	capability := req.RuntimeCapability
	if capability == "" {
		capability = runtimecompat.CapabilityToCore(req.CapabilityType)
	}
	clientSurface, err := runtimecompat.ProtocolToSurfaceForCapability(req.ClientProtocol, capability)
	if err != nil {
		return nil, err
	}
	planReq := coreruntime.Request{
		RequestID:       req.RequestID,
		TraceID:         req.TraceID,
		Capability:      capability,
		ClientSurface:   clientSurface,
		RequestedModel:  req.RequestedModel,
		ResolvedModelID: req.ModelCode,
		Body:            requestBody(req),
		Stream:          req.IsStream,
		ReceivedAt:      req.StartedAt,
	}
	if req.BillingSource == subscription.BillingSourceSubscription {
		planReq.AllowedGroupIDs = mapKeys(req.SubscriptionGroupQuotaDebitMultipliers)
	}

	plan, err := s.planner.Resolve(ctx, *subject, planReq)
	if errors.Is(err, coreruntime.ErrNoAllowedGroup) {
		// The caller can access routes, but none belongs to the subscription plan.
		// Preserve the established contract by switching this request to PAYG and
		// resolving again without the subscription membership constraint.
		req.BillingSource = subscription.BillingSourcePayg
		req.SubscriptionID = ""
		req.SubscriptionGroupQuotaDebitMultipliers = nil
		planReq.AllowedGroupIDs = nil
		plan, err = s.planner.Resolve(ctx, *subject, planReq)
	}
	if err != nil {
		s.logRouteFailure(req, subject, err)
		return nil, routePlanningError(err)
	}

	candidates := make([]*domain.RouteCandidate, 0, len(plan.Candidates))
	for _, planned := range plan.Candidates {
		candidate, convertErr := plannedCandidate(planned, planReq)
		if convertErr != nil {
			return nil, convertErr
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func plannedCandidate(planned coreruntime.PlannedTarget, req coreruntime.Request) (*domain.RouteCandidate, error) {
	binding := planned.Binding
	modelBinding := binding.ModelBinding
	up := binding.Upstream
	providerProtocol, err := runtimecompat.SurfaceToProtocolForCapability(modelBinding.RequestSurface, req.Capability)
	if err != nil {
		return nil, err
	}
	upstreamModel := modelBinding.UpstreamModelName
	if upstreamModel == "" {
		upstreamModel = planned.ModelID
	}
	group := planned.Group.Group
	cand := &domain.RouteCandidate{
		RouteID:                      planned.RouteID,
		GroupRank:                    planned.GroupRank,
		Priority:                     planned.Target.Priority,
		SupportsStream:               true,
		ModelCode:                    planned.ModelID,
		CapabilityType:               runtimecompat.CapabilityFromCore(req.Capability),
		RequestedModel:               firstNonEmptyRuntime(req.RequestedModel, planned.ModelID),
		ResolvedProviderFamily:       string(up.ProviderFamily),
		GroupAllowProtocolConversion: group.AllowProtocolConversion,
		Protocol:                     providerProtocol,
		BaseURL:                      up.BaseURL,
		Timeouts:                     routeTimeoutsFromUpstream(runtimecompat.CapabilityFromCore(req.Capability), up),
		ProviderCode:                 firstNonEmptyRuntime(up.Code, up.Name, up.ID),
		UpstreamConcurrencyLimit:     up.ConcurrencyLimit,
		ImageStreamMode:              imagePolicyString(modelBinding.Config, "stream_mode", domain.ImageStreamModeForceSync),
		ImageEditTransport:           imagePolicyString(modelBinding.Config, "edit_transport", domain.ImageEditTransportMultipart),
		ImageUpstreamResponseFormat:  imagePolicyString(modelBinding.Config, "upstream_response_format", ""),
		ImageMaxOutputCount:          imagePolicyInt(modelBinding.Config, "max_output_count", domain.DefaultImageOutputCount),
		ImageEditMaxOutputCount:      imagePolicyInt(modelBinding.Config, "edit_max_output_count", domain.DefaultImageOutputCount),
		AccountPriceBookID:           binding.CostPriceBookID,
		TenantMultiplier:             normalizedTenantMultiplier(binding.TenantMultiplier),
		CostPer1kTokens:              binding.CostPer1kTokens,
		GroupID:                      group.ID,
		GroupName:                    group.Name,
		RetailPriceBookID:            group.RetailPriceBookID,
		GroupDefaultUserMultiplier:   group.DefaultUserMultiplier,
		ConversionBucket:             binding.ConversionBucket,
	}
	if planned.MatchedRule != nil {
		cand.MatchedDispatchRuleID = planned.MatchedRule.ID
		cand.MatchedDispatchRuleSummary = fmt.Sprintf("%s:%s -> %s", planned.MatchedRule.MatchType, planned.MatchedRule.MatchValue, planned.MatchedRule.TargetModelID)
	}
	switch modelBinding.UpstreamKind {
	case coreupstream.AccessModeDirect:
		cand.EndpointID = up.ID
		cand.UpstreamModel = upstreamModel
		cand.APIKeyCiphertext = binding.APIKeyCiphertext
		cand.ExtraHeaders = mapStringCopy(binding.ExtraHeaders)
	case coreupstream.AccessModeOAuthPool:
		cand.PoolID = up.ID
		cand.PoolUpstreamModel = upstreamModel
		cand.UpstreamModel = upstreamModel
		cand.FixedProviderType = domain.FixedProviderType(binding.FixedProviderType)
		cand.OAuthStrategy = string(binding.SelectionStrategy)
	default:
		return nil, errors.New("unsupported upstream binding mode")
	}
	return cand, nil
}

// logRouteFailure records why a request could not be routed.
//
// The client only ever sees "no available upstream route" — a deliberately
// vague 503, since the reasons name internal groups, targets and credentials.
// That leaves the server as the only place the actual cause can live, and until
// this existed there was none: a mismatched provider key master silently
// dropped every candidate and produced no log line at all.
func (s *RuntimeRouteSelector) logRouteFailure(req *serving.Request, subject *coreidentity.Subject, err error) {
	fields := []zap.Field{
		zap.String("request_id", req.RequestID),
		zap.String("model", req.ModelCode),
		zap.String("tenant_id", subject.TenantID),
		zap.Error(err),
	}

	var noRoute *coreruntime.NoRouteError
	if !errors.As(err, &noRoute) {
		// Planning failed before any target was evaluated (no accessible group,
		// endpoint not enabled, ...). The error itself is the whole story.
		s.logger.Warn("route planning failed", fields...)
		return
	}
	for _, rejected := range noRoute.Rejections {
		fields = append(fields, zap.String("rejected."+rejected.Target.ID, string(rejected.Code)+": "+rejected.Detail))
	}
	s.logger.Warn("no executable route candidate", append(fields,
		zap.Int("rejected_targets", len(noRoute.Rejections)))...)
}

func routePlanningError(err error) error {
	switch {
	case errors.Is(err, commercial.ErrNoAccessibleGroup):
		return &serving.APIError{Status: http.StatusForbidden, Code: "no_accessible_group", Message: "no group is accessible to this caller"}
	case errors.Is(err, commercial.ErrClientSurfaceNotAllowed):
		return &serving.APIError{Status: http.StatusForbidden, Code: "endpoint_not_allowed", Message: "this API endpoint is not enabled for the accessible groups"}
	case errors.Is(err, coreruntime.ErrNoDispatchOption), errors.Is(err, coreruntime.ErrNoRouteCandidates):
		return &serving.APIError{Status: http.StatusServiceUnavailable, Code: "no_available_route", Message: "no available upstream route for this request"}
	default:
		return err
	}
}

func requestBody(req *serving.Request) []byte {
	if req == nil || req.Envelope == nil {
		return nil
	}
	return req.Envelope.ClientBody
}

func mapKeys(values map[string]float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
