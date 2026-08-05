package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/runtimecompat"
	"xiaodou/dai/internal/ai/serving"
)

// RuntimeEngine owns the complete post-auth runtime lifecycle. Route planning
// is a pipeline step, so callers cannot resolve routes before subscription and
// billing constraints have been established.
type RuntimeEngine struct {
	pipeline *serving.Pipeline
}

func NewRuntimeEngine(pipeline *serving.Pipeline) *RuntimeEngine {
	return &RuntimeEngine{pipeline: pipeline}
}

var _ coreruntime.Engine = (*RuntimeEngine)(nil)

func (e *RuntimeEngine) Execute(ctx context.Context, in coreruntime.ExecutionInput) (coreruntime.Result, error) {
	if e == nil || e.pipeline == nil {
		return coreruntime.Result{}, errors.New("runtime execution pipeline is not configured")
	}
	req, err := buildRuntimeServingRequest(in)
	if err != nil {
		return coreruntime.Result{}, err
	}
	err = e.pipeline.Run(ctx, req)
	if err == nil && in.Envelope.ResponseWriter != nil {
		writeRouteHeaders(in.Envelope.ResponseWriter, req)
	}
	return runtimeResultFromServing(req), err
}

func buildRuntimeServingRequest(in coreruntime.ExecutionInput) (*serving.Request, error) {
	clientProtocol, err := runtimecompat.SurfaceToProtocolForCapability(in.Request.ClientSurface, in.Request.Capability)
	if err != nil {
		return nil, err
	}
	body := in.Envelope.ClientBody
	envelope := &serving.RequestEnvelope{
		W:              in.Envelope.ResponseWriter,
		R:              in.Envelope.HTTPRequest,
		ClientProtocol: clientProtocol,
		IsStream:       in.Request.Stream,
		ClientBody:     body,
	}
	serviceTier, err := resolvedServiceTierFromInput(in)
	if err != nil {
		return nil, err
	}
	req := &serving.Request{
		Envelope:          envelope,
		RequestedModel:    firstNonEmptyRuntime(in.Request.RequestedModel, in.Request.ResolvedModelID),
		ModelCode:         firstNonEmptyRuntime(in.Request.ResolvedModelID, in.Request.RequestedModel),
		RuntimeCapability: in.Request.Capability,
		CapabilityType:    runtimecompat.CapabilityFromCore(in.Request.Capability),
		ClientProtocol:    clientProtocol,
		ExecutionMode:     normalizedExecutionMode(in.Request.ExecutionMode),
		IsStream:          in.Request.Stream,
		ServiceTier:       serviceTier,
		Subject:           runtimeSubjectPtr(in.Subject),
		StartedAt:         firstNonZeroTime(in.Request.ReceivedAt, time.Now()),
		RequestID:         in.Request.RequestID,
		TraceID:           in.Request.TraceID,
		ClientPath:        runtimeClientPath(in.Envelope.HTTPRequest),
	}
	applyResolvedRequestMeta(req, body, in.Envelope.HTTPRequest)
	return req, nil
}

func normalizedExecutionMode(mode coreruntime.ExecutionMode) coreruntime.ExecutionMode {
	if mode == coreruntime.ExecutionModeAsync {
		return mode
	}
	return coreruntime.ExecutionModeSync
}

func resolvedServiceTierFromInput(in coreruntime.ExecutionInput) (domain.ServiceTier, error) {
	if in.Request.ServiceTier != "" {
		tier, err := normalizeServiceTier(in.Request.ServiceTier)
		if err != nil {
			return "", &serving.APIError{Status: http.StatusBadRequest, Message: err.Error(), Code: "invalid_service_tier"}
		}
		return tier, nil
	}
	contentType := ""
	if in.Envelope.HTTPRequest != nil {
		contentType = in.Envelope.HTTPRequest.Header.Get("Content-Type")
	}
	tier, err := parseServiceTier(in.Envelope.ClientBody, contentType)
	if err != nil {
		return "", &serving.APIError{Status: http.StatusBadRequest, Message: err.Error(), Code: "invalid_service_tier"}
	}
	return tier, nil
}

func imagePolicyString(config map[string]any, key, fallback string) string {
	if len(config) == 0 {
		return fallback
	}
	raw, ok := config["image_generation"].(map[string]any)
	if !ok {
		return fallback
	}
	value, ok := raw[key].(string)
	if !ok {
		return fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func imagePolicyInt(config map[string]any, key string, fallback int) int {
	if len(config) == 0 {
		return fallback
	}
	raw, ok := config["image_generation"].(map[string]any)
	if !ok {
		return fallback
	}
	switch value := raw[key].(type) {
	case int:
		if value >= 1 && value <= domain.MaxImageOutputCount {
			return value
		}
	case float64:
		if value >= 1 && value <= domain.MaxImageOutputCount && value == float64(int(value)) {
			return int(value)
		}
	}
	return fallback
}

func runtimeClientPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}

func runtimeSubjectPtr(subject coreidentity.Subject) *coreidentity.Subject {
	copy := subject
	return &copy
}

func applyResolvedRequestMeta(req *serving.Request, body []byte, httpReq *http.Request) {
	if req == nil {
		return
	}
	contentType := ""
	if httpReq != nil {
		contentType = httpReq.Header.Get("Content-Type")
	}
	switch req.CapabilityType {
	case domain.CapabilityChat:
		req.ConversationID = extractConversationID(body, httpReq)
		if f, ok := formats.FormatIDForProtocol(req.ClientProtocol); ok {
			req.ReasoningEffort = formats.ExtractReasoningEffort(f, body)
		}
	case domain.CapabilityImage:
		if count, size := parseImageBillingMeta(body, contentType); count > 0 || size != "" {
			req.TokenUsage.ImageCount = count
			req.TokenUsage.ImageResolution = size
		}
	case domain.CapabilityVideo:
		var meta struct {
			Resolution string  `json:"resolution"`
			Duration   float64 `json:"duration"`
		}
		_ = json.Unmarshal(body, &meta)
		req.TokenUsage.VideoSeconds = meta.Duration
		req.TokenUsage.VideoResolution = meta.Resolution
	}
}

func routeTimeoutsFromUpstream(capType domain.CapabilityType, _ coreupstream.Upstream) domain.RouteTimeouts {
	return domain.DefaultRouteTimeouts(capType)
}

func normalizedTenantMultiplier(v float64) float64 {
	if v < 0 {
		return 1
	}
	return v
}

func firstNonEmptyRuntime(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func mapStringCopy(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func runtimeResultFromServing(req *serving.Request) coreruntime.Result {
	if req == nil {
		return coreruntime.Result{}
	}
	usage := map[string]any{
		"prompt_tokens":      req.TokenUsage.PromptTokens,
		"completion_tokens":  req.TokenUsage.CompletionTokens,
		"total_tokens":       req.TokenUsage.TotalTokens(),
		"cache_write_tokens": req.TokenUsage.CacheWriteTokens,
		"cache_read_tokens":  req.TokenUsage.CacheReadTokens,
		"reasoning_tokens":   req.TokenUsage.ReasoningTokens,
		"image_count":        req.TokenUsage.ImageCount,
		"image_resolution":   req.TokenUsage.ImageResolution,
		"video_seconds":      req.TokenUsage.VideoSeconds,
		"video_resolution":   req.TokenUsage.VideoResolution,
	}
	capability := req.RuntimeCapability
	if capability == "" {
		capability = runtimecompat.CapabilityToCore(req.CapabilityType)
	}
	upstreamSurface := surface.OpenAIChat
	routeID := ""
	if req.Candidate != nil {
		routeID = req.Candidate.RouteID
		if mapped, err := runtimecompat.ProtocolToSurfaceForCapability(req.Candidate.Protocol, capability); err == nil {
			upstreamSurface = mapped
		}
	}
	callerCharge := req.BillingResult.TenantPayableMicro
	if subject := req.RuntimeSubject(); subject != nil && subject.Scope == coreidentity.ScopeUser {
		callerCharge = req.BillingResult.UserChargedMicro
	}
	return coreruntime.Result{
		RequestID:            req.RequestID,
		Capability:           capability,
		ClientSurface:        runtimecompat.MustProtocolToSurfaceForCapability(req.ClientProtocol, capability),
		UpstreamSurface:      upstreamSurface,
		StatusCode:           req.HTTPStatus,
		RequestStatus:        string(req.RequestStatus),
		ResponseCommitted:    req.ResponseCommitted,
		RouteID:              routeID,
		Body:                 req.UpstreamResponseBody,
		Usage:                usage,
		CatalogBaseMicro:     req.BillingResult.CatalogBaseMicro,
		TenantPayableMicro:   req.BillingResult.TenantPayableMicro,
		UserPayableMicro:     req.BillingResult.UserPayableMicro,
		UserChargedMicro:     req.BillingResult.UserChargedMicro,
		APIKeyQuotaCostMicro: req.BillingResult.APIKeyQuotaCostMicro,
		CallerChargeMicro:    callerCharge,
		ErrorCode:            req.ErrorCode,
		ErrorMessage:         req.ErrorMessage,
		InternalErrorDetail:  req.InternalErrorDetail,
		FailedStep:           req.FailedStep,
		CreatedAt:            req.StartedAt,
	}
}
