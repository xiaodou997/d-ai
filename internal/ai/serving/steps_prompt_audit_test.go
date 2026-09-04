package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/promptaudit"
)

type promptAuditCheckerStub struct {
	decision promptaudit.Decision
	input    promptaudit.Input
}

func (s *promptAuditCheckerStub) Check(_ context.Context, in promptaudit.Input) promptaudit.Decision {
	s.input = in
	return s.decision
}

func TestPromptAuditStepBlocksBeforeLaterPipelineSteps(t *testing.T) {
	checker := &promptAuditCheckerStub{decision: promptaudit.Decision{Allow: false, ErrorCode: promptaudit.ErrorBlocked}}
	later := &countingStep{}
	req := &Request{Envelope: &RequestEnvelope{ClientBody: []byte(`{"messages":[{"role":"user","content":"x"}]}`)}, Subject: &coreidentity.Subject{TenantID: "tenant-1", UserID: "user-1", APIKeyID: "11111111-1111-1111-1111-111111111111"}, ClientProtocol: domain.ProtocolOpenAIChat, ModelCode: "m", CapabilityType: domain.CapabilityChat}
	err := NewPipeline(&PromptAuditStep{Checker: checker}, later).Run(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden || apiErr.Code != promptaudit.ErrorBlocked {
		t.Fatalf("err=%v", err)
	}
	if later.calls != 0 {
		t.Fatalf("later calls=%d", later.calls)
	}
	if checker.input.TenantID != "tenant-1" || string(checker.input.Body) == "" {
		t.Fatalf("input=%+v", checker.input)
	}
}

type countingStep struct{ calls int }

func (s *countingStep) Name() string                            { return "later" }
func (s *countingStep) Execute(context.Context, *Request) error { s.calls++; return nil }
func (s *countingStep) Rollback(context.Context, *Request)      {}
