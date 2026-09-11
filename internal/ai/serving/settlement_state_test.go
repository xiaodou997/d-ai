package serving

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func TestShouldVoidBillingBeforeProviderTerminal(t *testing.T) {
	req := &Request{RequestStatus: domain.RequestCancelled}
	markClientCancellation(req)
	if !ShouldVoidBilling(req) {
		t.Fatal("client cancellation before terminal should void billing")
	}
	if req.BillingReason != "client_disconnect_before_terminal" {
		t.Fatalf("billing reason = %q", req.BillingReason)
	}
}

func TestClientDeliveryFailureAfterProviderTerminalRemainsBillable(t *testing.T) {
	req := &Request{ProviderTerminalState: domain.ProviderTerminalCompleted, UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}}
	markClientCancellation(req)
	if req.RequestStatus != domain.RequestFailed {
		t.Fatalf("request status = %q, want failed delivery", req.RequestStatus)
	}
	if ShouldVoidBilling(req) {
		t.Fatal("terminal provider execution must remain billable")
	}
}

func TestStreamClientCancelledRecognizesParentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dc := newDeadlineController(ctx, domain.RouteTimeouts{
		ResponseHeader: time.Minute,
		FirstByte:      time.Minute,
		Idle:           time.Minute,
		MaxDuration:    time.Minute,
	})
	defer dc.stop()
	cancel()
	if !streamClientCancelled(dc, context.Canceled) {
		t.Fatal("parent context cancellation was not recognized")
	}
}

func TestObserveProviderStreamFrameRecognizesResponsesTerminal(t *testing.T) {
	req := &Request{}
	observeProviderStreamFrame(req, []byte(`{"type":"response.completed","status":"completed"}`), "")
	if req.ProviderTerminalState != domain.ProviderTerminalCompleted {
		t.Fatalf("provider terminal state = %q, want completed", req.ProviderTerminalState)
	}
}

func TestObserveProviderStreamEventOnlyFailureTerminal(t *testing.T) {
	req := &Request{}
	observeProviderStreamFrame(req, []byte(`{}`), "response.failed")
	if req.ProviderTerminalState != domain.ProviderTerminalFailed {
		t.Fatalf("provider terminal state = %q, want failed", req.ProviderTerminalState)
	}
}

func TestUpstreamCancellationIsNotClientCancellation(t *testing.T) {
	req := &Request{UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}}
	markUpstreamCancellation(req)
	if req.RequestStatus != domain.RequestFailed || req.CancellationOrigin != domain.CancellationProvider {
		t.Fatalf("upstream cancellation state = %s/%s", req.RequestStatus, req.CancellationOrigin)
	}
	if ShouldVoidBilling(req) {
		t.Fatal("upstream cancellation must not be treated as a customer disconnect")
	}
}

func TestProviderCancellationVoidsCustomerCharge(t *testing.T) {
	req := &Request{
		RequestStatus:         domain.RequestCancelled,
		ProviderTerminalState: domain.ProviderTerminalCancelled,
		CancellationOrigin:    domain.CancellationProvider,
	}
	if !ShouldVoidBilling(req) {
		t.Fatal("provider cancellation should void customer charge")
	}
}
