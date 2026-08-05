package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"xiaodou/dai/internal/ai/billingledger"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

type recordingBillingAdmitter struct {
	intent    billingledger.Intent
	admission billingledger.Admission
	err       error
}

func (a *recordingBillingAdmitter) Admit(_ context.Context, intent billingledger.Intent) (billingledger.Admission, error) {
	a.intent = intent
	return a.admission, a.err
}

func TestBillingAdmissionStepUsesStructuredBillingErrorsAndNeverAttemptsUpstream(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"insufficient", billingledger.ErrInsufficientBalance, http.StatusPaymentRequired, "billing_insufficient_balance"},
		{"conflict", billingledger.ErrAdmissionConflict, http.StatusServiceUnavailable, "billing_lease_conflict"},
		{"protocol", billingledger.ErrProtocolViolation, http.StatusServiceUnavailable, "billing_protocol_violation"},
		{"dependency", billingledger.ErrDependencyUnavailable, http.StatusServiceUnavailable, "billing_dependency_unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			admitter := &recordingBillingAdmitter{err: tc.err}
			req := billingAdmissionRequest()
			err := (&BillingAdmissionStep{Coordinator: admitter}).Execute(context.Background(), req)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want APIError", err)
			}
			if apiErr.Status != tc.wantStatus || apiErr.Code != tc.wantCode {
				t.Fatalf("error = (%d,%s), want (%d,%s)", apiErr.Status, apiErr.Code, tc.wantStatus, tc.wantCode)
			}
			if len(req.Attempts) != 0 || req.BillingAdmissionActive {
				t.Fatalf("billing failure must precede upstream: attempts=%d active=%v", len(req.Attempts), req.BillingAdmissionActive)
			}
		})
	}
}

func TestBillingAdmissionStepStoresOnlyCoordinatorReceipt(t *testing.T) {
	admitter := &recordingBillingAdmitter{admission: billingledger.Admission{
		RequestID: "request-admit", WindowID: "bw_test", LeaseID: "CL_test",
	}}
	req := billingAdmissionRequest()
	if err := (&BillingAdmissionStep{Coordinator: admitter}).Execute(context.Background(), req); err != nil {
		t.Fatalf("execute admission: %v", err)
	}
	if req.BillingWindowID != "bw_test" || req.BillingLeaseID != "CL_test" || !req.BillingAdmissionActive {
		t.Fatalf("request receipt = window:%s lease:%s active:%v",
			req.BillingWindowID, req.BillingLeaseID, req.BillingAdmissionActive)
	}
	if admitter.intent.OwnerType != string(domain.OwnerUser) || !admitter.intent.WantTenant ||
		!admitter.intent.WantUser || admitter.intent.TenantID != "tenant-admit" ||
		admitter.intent.UserID != "user-admit" {
		t.Fatalf("intent = %#v", admitter.intent)
	}
}

func billingAdmissionRequest() *Request {
	routeID := "11111111-1111-1111-1111-111111111111"
	return &Request{
		RequestID: "request-admit",
		Subject: &coreidentity.Subject{
			Scope: coreidentity.ScopeUser, TenantID: "tenant-admit", UserID: "user-admit",
		},
		Candidates: []*domain.RouteCandidate{{
			RouteID: routeID, TenantMultiplier: 1,
		}},
		BillingSnapshots: map[string]domain.BillingSnapshot{
			routeID: {EffectiveUserMultiplier: 1},
		},
	}
}
