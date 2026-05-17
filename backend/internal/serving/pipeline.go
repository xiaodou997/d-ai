// Package serving implements the AI request execution pipeline.
// Each request flows through a series of ordered steps. If any step fails,
// previously-executed steps with rollback functions are unwound in reverse.
//
// Default pipeline order:
//
//	AuthN → AuthZ → QuotaCheck → RouteSelect → RateLimit →
//	QuotaReserve → URMFreeze → Execute → URMConfirm → UsageLog
package serving

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats/canonical"
)

// ============================================================================
// Pipeline request context
// ============================================================================

// Request carries all state through the pipeline. Steps read from and write to
// this struct; they must not share mutable state via closures.
type Request struct {
	// W is the client-facing response writer. ExecuteStep writes to it directly.
	W http.ResponseWriter
	// R is the inbound HTTP request (read-only after parsing).
	R *http.Request

	// Parsed from HTTP body before pipeline starts
	ModelCode      string
	CapabilityType domain.CapabilityType
	ClientProtocol domain.UpstreamProtocol
	IsStream       bool

	// Canonical form of the decoded AI request
	ChatReq      *canonical.ChatRequest
	EmbedReq     *canonical.EmbeddingRequest
	ImageReq     *canonical.ImageRequest

	// Resolved by AuthN step
	APIKey *domain.APIKeyAuth

	// Extracted from request body; used by RouteSelect for sticky routing.
	ConversationID string

	// Resolved by RouteSelect step
	Candidate          *domain.RouteCandidate
	// Resolved by RouteSelect step when endpoint.auth_type = 'oauth'
	SelectedCredential *domain.OAuthCredential

	// Resolved by URMFreeze step
	URMTransactionID string

	// Set by QuotaReserveStep — the exact amount reserved against the API key's
	// quota_reserved column. UsageLogger must release this same amount during
	// confirmation; releasing the post-hoc billing cost instead would let the
	// reserved balance drift (estimate ≠ actual cost).
	QuotaReservedAmount int64

	// Set to true by URMBiller.Confirm after it has computed and written BillingResult.
	// UsageLogStep skips billing calculation when this is true.
	BillingResolved bool

	// Filled by Execute step
	TokenUsage       domain.TokenUsage
	TokenCountSource string // "upstream" | "estimated" — set by Execute, read by UsageLogger
	UpstreamBodySize int    // byte length of the serialised upstream request body; used for token estimation
	BillingResult    domain.BillingResult
	RequestStatus    domain.RequestStatus
	HTTPStatus       int
	UpstreamStatus   int
	ErrorCode        string
	ErrorMessage     string
	LatencyMs        int
	FirstTokenMs     int

	// Internal timestamps
	StartedAt time.Time
	RequestID string
	TraceID   string
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

// ============================================================================
// Pipeline
// ============================================================================

// Pipeline chains Steps and manages rollback on failure.
type Pipeline struct {
	steps []Step
}

// NewPipeline constructs a pipeline from an ordered list of steps.
func NewPipeline(steps ...Step) *Pipeline {
	return &Pipeline{steps: steps}
}

// Run executes all steps in order. On the first error, it rolls back
// all previously-completed steps in reverse order, then returns the error.
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
			return &PipelineError{Step: step.Name(), Cause: err}
		}
		stepSpan.End()
		completed = append(completed, step)
	}
	return nil
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
