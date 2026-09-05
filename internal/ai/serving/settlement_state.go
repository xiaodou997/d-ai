package serving

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"syscall"

	"xiaodou/dai/internal/ai/domain"
)

// observeProviderStreamFrame recognizes terminal signals without assuming a
// particular provider's JSON envelope. The protocol-specific bridge handles
// richer conversion; this observer only records settlement facts.
func observeProviderStreamFrame(req *Request, payload []byte, eventType string) {
	if req == nil {
		return
	}
	initializeSettlementState(req)
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) ||
		eventType == "message_stop" || eventType == "response.completed" ||
		eventType == "response.complete" || eventType == "done" {
		markProviderTerminal(req, domain.ProviderTerminalCompleted)
		return
	}
	if eventType == "response.failed" || eventType == "response.error" || eventType == "error" {
		markProviderTerminal(req, domain.ProviderTerminalFailed)
		return
	}
	if eventType == "response.cancelled" || eventType == "response.canceled" || eventType == "cancelled" || eventType == "canceled" {
		markProviderTerminal(req, domain.ProviderTerminalCancelled)
		return
	}

	var envelope struct {
		Type   string `json:"type"`
		Status string `json:"status"`
		Error  any    `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(payload), &envelope) != nil {
		return
	}
	typ := strings.ToLower(strings.TrimSpace(envelope.Type))
	status := strings.ToLower(strings.TrimSpace(envelope.Status))
	switch {
	case strings.Contains(typ, "response.completed"), typ == "message_stop", typ == "done":
		markProviderTerminal(req, domain.ProviderTerminalCompleted)
	case strings.Contains(typ, "response.failed"), typ == "error" || envelope.Error != nil:
		markProviderTerminal(req, domain.ProviderTerminalFailed)
	case strings.Contains(typ, "response.cancel") || status == "cancelled" || status == "canceled":
		markProviderTerminal(req, domain.ProviderTerminalCancelled)
	case status == "completed" || status == "complete" || status == "succeeded":
		markProviderTerminal(req, domain.ProviderTerminalCompleted)
	case status == "failed":
		markProviderTerminal(req, domain.ProviderTerminalFailed)
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
	if req == nil {
		return false
	}
	initializeSettlementState(req)
	return req.RequestStatus == domain.RequestCancelled &&
		(req.CancellationOrigin == domain.CancellationClient || req.CancellationOrigin == domain.CancellationGateway || req.CancellationOrigin == domain.CancellationProvider) &&
		req.ProviderTerminalState != domain.ProviderTerminalCompleted &&
		req.ProviderTerminalState != domain.ProviderTerminalFailed
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
