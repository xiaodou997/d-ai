package serving

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/billingledger"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/subscription"
)

// BillingAdmitter is the serving seam of the deep Billing Ledger module.
// Serving knows only how to admit a request; lease rotation and settlement are
// implementation details behind this interface.
type BillingAdmitter interface {
	Admit(context.Context, billingledger.Intent) (billingledger.Admission, error)
}

type BillingAdmissionStep struct {
	Coordinator BillingAdmitter
	Logger      *zap.Logger
	Metrics     interface {
		BillingAdmissionFailure(reason string)
	}
}

func (s *BillingAdmissionStep) Name() string { return "billing_admission" }

func (s *BillingAdmissionStep) Execute(ctx context.Context, req *Request) error {
	subject := req.RuntimeSubject()
	if subject == nil || subject.TenantID == "" {
		return nil
	}
	ownerType := runtimeSubjectLegacyOwnerType(subject)
	userID := subject.UserID
	wantTenant := requestMayChargeTenant(req)
	wantUser := req.BillingSource != subscription.BillingSourceSubscription &&
		ownerType == domain.OwnerUser && userID != "" && requestMayChargeUser(req)
	if !wantUser && ownerType != domain.OwnerUser {
		userID = ""
	}
	if !wantTenant && !wantUser {
		return nil
	}
	if s.Coordinator == nil {
		s.recordAdmissionFailure("dependency_unavailable")
		return apiError(http.StatusServiceUnavailable, "billing_dependency_unavailable",
			"billing admission is unavailable")
	}
	admission, err := s.Coordinator.Admit(ctx, billingledger.Intent{
		RequestID:  req.RequestID,
		OwnerType:  string(ownerType),
		TenantID:   subject.TenantID,
		UserID:     userID,
		WantTenant: wantTenant,
		WantUser:   wantUser,
		RequestTTL: RequestLeaseTTL(req),
	})
	if err != nil {
		s.logger().Warn("billing admission failed",
			zap.Error(err), zap.String("request_id", req.RequestID),
			zap.String("tenant_id", subject.TenantID), zap.String("user_id", subject.UserID))
		switch {
		case errors.Is(err, billingledger.ErrInsufficientBalance):
			s.recordAdmissionFailure("insufficient_balance")
			return apiError(http.StatusPaymentRequired, "billing_insufficient_balance",
				"account has no available balance or has outstanding debt")
		case errors.Is(err, billingledger.ErrAdmissionConflict):
			s.recordAdmissionFailure("lease_conflict")
			return apiErrorWithCause(http.StatusServiceUnavailable, "billing_lease_conflict",
				"billing lease is being reconciled", err)
		case errors.Is(err, billingledger.ErrProtocolViolation):
			s.recordAdmissionFailure("protocol_violation")
			return apiErrorWithCause(http.StatusServiceUnavailable, "billing_protocol_violation",
				"billing state requires reconciliation", err)
		default:
			s.recordAdmissionFailure("dependency_unavailable")
			return apiErrorWithCause(http.StatusServiceUnavailable, "billing_dependency_unavailable",
				"billing admission is temporarily unavailable", err)
		}
	}
	req.BillingWindowID = admission.WindowID
	req.BillingLeaseID = admission.LeaseID
	req.BillingAdmissionActive = admission.LeaseID != ""
	return nil
}

func (s *BillingAdmissionStep) recordAdmissionFailure(reason string) {
	if s.Metrics != nil {
		s.Metrics.BillingAdmissionFailure(reason)
	}
}

func (s *BillingAdmissionStep) Rollback(_ context.Context, _ *Request) {}

func (s *BillingAdmissionStep) logger() *zap.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return zap.L()
}

func requestMayChargeTenant(req *Request) bool {
	if req == nil || len(req.Candidates) == 0 {
		return true
	}
	sawCandidate := false
	for _, candidate := range req.Candidates {
		if candidate == nil {
			continue
		}
		sawCandidate = true
		if candidate.TenantMultiplier != 0 {
			return true
		}
	}
	return !sawCandidate
}

func requestMayChargeUser(req *Request) bool {
	if req == nil || len(req.Candidates) == 0 {
		return true
	}
	sawCandidate := false
	for _, candidate := range req.Candidates {
		if candidate == nil {
			continue
		}
		sawCandidate = true
		snapshot, ok := req.BillingSnapshots[candidate.RouteID]
		if !ok || snapshot.EffectiveUserMultiplier != 0 {
			return true
		}
	}
	return !sawCandidate
}

// RequestLeaseTTL covers the request-level retry deadline. Attempts are bounded
// by the same deadline, so summing every route duration would retain admissions
// long after execution is guaranteed to have stopped.
func RequestLeaseTTL(req *Request) time.Duration {
	ttl := defaultRetryMaxElapsed(req)
	if ttl < time.Minute {
		ttl = time.Minute
	}
	return ttl + 2*time.Minute
}
