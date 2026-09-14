package serving

import (
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

func observeReportedUsage(req *Request, data []byte, event string) {
	if req == nil || req.Candidate == nil || !domain.UsesReportedTokenBilling(req.CapabilityType) {
		return
	}
	req.UsageEvidence = formats.ObserveUsage(req.UsageEvidence, data, event, req.Candidate.Protocol, req.CapabilityType)
	ApplyReportedUsage(req)
}

// ApplyReportedUsage is also called at settlement, so stale estimates or
// bridge-generated placeholder counters cannot enter the ledger.
func ApplyReportedUsage(req *Request) {
	if req == nil || !domain.UsesReportedTokenBilling(req.CapabilityType) {
		return
	}
	req.TokenUsage = req.UsageEvidence.Tokens()
	req.TokenCountSource = domain.TokenUsageSourceMissing
	if req.UsageEvidence.Reported() {
		req.TokenCountSource = domain.TokenUsageSourceUpstream
	}
}

func resetAttemptUsage(req *Request) {
	if !domain.UsesReportedTokenBilling(req.CapabilityType) {
		req.TokenUsage.ImageCount = 0
		req.TokenUsage.VideoSeconds = 0
		req.MediaUsageConfirmed = false
	}
	req.UsageEvidence = domain.UsageEvidence{}
	ApplyReportedUsage(req)
	req.RequestStatus = ""
	req.HTTPStatus = 0
	req.FirstTokenMs = 0
	req.ProviderTerminalState = domain.ProviderTerminalUnknown
	req.ClientDeliveryState = domain.ClientDeliveryUnknown
	req.CancellationOrigin = domain.CancellationNone
	req.ResponseSummaryState = domain.ResponseSummaryUnavailable
	req.AuditResponseMessage = nil
	req.UpstreamErrorCode = ""
	req.ErrorCode, req.ErrorMessage, req.InternalErrorDetail, req.FailedStep, req.BillingReason = "", "", "", "", ""
}
