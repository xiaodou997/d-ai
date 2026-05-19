package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"uni-ai-api/backend/internal/domain"
)

// fakeSelector returns canned candidates/error for testing RouteCandidatesStep.
type fakeSelector struct {
	candidates []*domain.RouteCandidate
	err        error
}

func (f *fakeSelector) SelectCandidates(_ context.Context, _ *Request) ([]*domain.RouteCandidate, error) {
	return f.candidates, f.err
}

// fakeStep is a configurable Step used to exercise Pipeline.Run.
type fakeStep struct {
	name        string
	execErr     error
	executed    bool
	rolledBack  bool
}

func (s *fakeStep) Name() string { return s.name }
func (s *fakeStep) Execute(_ context.Context, _ *Request) error {
	s.executed = true
	return s.execErr
}
func (s *fakeStep) Rollback(_ context.Context, _ *Request) { s.rolledBack = true }

func TestPipelineRunAllSucceed(t *testing.T) {
	a := &fakeStep{name: "a"}
	b := &fakeStep{name: "b"}
	p := NewPipeline(a, b)

	if err := p.Run(context.Background(), &Request{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !a.executed || !b.executed {
		t.Fatalf("both steps should have executed")
	}
	if a.rolledBack || b.rolledBack {
		t.Fatalf("no step should be rolled back on success")
	}
}

func TestPipelineRunRollsBackPriorStepsOnFailure(t *testing.T) {
	a := &fakeStep{name: "a"}
	b := &fakeStep{name: "b"}
	c := &fakeStep{name: "c", execErr: errors.New("boom")}
	d := &fakeStep{name: "d"}
	p := NewPipeline(a, b, c, d)

	err := p.Run(context.Background(), &Request{})
	if err == nil {
		t.Fatal("expected error from failing step")
	}

	var pe *PipelineError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PipelineError, got %T", err)
	}
	if pe.Step != "c" {
		t.Fatalf("PipelineError.Step = %q, want c", pe.Step)
	}

	if !a.rolledBack || !b.rolledBack {
		t.Fatalf("prior steps should have been rolled back (a=%v b=%v)", a.rolledBack, b.rolledBack)
	}
	if c.rolledBack {
		t.Fatalf("failing step itself must not be rolled back")
	}
	if d.executed {
		t.Fatalf("steps after the failing one must not execute")
	}
}

func TestPipelineErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	pe := &PipelineError{Step: "x", Cause: inner}
	if !errors.Is(pe, inner) {
		t.Fatalf("PipelineError should unwrap to its cause")
	}
}

// RouteCandidatesStep must preserve a structured *APIError (e.g. the 400
// no_matching_deployment returned by the postgres RouteSelector under strict
// protocol matching) instead of collapsing every selection failure into a 503.
func TestRouteCandidatesStepPreservesAPIError(t *testing.T) {
	want := &APIError{
		Status:  http.StatusBadRequest,
		Code:    "no_matching_deployment",
		Message: `no upstream deployment configured for model "gpt-x" with client protocol "anthropic_messages"`,
	}
	step := &RouteCandidatesStep{Selector: &fakeSelector{err: want}}
	err := step.Execute(context.Background(), &Request{})
	var got *APIError
	if !errors.As(err, &got) {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if got.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got.Status)
	}
	if got.Code != "no_matching_deployment" {
		t.Fatalf("code = %q, want no_matching_deployment", got.Code)
	}
}

// Generic selection errors (e.g. DB outage) should still surface as 503
// no_available_route — the new APIError pass-through must not break the
// fall-through path.
func TestRouteCandidatesStepWrapsGenericError(t *testing.T) {
	step := &RouteCandidatesStep{Selector: &fakeSelector{err: errors.New("db unreachable")}}
	err := step.Execute(context.Background(), &Request{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError wrapper, got %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", apiErr.Status)
	}
	if apiErr.Code != "no_available_route" {
		t.Fatalf("code = %q, want no_available_route", apiErr.Code)
	}
}
