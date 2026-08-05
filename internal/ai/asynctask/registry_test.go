package asynctask

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type stubHandler struct {
	prepare func(context.Context, Submission) (Prepared, error)
	execute func(context.Context, Task) (Result, error)
}

func (h stubHandler) Prepare(ctx context.Context, sub Submission) (Prepared, error) {
	if h.prepare != nil {
		return h.prepare(ctx, sub)
	}
	return Prepared{Input: []byte(`{}`), ModelCode: "m"}, nil
}

func (h stubHandler) Execute(ctx context.Context, task Task) (Result, error) {
	if h.execute != nil {
		return h.execute(ctx, task)
	}
	return Result{Status: domain.TaskCompleted, Output: []byte(`{}`)}, nil
}

func TestRegistryLookupAndDefaults(t *testing.T) {
	r := newRegistry()
	r.register("api.images.generation", stubHandler{}, Options{})

	reg, ok := r.lookup("api.images.generation")
	if !ok {
		t.Fatal("registered type should be found")
	}
	// Image work must not silently retry: the double-spend guard cannot see a
	// charge whose usage log has not committed yet.
	if reg.opts.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts default = %d, want 1", reg.opts.MaxAttempts)
	}
	if _, ok := r.lookup("nope"); ok {
		t.Fatal("unregistered type should not be found")
	}
}

func TestRegistryTypesAreSorted(t *testing.T) {
	r := newRegistry()
	r.register("console.images.edit", stubHandler{}, Options{})
	r.register("api.images.generation", stubHandler{}, Options{})
	r.register("app.images.edit", stubHandler{}, Options{})

	got := r.types()
	want := []string{"api.images.generation", "app.images.edit", "console.images.edit"}
	if len(got) != len(want) {
		t.Fatalf("types() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("types() = %v, want %v", got, want)
		}
	}
}

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected panic, got none", name)
		}
	}()
	fn()
}

func TestRegistryRejectsWiringMistakes(t *testing.T) {
	// Each of these is a composition-root bug that is strictly worse when found
	// late: a duplicate silently shadows a capability, and a late registration
	// produces rows no worker will ever claim.
	t.Run("duplicate type", func(t *testing.T) {
		r := newRegistry()
		r.register("api.images.generation", stubHandler{}, Options{})
		mustPanic(t, "duplicate", func() {
			r.register("api.images.generation", stubHandler{}, Options{})
		})
	})

	t.Run("register after freeze", func(t *testing.T) {
		r := newRegistry()
		r.freeze()
		mustPanic(t, "after freeze", func() {
			r.register("api.images.generation", stubHandler{}, Options{})
		})
	})

	t.Run("empty type", func(t *testing.T) {
		r := newRegistry()
		mustPanic(t, "empty type", func() { r.register("", stubHandler{}, Options{}) })
	})

	t.Run("nil handler", func(t *testing.T) {
		r := newRegistry()
		mustPanic(t, "nil handler", func() { r.register("api.images.generation", nil, Options{}) })
	})
}

func TestIdempotencyScopeIsolatesCredentials(t *testing.T) {
	// Two API keys in one tenant are two integrations; their independent
	// "retry-1" keys must not collide.
	a := idempotencyScope(SubjectRef{TenantID: "t1", APIKeyID: "key-a"})
	b := idempotencyScope(SubjectRef{TenantID: "t1", APIKeyID: "key-b"})
	if a == b {
		t.Fatalf("two API keys in one tenant share a scope: %q", a)
	}

	invoke := idempotencyScope(SubjectRef{TenantID: "t1", InvokeKeyID: "invk-1"})
	if invoke == a {
		t.Fatal("an app key and an API key share a scope")
	}

	user := idempotencyScope(SubjectRef{TenantID: "t1", UserID: "u1"})
	other := idempotencyScope(SubjectRef{TenantID: "t1", UserID: "u2"})
	if user == other {
		t.Fatal("two console users share a scope")
	}
}

func TestIdempotencyFingerprintDistinguishesRequests(t *testing.T) {
	base := idempotencyFingerprint("api.images.generation", []byte(`{"prompt":"a cat"}`))

	same := idempotencyFingerprint("api.images.generation", []byte(`{"prompt":"a cat"}`))
	if string(base) != string(same) {
		t.Fatal("fingerprint is not stable for an identical request")
	}

	// A reused key carrying different input must be rejected, not silently
	// answered with the unrelated original task.
	diffInput := idempotencyFingerprint("api.images.generation", []byte(`{"prompt":"a dog"}`))
	if string(base) == string(diffInput) {
		t.Fatal("different input produced the same fingerprint")
	}

	diffType := idempotencyFingerprint("api.images.edit", []byte(`{"prompt":"a cat"}`))
	if string(base) == string(diffType) {
		t.Fatal("different task type produced the same fingerprint")
	}
}

func TestRetryableMarking(t *testing.T) {
	if IsRetryable(context.Canceled) {
		t.Fatal("a plain error must not be retryable")
	}
	if !IsRetryable(Retryable(context.Canceled)) {
		t.Fatal("a marked error must be retryable")
	}
	if Retryable(nil) != nil {
		t.Fatal("Retryable(nil) must stay nil")
	}
}
