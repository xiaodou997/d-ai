package serving

import (
	"context"

	"xiaodou/dai/internal/ai/core/catalog"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
)

// AdmissionGate answers "would this request be allowed right now?" without
// executing it.
//
// It exists for async submission. A queued task is accepted while the caller is
// still on the phone but runs much later, so the obvious failures — no balance,
// no quota, model not granted, no route priced — have to be reported at submit
// time or the caller learns about them by polling a failed task.
//
// It runs the same gate steps the pipeline runs, in the same order, against a
// synthesized request with no envelope. That is the point: the reason an async
// submission is refused is the reason the synchronous call would have given.
//
// It is advisory. Execution runs these steps again for real, and that run is
// authoritative — so passing the gate is not a promise, and there is no window
// where the gate's verdict is trusted in place of the real one.
//
// Deliberately excluded:
//
//   - AuthNStep: the caller is already authenticated; the subject is the input.
//   - BillingAdmissionStep: the execution pipeline takes a durable credit-lease
//     admission. A task that
//     may sit in the queue for minutes must not hold one across the wait.
//   - RateLimitStep: rate limits belong to the moment of execution, not the
//     moment of submission, and it holds a concurrency lease.
//   - ExecuteStep: it would do the work.
type AdmissionGate struct {
	// Steps are gate steps to run in order. Every one must be free of external
	// side effects — the gate may run for a request that never executes.
	Steps []GateStep
}

// GateStep is the part of Step the gate uses. Every pipeline Step satisfies it,
// so the gate runs the real ones; but narrowing it here makes the gate's central
// property structural rather than a comment: it cannot roll anything back,
// because it is only ever allowed to run steps that have nothing to undo.
type GateStep interface {
	Name() string
	Execute(ctx context.Context, req *Request) error
}

// AdmissionInput is what the gate needs to synthesize a request. It mirrors what
// the runtime derives from the HTTP body, minus the body.
type AdmissionInput struct {
	Subject           coreidentity.Subject
	ModelCode         string
	RequestedModel    string
	RuntimeCapability catalog.Capability
	CapabilityType    domain.CapabilityType
	ClientProtocol    domain.UpstreamProtocol
}

// NewAdmissionGate wires the gate from the same step instances the pipeline
// uses, so the two can never drift apart in configuration.
//
// Pass the steps in pipeline order: AuthZ, QuotaCheck, SubscriptionGate,
// RouteCandidates, BillingGuard. Nil steps are skipped, which keeps callers
// that have not configured a collaborator (no subscriptions, say) working.
func NewAdmissionGate(steps ...GateStep) *AdmissionGate {
	live := make([]GateStep, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		live = append(live, step)
	}
	return &AdmissionGate{Steps: live}
}

// Admit reports whether the request would be allowed. It returns nil, or an
// *APIError carrying the status code the synchronous endpoint would have used.
//
// It calls each step directly instead of going through Pipeline.Run, because
// Run drives finalizers — which write a usage log. A submission must not
// produce a usage record for work that has not happened.
func (g *AdmissionGate) Admit(ctx context.Context, in AdmissionInput) error {
	if g == nil || len(g.Steps) == 0 {
		return nil
	}
	subject := in.Subject
	req := &Request{
		// No envelope: nothing here may read the inbound HTTP request. Every
		// step in this gate has been checked to only read Subject, ModelCode
		// and routing state.
		Envelope:          nil,
		Subject:           &subject,
		RequestedModel:    in.RequestedModel,
		ModelCode:         in.ModelCode,
		RuntimeCapability: in.RuntimeCapability,
		CapabilityType:    in.CapabilityType,
		ClientProtocol:    in.ClientProtocol,
		ExecutionMode:     coreruntime.ExecutionModeAsync,
	}
	for _, step := range g.Steps {
		if err := step.Execute(ctx, req); err != nil {
			return err
		}
	}
	return nil
}
