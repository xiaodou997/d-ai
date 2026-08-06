package serving

import (
	"context"

	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/riskcontrol"
)

// ============================================================================
// ContentModerationStep — risk-control-center content safety check
// ============================================================================

// ContentModerationStep runs the risk-control-center's keyword + AI
// moderation check on the last user message before the request reaches
// quota/routing (cheap early rejection). Checker may be nil (feature not
// wired) in which case the step is a no-op; Worker handles the observe-mode
// async path so this step never adds upstream latency in that mode.
//
// This step never mutates account status: pre_block only rejects the
// current request, and repeated violations only raise an ai_risk_events row
// for an admin to act on (see internal/riskcontrol).
type ContentModerationStep struct {
	Checker *riskcontrol.Checker
	Worker  *riskcontrol.Worker
}

func (s *ContentModerationStep) Name() string { return "content_moderation" }

func (s *ContentModerationStep) Execute(ctx context.Context, req *Request) error {
	if s.Checker == nil || s.Checker.Config == nil {
		return nil
	}
	cfg, err := s.Checker.Config.Get(ctx)
	if err != nil || !cfg.Enabled || cfg.Mode == domain.RiskControlModeOff {
		return nil
	}

	subject := req.RuntimeSubject()
	if subject == nil {
		return nil
	}

	text := extractModerationText(req)
	if text == "" {
		return nil
	}

	in := riskcontrol.CheckInput{
		RequestID:      req.RequestID,
		TenantID:       subject.TenantID,
		UserID:         subject.UserID,
		APIKeyID:       subject.APIKeyID,
		ModelCode:      req.ModelCode,
		CapabilityType: string(req.CapabilityType),
		Text:           text,
	}

	if cfg.Mode == domain.RiskControlModeObserve {
		if s.Worker != nil {
			s.Worker.Submit(riskcontrol.WorkerTask{Config: cfg, Input: in})
		}
		return nil
	}

	// pre_block: block synchronously on this goroutine so the log write
	// happens before the client sees the rejection.
	det := s.Checker.Detect(ctx, cfg, text)
	// Cache hits skip Record (no log, no violation count) — same text is
	// one behavior, not N.
	if !det.FromCache {
		s.Checker.Record(ctx, cfg, in, det, cfg.Mode)
	}
	if det.Flagged {
		return apiError(cfg.BlockStatusCode, "content_moderation_blocked", cfg.BlockMessage)
	}
	return nil
}

func (s *ContentModerationStep) Rollback(_ context.Context, _ *Request) {}

func extractModerationText(req *Request) string {
	if req.Envelope == nil || len(req.Envelope.ClientBody) == 0 {
		return ""
	}
	// Image requests carry their user text in `prompt`, not in `messages`, so
	// the conversation extractor returns nothing for them — moderation was a
	// no-op for every generated and edited image until this branch existed.
	if req.ClientProtocol == domain.ProtocolOpenAIImages {
		return riskcontrol.ExtractImagePrompt(req.Envelope.ClientBody, moderationContentType(req))
	}
	messages, _ := audit.ExtractRequestPayload(req.Envelope.ClientBody, req.ClientProtocol)
	return riskcontrol.ExtractLastUserText(req.ClientProtocol, messages)
}

// moderationContentType reports the request's transport so multipart image
// edits can be parsed. Queued tasks go through a synthesized http.Request that
// carries the persisted Content-Type, so this works on the async path too.
func moderationContentType(req *Request) string {
	if req.Envelope == nil || req.Envelope.R == nil {
		return ""
	}
	return req.Envelope.R.Header.Get("Content-Type")
}
