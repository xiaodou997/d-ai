package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

// recordingStep captures what the gate handed it and optionally fails.
type recordingStep struct {
	name string
	err  error
	seen *[]string
	// capture lets a test inspect the synthesized request.
	capture func(*Request)
}

func (s *recordingStep) Name() string { return s.name }

// Rollback is absent on purpose: GateStep does not have one, which is the
// gate's guarantee that it only runs steps with nothing to undo.
func (s *recordingStep) Execute(_ context.Context, req *Request) error {
	if s.seen != nil {
		*s.seen = append(*s.seen, s.name)
	}
	if s.capture != nil {
		s.capture(req)
	}
	return s.err
}

func admissionInput() AdmissionInput {
	return AdmissionInput{
		Subject: coreidentity.Subject{
			AuthMethod: coreidentity.AuthMethodAPIKey,
			Scope:      coreidentity.ScopeTenant,
			TenantID:   "tenant-a",
			APIKeyID:   "key-1",
		},
		ModelCode:      "gpt-image-1",
		RequestedModel: "gpt-image-1",
		CapabilityType: domain.CapabilityImage,
		ClientProtocol: domain.ProtocolOpenAIImages,
	}
}

func TestAdmissionGateRunsStepsInOrder(t *testing.T) {
	var seen []string
	gate := NewAdmissionGate(
		&recordingStep{name: "authz", seen: &seen},
		&recordingStep{name: "quota", seen: &seen},
		&recordingStep{name: "subscription", seen: &seen},
		&recordingStep{name: "routes", seen: &seen},
		&recordingStep{name: "billing", seen: &seen},
	)
	if err := gate.Admit(context.Background(), admissionInput()); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	want := []string{"authz", "quota", "subscription", "routes", "billing"}
	if len(seen) != len(want) {
		t.Fatalf("ran %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("ran %v, want %v", seen, want)
		}
	}
}

// TestAdmissionGateStopsAtFirstRejection: the later steps assume the earlier
// ones passed, and running them anyway would waste database work on a request
// that is already refused.
func TestAdmissionGateStopsAtFirstRejection(t *testing.T) {
	var seen []string
	gate := NewAdmissionGate(
		&recordingStep{name: "authz", seen: &seen},
		&recordingStep{name: "quota", seen: &seen, err: apiError(http.StatusPaymentRequired, "quota_exhausted", "no quota left")},
		&recordingStep{name: "routes", seen: &seen},
	)
	err := gate.Admit(context.Background(), admissionInput())
	if err == nil {
		t.Fatal("Admit accepted a request the quota step rejected")
	}
	if len(seen) != 2 || seen[1] != "quota" {
		t.Fatalf("ran %v, want to stop after quota", seen)
	}

	// The caller needs the synchronous endpoint's own status code to pass back.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError; the transport could not choose a status", err)
	}
	if apiErr.Status != http.StatusPaymentRequired || apiErr.Code != "quota_exhausted" {
		t.Fatalf("error = %+v, want the step's own 402 quota_exhausted", apiErr)
	}
}

// TestAdmissionGateSynthesizesRequestWithoutEnvelope pins the contract that lets
// this reuse the pipeline's steps at all: no envelope is present, so any step
// added here must never read the inbound HTTP request.
func TestAdmissionGateSynthesizesRequestWithoutEnvelope(t *testing.T) {
	var got *Request
	gate := NewAdmissionGate(&recordingStep{
		name:    "capture",
		capture: func(req *Request) { got = req },
	})
	if err := gate.Admit(context.Background(), admissionInput()); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if got == nil {
		t.Fatal("step never ran")
	}
	if got.Envelope != nil {
		t.Fatal("the gate synthesized an envelope; steps must not be able to read a request that does not exist")
	}
	if got.RuntimeSubject() == nil || got.RuntimeSubject().TenantID != "tenant-a" {
		t.Fatalf("subject = %+v, want the caller's", got.RuntimeSubject())
	}
	if got.ModelCode != "gpt-image-1" || got.CapabilityType != domain.CapabilityImage {
		t.Fatalf("request = %+v, want the admission input's model and capability", got)
	}
}

// TestAdmissionGateCopiesSubject: the gate must not hand steps a pointer into
// the caller's own Subject, or a step mutating it would corrupt the caller.
func TestAdmissionGateCopiesSubject(t *testing.T) {
	in := admissionInput()
	gate := NewAdmissionGate(&recordingStep{
		name:    "mutate",
		capture: func(req *Request) { req.RuntimeSubject().TenantID = "clobbered" },
	})
	if err := gate.Admit(context.Background(), in); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if in.Subject.TenantID != "tenant-a" {
		t.Fatalf("caller's subject was mutated to %q by a step", in.Subject.TenantID)
	}
}

func TestAdmissionGateSkipsNilStepsAndEmptyGate(t *testing.T) {
	// A deployment without subscriptions passes a nil step; that must not panic.
	gate := NewAdmissionGate(nil, &recordingStep{name: "authz"}, nil)
	if err := gate.Admit(context.Background(), admissionInput()); err != nil {
		t.Fatalf("Admit with nil steps: %v", err)
	}
	if len(gate.Steps) != 1 {
		t.Fatalf("kept %d steps, want the 1 non-nil", len(gate.Steps))
	}

	var empty *AdmissionGate
	if err := empty.Admit(context.Background(), admissionInput()); err != nil {
		t.Fatalf("nil gate must admit: %v", err)
	}
}

// TestAdmissionGateAcceptsRealPipelineSteps: the gate must take the very step
// instances the pipeline uses, or the two would drift and an async submission
// would be judged by different rules than its execution.
func TestAdmissionGateAcceptsRealPipelineSteps(t *testing.T) {
	var (
		_ GateStep = (*QuotaCheckStep)(nil)
		_ GateStep = (*SubscriptionGateStep)(nil)
		_ GateStep = (*BalanceGateStep)(nil)
		_ GateStep = (*RouteCandidatesStep)(nil)
		_ GateStep = (*BillingGuardStep)(nil)
	)
	// And the steps the gate must never run are still full Steps, so their
	// exclusion is a choice at the wiring site rather than an accident.
	var (
		_ Step = (*RateLimitStep)(nil)
		_ Step = (*AuthNStep)(nil)
	)
}
