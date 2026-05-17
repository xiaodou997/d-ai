package serving

import (
	"context"
	"errors"
	"testing"
)

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
