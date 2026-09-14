package serving

import (
	"bytes"
	"context"
	"errors"
	"syscall"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

// observeProviderStreamFrame recognizes terminal signals without assuming a
// particular provider's JSON envelope. The protocol-specific bridge handles
// richer conversion; this observer only records settlement facts.
func observeProviderStreamFrame(req *Request, payload []byte, eventType string) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	outcome := formats.InspectStreamOutcome(payload, eventType)
	if outcome.Event == "[DONE]" && req.Candidate != nil && req.Candidate.Protocol == domain.ProtocolOpenAIResponses {
		return // Responses requires an explicit response terminal, not just EOF/[DONE].
	}
	if outcome.State != "" {
		markProviderTerminal(req, outcome.State)
	}
	if outcome.State == domain.ProviderTerminalFailed || outcome.State == domain.ProviderTerminalIncomplete || outcome.State == domain.ProviderTerminalCancelled {
		req.FailedStep = "execute"
		if outcome.Message != "" {
			detail := outcome.Message
			if outcome.Code != "" {
				detail = outcome.Code + ": " + detail
			}
			req.InternalErrorDetail = RedactInternalErrorDetail(detail)
		}
		if outcome.Code != "" {
			req.UpstreamErrorCode = outcome.Code
		}
	}

}

func updateResponseSummaryState(req *Request, summary []byte, complete bool) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	if len(bytes.TrimSpace(summary)) == 0 {
		req.ResponseSummaryState = domain.ResponseSummaryEmpty
		return
	}
	if complete {
		req.ResponseSummaryState = domain.ResponseSummaryComplete
	} else {
		req.ResponseSummaryState = domain.ResponseSummaryPartial
	}
}

// initializeSettlementState gives every request an explicit baseline. The
// zero value remains compatible with callers that construct Request directly.
func initializeSettlementState(req *Request) {
	if req == nil {
		return
	}
	if req.ProviderTerminalState == "" {
		req.ProviderTerminalState = domain.ProviderTerminalUnknown
	}
	if req.ClientDeliveryState == "" {
		req.ClientDeliveryState = domain.ClientDeliveryUnknown
	}
	if req.CancellationOrigin == "" {
		req.CancellationOrigin = domain.CancellationNone
	}
	if req.ResponseSummaryState == "" {
		req.ResponseSummaryState = domain.ResponseSummaryUnavailable
	}
}

// EnsureSettlementState normalizes the lifecycle fields for adapters and
// finalizers that may receive a request which did not execute the relay step.
func EnsureSettlementState(req *Request) { initializeSettlementState(req) }

func markProviderTerminal(req *Request, state domain.ProviderTerminalState) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	if req.ProviderTerminalState == domain.ProviderTerminalFailed && state != domain.ProviderTerminalFailed {
		return
	}
	if state == domain.ProviderTerminalCompleted && (req.ProviderTerminalState == domain.ProviderTerminalFailed || req.ProviderTerminalState == domain.ProviderTerminalIncomplete || req.ProviderTerminalState == domain.ProviderTerminalCancelled) {
		return
	}
	req.ProviderTerminalState = state
}

func markClientDelivery(req *Request, state domain.ClientDeliveryState) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	req.ClientDeliveryState = state
}

func markClientCancellation(req *Request) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	req.CancellationOrigin = domain.CancellationClient
	markClientDelivery(req, domain.ClientDeliveryDisconnected)
	if req.ProviderTerminalState == domain.ProviderTerminalCompleted || req.ProviderTerminalState == domain.ProviderTerminalFailed {
		req.RequestStatus = domain.RequestFailed
		req.BillingReason = "client_delivery_failed_after_terminal"
	} else {
		req.RequestStatus = domain.RequestCancelled
		req.BillingReason = "client_disconnect_before_terminal"
	}
}

func markGatewayCancellation(req *Request, reason string) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	req.RequestStatus = domain.RequestCancelled
	req.CancellationOrigin = domain.CancellationGateway
	if reason != "" {
		req.BillingReason = reason
	}
}

func markUpstreamCancellation(req *Request) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	req.RequestStatus = domain.RequestFailed
	req.CancellationOrigin = domain.CancellationProvider
	req.ProviderTerminalState = domain.ProviderTerminalIncomplete
	req.BillingReason = "upstream_cancelled"
}

func isClientContextCancellation(ctx context.Context) bool {
	return ctx != nil && errors.Is(context.Cause(ctx), context.Canceled)
}

// shouldVoidBilling is intentionally conservative: a request is customer
// billable after interruption only when a provider terminal event was
// observed. This protects customers from paying for an unconfirmed stream
// while preserving raw observed usage in the usage log.
func shouldVoidBilling(req *Request) bool {
	return !DecideCompletion(req).Billable
}

// ShouldVoidBilling is the settlement boundary used by adapters. It exposes
// only the final chargeability decision, keeping lifecycle details in serving.
func ShouldVoidBilling(req *Request) bool { return shouldVoidBilling(req) }

func isClientDisconnectError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return false
}
