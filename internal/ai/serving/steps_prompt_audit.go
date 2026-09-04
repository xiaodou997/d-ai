package serving

import (
	"context"
	"net/http"

	"xiaodou/dai/internal/ai/promptaudit"
)

type PromptAuditChecker interface {
	Check(context.Context, promptaudit.Input) promptaudit.Decision
}

// PromptAuditStep runs after authentication and before any quota, routing,
// billing or upstream side effect. Observe mode only submits an in-memory job;
// blocking mode fails closed when the Guard cannot make a trustworthy decision.
type PromptAuditStep struct {
	Checker PromptAuditChecker
}

func (s *PromptAuditStep) Name() string { return "prompt_audit" }

func (s *PromptAuditStep) Execute(ctx context.Context, req *Request) error {
	if s == nil || s.Checker == nil || req == nil || req.Envelope == nil || len(req.Envelope.ClientBody) == 0 {
		return nil
	}
	subject := req.RuntimeSubject()
	if subject == nil {
		return nil
	}
	decision := s.Checker.Check(ctx, promptaudit.Input{
		RequestID: req.RequestID, TenantID: subject.TenantID, UserID: subject.UserID,
		APIKeyID: subject.APIKeyID, ModelCode: req.ModelCode,
		CapabilityType: string(req.CapabilityType), Protocol: string(req.ClientProtocol),
		Body: req.Envelope.ClientBody,
	})
	if decision.Allow {
		return nil
	}
	switch decision.ErrorCode {
	case promptaudit.ErrorBlocked:
		return apiError(http.StatusForbidden, promptaudit.ErrorBlocked, "提示词安全审计拒绝了该请求，请调整输入后重试")
	case promptaudit.ErrorInvalidResponse:
		return apiError(http.StatusServiceUnavailable, promptaudit.ErrorInvalidResponse, "提示词安全审计暂时不可用，请稍后重试")
	default:
		return apiError(http.StatusServiceUnavailable, promptaudit.ErrorUnavailable, "提示词安全审计暂时不可用，请稍后重试")
	}
}

func (s *PromptAuditStep) Rollback(context.Context, *Request) {}
