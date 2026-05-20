package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"xiaodou/uni-ai-api/internal/domain"
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

// ============================================================================
// Finalizer tests
// ============================================================================

// fakeFinalizer records invocations and captures req state at call time.
type fakeFinalizer struct {
	name     string
	called   bool
	snapshot Request
}

func (f *fakeFinalizer) Name() string { return f.name }
func (f *fakeFinalizer) Finalize(_ context.Context, req *Request) {
	f.called = true
	f.snapshot = *req
}

// TestFinalizerRunsOnSuccess verifies the finalizer is called after a
// successful pipeline and req.RequestStatus is normalized to "success".
func TestFinalizerRunsOnSuccess(t *testing.T) {
	a := &fakeStep{name: "a"}
	fin := &fakeFinalizer{name: "fin"}
	p := NewPipeline(a).WithFinalizers(fin)

	if err := p.Run(context.Background(), &Request{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fin.called {
		t.Fatal("finalizer was not called on success")
	}
	if fin.snapshot.RequestStatus != domain.RequestSuccess {
		t.Fatalf("RequestStatus = %q, want %q", fin.snapshot.RequestStatus, domain.RequestSuccess)
	}
}

// TestFinalizerRunsOnFailure verifies the finalizer is called even when a step
// fails, and that req.RequestStatus is normalized to "failed".
func TestFinalizerRunsOnFailure(t *testing.T) {
	a := &fakeStep{name: "a"}
	b := &fakeStep{name: "b", execErr: errors.New("boom")}
	fin := &fakeFinalizer{name: "fin"}
	p := NewPipeline(a, b).WithFinalizers(fin)

	err := p.Run(context.Background(), &Request{})
	if err == nil {
		t.Fatal("expected error from failing step")
	}
	if !fin.called {
		t.Fatal("finalizer was not called on failure")
	}
	if fin.snapshot.RequestStatus != domain.RequestFailed {
		t.Fatalf("RequestStatus = %q, want %q", fin.snapshot.RequestStatus, domain.RequestFailed)
	}
}

// TestFinalizerRunsAfterRollback verifies that rollbacks complete before
// any finalizer runs.
func TestFinalizerRunsAfterRollback(t *testing.T) {
	var seq []string
	monA := &sequenceStep{name: "a", seq: &seq, tag: "rollback-a"}
	failB := &fakeStep{name: "b", execErr: errors.New("boom")}
	finFin := &sequenceFinalizer{name: "fin", seq: &seq, tag: "finalizer"}

	p := NewPipeline(monA, failB).WithFinalizers(finFin)
	if err := p.Run(context.Background(), &Request{}); err == nil {
		t.Fatal("expected error")
	}

	if len(seq) < 2 {
		t.Fatalf("expected at least 2 sequence entries, got %v", seq)
	}
	if seq[0] != "rollback-a" {
		t.Fatalf("expected rollback first, got %q", seq[0])
	}
	if seq[1] != "finalizer" {
		t.Fatalf("expected finalizer second, got %q", seq[1])
	}
}

// sequenceStep records its rollback tag into a shared slice.
type sequenceStep struct {
	name string
	seq  *[]string
	tag  string
}

func (s *sequenceStep) Name() string { return s.name }
func (s *sequenceStep) Execute(_ context.Context, _ *Request) error { return nil }
func (s *sequenceStep) Rollback(_ context.Context, _ *Request) {
	*s.seq = append(*s.seq, s.tag)
}

// sequenceFinalizer records its tag into a shared slice.
type sequenceFinalizer struct {
	name string
	seq  *[]string
	tag  string
}

func (f *sequenceFinalizer) Name() string { return f.name }
func (f *sequenceFinalizer) Finalize(_ context.Context, _ *Request) {
	*f.seq = append(*f.seq, f.tag)
}

// TestFinalizerNormalizesAPIError verifies that an APIError from a failed step
// is reflected in HTTPStatus and ErrorCode before the finalizer runs.
func TestFinalizerNormalizesAPIError(t *testing.T) {
	failStep := &apiErrorStep{
		name: "auth",
		err:  &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "bad key"},
	}
	fin := &fakeFinalizer{name: "fin"}
	p := NewPipeline(failStep).WithFinalizers(fin)

	_ = p.Run(context.Background(), &Request{})
	if !fin.called {
		t.Fatal("finalizer not called")
	}
	if fin.snapshot.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("HTTPStatus = %d, want 401", fin.snapshot.HTTPStatus)
	}
	if fin.snapshot.ErrorCode != "unauthorized" {
		t.Fatalf("ErrorCode = %q, want unauthorized", fin.snapshot.ErrorCode)
	}
}

// TestFinalizerPreservesExecuteSetFields verifies that fields already written
// by Execute are not overwritten by normalizePipelineError.
func TestFinalizerPreservesExecuteSetFields(t *testing.T) {
	// Simulate Execute having written its own status before a late step fails.
	setStatusStep := &setReqFieldsStep{
		name:          "execute",
		requestStatus: domain.RequestSuccess,
		httpStatus:    http.StatusOK,
	}
	lateFailStep := &fakeStep{name: "usage_log", execErr: errors.New("db down")}
	fin := &fakeFinalizer{name: "fin"}
	p := NewPipeline(setStatusStep, lateFailStep).WithFinalizers(fin)

	_ = p.Run(context.Background(), &Request{})
	if fin.snapshot.RequestStatus != domain.RequestSuccess {
		t.Fatalf("RequestStatus overwritten: got %q, want %q", fin.snapshot.RequestStatus, domain.RequestSuccess)
	}
	if fin.snapshot.HTTPStatus != http.StatusOK {
		t.Fatalf("HTTPStatus overwritten: got %d, want 200", fin.snapshot.HTTPStatus)
	}
}

// TestMultipleFinalizersAllRun verifies all attached finalizers are called.
func TestMultipleFinalizersAllRun(t *testing.T) {
	fin1 := &fakeFinalizer{name: "fin1"}
	fin2 := &fakeFinalizer{name: "fin2"}
	p := NewPipeline(&fakeStep{name: "a"}).WithFinalizers(fin1, fin2)

	_ = p.Run(context.Background(), &Request{})
	if !fin1.called || !fin2.called {
		t.Fatalf("fin1.called=%v fin2.called=%v, both should be true", fin1.called, fin2.called)
	}
}

// apiErrorStep is a step that always fails with a given *APIError.
type apiErrorStep struct {
	name string
	err  *APIError
}

func (s *apiErrorStep) Name() string                               { return s.name }
func (s *apiErrorStep) Execute(_ context.Context, _ *Request) error { return s.err }
func (s *apiErrorStep) Rollback(_ context.Context, _ *Request)      {}

// setReqFieldsStep sets specific fields on req and then succeeds.
type setReqFieldsStep struct {
	name          string
	requestStatus domain.RequestStatus
	httpStatus    int
}

func (s *setReqFieldsStep) Name() string { return s.name }
func (s *setReqFieldsStep) Execute(_ context.Context, req *Request) error {
	if s.requestStatus != "" {
		req.RequestStatus = s.requestStatus
	}
	if s.httpStatus != 0 {
		req.HTTPStatus = s.httpStatus
	}
	return nil
}
func (s *setReqFieldsStep) Rollback(_ context.Context, _ *Request) {}
