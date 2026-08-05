// Package serving implements the AI request execution pipeline.
// Each request flows through a series of ordered steps. If any step fails,
// previously-executed steps with rollback functions are unwound in reverse.
//
// Default pipeline order:
//
//	AuthN → AuthZ → QuotaCheck → RouteSelect → RateLimit → Execute
//
// 请求结束时 finalizer 将使用记录、配额和 V3 请求入账原子完成，再由异步
// settlement worker 聚合到 URM，不做每请求同步扣款。
package serving

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"xiaodou/dai/internal/ai/core/catalog"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

// ============================================================================
// Pipeline request context
// ============================================================================

// Request carries all state through the pipeline. Steps read from and write to
// this struct; they must not share mutable state via closures.
//
// Wire-level concerns (ResponseWriter / *http.Request) live in Envelope rather
// than directly on Request, so that the pipeline can be exercised in tests and
// the Execute step can retry across multiple upstream attempts without
// re-allocating the request object.
type Request struct {
	// Envelope owns the HTTP transport for this request. Steps that need to
	// read the inbound request (AuthN) or write a response (Execute via Relay)
	// reach into the envelope. It is populated by the HTTP handler before the
	// pipeline runs.
	Envelope *RequestEnvelope

	// Parsed from HTTP body before pipeline starts. The full client body lives
	// on Envelope.ClientBody and is the source of truth for the upstream call.
	// Image edits are normalized through imageedit before the provider adapter.
	RequestedModel string
	ModelCode      string
	// RuntimeCapability preserves the precise planning capability (for example
	// image_generation versus image_edit) that the legacy CapabilityType cannot
	// represent.
	RuntimeCapability catalog.Capability
	CapabilityType    domain.CapabilityType
	ClientProtocol    domain.UpstreamProtocol
	ExecutionMode     coreruntime.ExecutionMode
	IsStream          bool
	// ImageClientResponseFormat is a platform response preference. It is never
	// forwarded as a GPT Image upstream request parameter.
	ImageClientResponseFormat   string
	MatchedDispatchRuleID       string
	MatchedDispatchRuleSummary  string
	ResolvedLogicalModel        string
	ResolvedProviderFamily      string
	ProtocolConversionEnabled   bool
	SelectedUpstreamProtocol    string
	SelectedUpstreamTargetType  string
	SelectedUpstreamModel       string
	UpstreamModelMappingApplied bool
	PublicResponseModel         string
	ServiceTier                 domain.ServiceTier

	// Subject is the canonical runtime caller context for every runtime path.
	Subject *coreidentity.Subject

	// Extracted from request body; used by RouteCandidates for sticky routing.
	ConversationID string

	// Extracted from request body at ingress (client-side, pre-conversion),
	// normalized to low/medium/high/xhigh/max; empty when not declared.
	// Recorded on the usage log only.
	ReasoningEffort string

	// Populated by RouteCandidatesStep: the ordered list of routes the Execute
	// step may try. Sticky-pinned candidates are placed first.
	Candidates     []*domain.RouteCandidate
	UsedCandidates map[string]bool // route_id → already attempted

	// The currently-selected (or last-attempted) candidate. ExecuteStep sets
	// this on every attempt and downstream steps (URMConfirm, UsageLog,
	// observability) read it as "the route that actually served this request".
	Candidate *domain.RouteCandidate

	// SelectedCredential is the OAuth credential chosen for the current
	// attempt when Candidate is a pool route. Reset between retries that swap
	// credentials (401) or routes.
	SelectedCredential *domain.OAuthCredential

	// Attempts records every upstream call made during Execute. Used by
	// X-Route-Trace observability and to drive 429 backoff decisions.
	Attempts []AttemptRecord

	// StickyHit is set to true when the first candidate was loaded from the
	// sticky Redis binding rather than freshly scored.
	StickyHit bool

	// StickyBinding is the binding StickyHit was resolved from. Execute reads
	// it to pin the pool credential as well, so a conversation stays on one
	// physical upstream account and not merely on one route.
	StickyBinding *routing.StickyBinding

	// Set by Execute step: rebuilt assistant message for the audit log.
	// Sync requests: populated immediately after body read.
	// Stream requests: populated after the stream drains (finishStream).
	AuditResponseMessage []byte

	// Filled by Execute step
	TokenUsage           domain.TokenUsage
	TokenCountSource     string // domain.TokenUsageSource* — set by Execute, read by UsageLogger
	ClientPath           string
	UpstreamBodySize     int    // byte length of the serialised upstream request body; used for token estimation
	UpstreamResponseBody []byte // raw sync response body (nil for streams; used for replay)
	BillingSnapshots     map[string]domain.BillingSnapshot
	BillingResult        domain.BillingResult

	// Billing source decided by SubscriptionGateStep (post QuotaCheck, before
	// routing). "payg" (按量) or "subscription" (订阅覆盖). Empty ⇒ never gated;
	// downstream (usage log / ledger) coalesces empty to "payg".
	// SubscriptionID is set only when BillingSource == "subscription".
	BillingSource  string
	SubscriptionID string
	// SubscriptionGroupQuotaDebitMultipliers is the covering subscription's {group_id: quota_debit_multiplier}
	// snapshot, written by SubscriptionGateStep when the request stays subscription-
	// covered. RuntimeRouteSelector intersects candidate groups with its key set (empty
	// intersection ⇒ falls back to payg); financial completion debits
	// RetailBaseMicro × the immutable plan weight.
	SubscriptionGroupQuotaDebitMultipliers map[string]float64
	BillingWindowID                        string
	BillingLeaseID                         string
	BillingAdmissionActive                 bool
	FinancialCompletionFailed              bool

	// RateLimitLease holds configured concurrency slots acquired before Execute.
	// The pipeline releases it on every rollback and finalizer path.
	RateLimitLease RateLimitLease

	RequestStatus domain.RequestStatus
	HTTPStatus    int
	// ResponseCommitted is true once the Execute step has written response
	// headers to the client. Past this point the pipeline can no longer emit
	// a fresh HTTP error status — only an in-band (SSE) error frame. It must
	// not be conflated with HTTPStatus, which normalizePipelineError also sets
	// for audit logging on errors that never reached the wire.
	ResponseCommitted bool
	UpstreamStatus    int
	ErrorCode         string
	ErrorMessage      string
	LatencyMs         int
	FirstTokenMs      int
	Timing            RequestTiming

	// InternalErrorDetail / FailedStep are admin-only diagnostics: the raw,
	// unsanitized, untruncated error text (Go error chain and/or full upstream
	// response body) and the pipeline step that produced it. Unlike ErrorCode /
	// ErrorMessage, these never reach the client and never reach tenant/user
	// self-service usage endpoints — only the platform admin detail endpoint
	// selects them (they live in ai_request_payloads, not ai_usage_logs).
	InternalErrorDetail string
	FailedStep          string

	// Internal timestamps
	StartedAt time.Time
	RequestID string
	TraceID   string
}

func (r *Request) RuntimeSubject() *coreidentity.Subject {
	if r == nil {
		return nil
	}
	return r.Subject
}

func runtimeSubjectUsesAPIKeyQuota(subject *coreidentity.Subject) bool {
	return subject != nil && subject.AuthMethod == coreidentity.AuthMethodAPIKey && subject.APIKeyID != ""
}

func runtimeSubjectLegacyOwnerType(subject *coreidentity.Subject) domain.OwnerType {
	if subject == nil {
		return domain.OwnerTenant
	}
	switch subject.Scope {
	case coreidentity.ScopeUser:
		return domain.OwnerUser
	default:
		return domain.OwnerTenant
	}
}

func runtimeSubjectStickyKey(subject *coreidentity.Subject) string {
	if subject == nil {
		return ""
	}
	if subject.APIKeyID != "" {
		return subject.APIKeyID
	}
	return string(subject.RequestSource) + ":" + string(subject.Scope) + ":" + subject.TenantID + ":" + subject.UserID
}

func (r *Request) PublicModel() string {
	if r == nil {
		return ""
	}
	if r.RequestedModel != "" {
		return r.RequestedModel
	}
	return r.ModelCode
}

func (r *Request) StickyModelKey() string {
	if r == nil {
		return ""
	}
	if r.RequestedModel != "" {
		return r.RequestedModel
	}
	return r.ModelCode
}

// UpstreamStream reports whether THIS upstream call should be made in streaming
// mode. It is decoupled from the client-facing IsStream: for image generation
// the binding policy (ImageStreamMode) governs how we call the upstream —
// force_stream/force_sync override the client's preference — because some image
// upstreams only support one transport. Non-image traffic (and image "auto")
// falls back to the client's IsStream. The relay then adapts the upstream
// response shape back to whatever the client actually asked for.
func (r *Request) UpstreamStream() bool {
	if r == nil {
		return false
	}
	if r.CapabilityType == domain.CapabilityImage && r.Candidate != nil {
		switch r.Candidate.ImageStreamMode {
		case domain.ImageStreamModeForceStream:
			return true
		case domain.ImageStreamModeForceSync:
			return false
		}
	}
	return r.IsStream
}

func (r *Request) SetCandidate(c *domain.RouteCandidate) {
	if r == nil {
		return
	}
	r.Candidate = c
	if c == nil {
		return
	}
	r.ModelCode = c.ModelCode
	r.MatchedDispatchRuleID = c.MatchedDispatchRuleID
	r.MatchedDispatchRuleSummary = c.MatchedDispatchRuleSummary
	r.ResolvedLogicalModel = c.ModelCode
	r.ResolvedProviderFamily = c.ResolvedProviderFamily
	r.ProtocolConversionEnabled = c.GroupAllowProtocolConversion
	r.SelectedUpstreamProtocol = string(c.Protocol)
	r.SelectedUpstreamTargetType = c.UpstreamTargetType()
	r.SelectedUpstreamModel = c.EffectiveUpstreamModel()
	r.UpstreamModelMappingApplied = r.SelectedUpstreamModel != "" && r.SelectedUpstreamModel != c.ModelCode
	r.PublicResponseModel = r.PublicModel()
}

// ============================================================================
// Step interface
// ============================================================================

// Step is one stage in the serving pipeline.
type Step interface {
	// Name returns a human-readable label used in logs and traces.
	Name() string
	// Execute runs the step. Returns an error to abort the pipeline.
	// If Execute succeeds and Rollback is non-nil, Rollback will be called
	// on pipeline failure for all subsequent steps.
	Execute(ctx context.Context, req *Request) error
	// Rollback undoes side effects if a later step fails. May be nil.
	Rollback(ctx context.Context, req *Request)
}

// Finalizer runs unconditionally after the pipeline completes (whether success
// or failure, after all rollbacks). Finalizers must not return errors — any
// internal failures should be logged and swallowed.
type Finalizer interface {
	Name() string
	Finalize(ctx context.Context, req *Request)
}

// ============================================================================
// Pipeline
// ============================================================================

// Pipeline chains Steps and manages rollback on failure.
type Pipeline struct {
	steps      []Step
	finalizers []Finalizer
}

// NewPipeline constructs a pipeline from an ordered list of steps.
func NewPipeline(steps ...Step) *Pipeline {
	return &Pipeline{steps: steps}
}

// WithFinalizers attaches finalizers that run unconditionally after Run
// completes (success or failure, after all rollbacks). Returns p for chaining.
func (p *Pipeline) WithFinalizers(fns ...Finalizer) *Pipeline {
	p.finalizers = append(p.finalizers, fns...)
	return p
}

// Run executes all steps in order. On the first error, it rolls back
// all previously-completed steps in reverse order, then returns the error.
// Finalizers (if any) always run last, after rollback, with normalized req state.
// The ResponseWriter should already have been written by the Execute step
// before pipeline failure occurs (errors after execution are best-effort).
func (p *Pipeline) Run(ctx context.Context, req *Request) error {
	tracer := otel.Tracer("uni-ai-api/serving")
	ctx, span := tracer.Start(ctx, "pipeline.run")
	if req.ModelCode != "" {
		span.SetAttributes(attribute.String("ai.model", req.ModelCode))
	}
	if req.RequestID != "" {
		span.SetAttributes(attribute.String("request.id", req.RequestID))
	}
	defer span.End()

	var pipeErr error
	if len(p.finalizers) > 0 {
		defer func() {
			req.MarkCompleted(time.Now())
			normalizePipelineError(req, pipeErr)
			for _, f := range p.finalizers {
				f.Finalize(ctx, req)
			}
		}()
	}

	completed := make([]Step, 0, len(p.steps))

	for _, step := range p.steps {
		stepCtx, stepSpan := tracer.Start(ctx, "step."+step.Name())
		err := step.Execute(stepCtx, req)
		if err != nil {
			stepSpan.RecordError(err)
			stepSpan.SetStatus(codes.Error, err.Error())
			stepSpan.End()
			// Rollback completed steps in reverse order
			for i := len(completed) - 1; i >= 0; i-- {
				completed[i].Rollback(stepCtx, req)
			}
			span.SetStatus(codes.Error, err.Error())
			pipeErr = &PipelineError{Step: step.Name(), Cause: err}
			return pipeErr
		}
		stepSpan.End()
		completed = append(completed, step)
	}
	return nil
}

// normalizePipelineError fills any unset result fields on req from the pipeline
// error. Fields already written by the Execute step are preserved.
func normalizePipelineError(req *Request, err error) {
	if err == nil {
		if req.RequestStatus == "" {
			req.RequestStatus = domain.RequestSuccess
		}
		return
	}
	if req.RequestStatus == "" {
		req.RequestStatus = domain.RequestFailed
	}

	// cause is the error with the failing step name peeled off; it is what we
	// want to describe in InternalErrorDetail (the step name goes to
	// FailedStep instead so it isn't duplicated in the free-text detail).
	cause := err
	if pipeErr, ok := err.(*PipelineError); ok {
		if req.FailedStep == "" {
			req.FailedStep = pipeErr.Step
		}
		cause = pipeErr.Cause
	}

	var apiErr *APIError
	if errors.As(cause, &apiErr) {
		if req.HTTPStatus == 0 {
			req.HTTPStatus = apiErr.Status
		}
		if req.ErrorCode == "" {
			req.ErrorCode = apiErr.Code
		}
		if apiErr.cause != nil {
			cause = apiErr.cause
		}
	} else {
		if req.HTTPStatus == 0 {
			req.HTTPStatus = http.StatusInternalServerError
		}
		if req.ErrorCode == "" {
			req.ErrorCode = "internal_error"
		}
	}

	// Admin-only diagnostics: capture the real, unwrapped error text even
	// when it was already condensed into a client-safe ErrorMessage/ErrorCode
	// above (or left blank, as pre-Execute steps never populate ErrorMessage
	// today). Never overwrites a detail an earlier step already set.
	if req.InternalErrorDetail == "" && cause != nil {
		req.InternalErrorDetail = RedactInternalErrorDetail(cause.Error())
	}
}

// PipelineError wraps an error with the name of the step that failed.
type PipelineError struct {
	Step  string
	Cause error
}

func (e *PipelineError) Error() string {
	return "pipeline[" + e.Step + "]: " + e.Cause.Error()
}

func (e *PipelineError) Unwrap() error { return e.Cause }
