package serving

import (
	"net/http"
	"xiaodou/dai/internal/ai/domain"
)

// RequestFailure is a request-level error. An attempt failure or delivery
// interruption alone is not a request failure. PublicMessage is already scrubbed
// by egress; InternalErrorDetail never enters this object.
type RequestFailure struct {
	Origin  string `json:"origin"`
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CompletionDecision struct {
	EndReason    string          `json:"end_reason"`
	ChargeReason string          `json:"charge_reason"`
	Billable     bool            `json:"billable"`
	Error        *RequestFailure `json:"error,omitempty"`
}

// DecideCompletion consumes structured execution facts, never error-message
// text or the legacy aggregate RequestStatus. This is the shared boundary for
// error visibility, financial policy and request statistics.
func DecideCompletion(req *Request) CompletionDecision {
	if req == nil {
		return CompletionDecision{EndReason: "unconfirmed", ChargeReason: "unconfirmed_execution"}
	}
	d := CompletionDecision{EndReason: "completed", ChargeReason: "reported_usage"}
	client := req.CancellationOrigin == domain.CancellationClient || req.ErrorCode == "client_disconnected"
	delivery := client || req.ErrorCode == "stream_write_error"
	failed := req.ProviderTerminalState == domain.ProviderTerminalFailed ||
		(req.HTTPStatus >= http.StatusBadRequest && req.HTTPStatus != 499) ||
		(!delivery && (req.ErrorCode != "" || req.ProviderTerminalState == domain.ProviderTerminalIncomplete || req.ProviderTerminalState == domain.ProviderTerminalCancelled))
	if failed {
		origin := "gateway"
		if req.ProviderTerminalState == domain.ProviderTerminalFailed || req.ProviderTerminalState == domain.ProviderTerminalIncomplete || req.CancellationOrigin == domain.CancellationProvider {
			origin = "upstream"
		}
		code := req.ErrorCode
		if code == "" || delivery {
			code = "provider_terminal_error"
		}
		stage := req.FailedStep
		if stage == "" {
			stage = "execute"
		}
		d.EndReason, d.ChargeReason = "request_error", "request_error_waived"
		message := req.ErrorMessage
		if delivery && req.ProviderTerminalState == domain.ProviderTerminalFailed {
			message = "上游明确报错，本次未计费"
		}
		d.Error = &RequestFailure{Origin: origin, Stage: stage, Code: code, Message: message}
		return d
	}
	if delivery || req.HTTPStatus == 499 {
		d.EndReason = "client_interrupted"
		d.ChargeReason = "reported_usage_before_disconnect"
	}
	if domain.UsesReportedTokenBilling(req.CapabilityType) {
		if !req.UsageEvidence.Reported() {
			d.ChargeReason = "missing_usage"
			return d
		}
		d.Billable = true
		return d
	}
	// Media needs a provider-confirmed result, not the requested quantity.
	if req.ProviderTerminalState != domain.ProviderTerminalCompleted || !req.MediaUsageConfirmed {
		d.ChargeReason = "missing_media_evidence"
		return d
	}
	d.Billable = true
	d.ChargeReason = "confirmed_media_output"
	return d
}
