package subscription

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/urm"
)

type reconcileOnlyRepo struct {
	Repo
	getPlanCalls   int
	finalizedOrder *Order
	finalizeEvent  string
}

func (r *reconcileOnlyRepo) GetPlan(context.Context, string) (*Plan, error) {
	r.getPlanCalls++
	return nil, ErrPlanNotFound
}

func (r *reconcileOnlyRepo) FinalizeOrder(_ context.Context, order *Order, eventID string) (*Subscription, error) {
	r.finalizedOrder = order
	r.finalizeEvent = eventID
	return &Subscription{ID: "sub-snapshot"}, nil
}

type reconcilePurchaser struct {
	request urm.StrictDebitRequest
}

func (p *reconcilePurchaser) DebitStrict(_ context.Context, request urm.StrictDebitRequest) (*urm.StrictDebitResponse, error) {
	p.request = request
	return &urm.StrictDebitResponse{AuthorizationID: "auth-snapshot"}, nil
}

func TestReconcileOrderUsesImmutableOrderSnapshotWithoutPlanLookup(t *testing.T) {
	repo := &reconcileOnlyRepo{}
	purchaser := &reconcilePurchaser{}
	service := NewService(repo, purchaser, zap.NewNop())
	order := &Order{
		ID:                                 "order-snapshot",
		OrderNo:                            "SUB_snapshot",
		TenantID:                           "tenant-snapshot",
		UserID:                             "user-snapshot",
		PlanID:                             "plan-may-now-be-edited-or-missing",
		PlanNameSnapshot:                   "Snapshot plan",
		PriceCredits:                       123,
		DurationDaysSnapshot:               30,
		TotalLimitMicroSnapshot:            1_000_000,
		GroupQuotaDebitMultipliersSnapshot: map[string]float64{"group-snapshot": 1.25},
	}

	service.reconcileOrder(context.Background(), order)

	if repo.getPlanCalls != 0 {
		t.Fatalf("reconcile looked up mutable plan %d times", repo.getPlanCalls)
	}
	if repo.finalizedOrder != order || repo.finalizeEvent != "auth-snapshot" {
		t.Fatalf("finalize did not receive original order snapshot: order=%p event=%q", repo.finalizedOrder, repo.finalizeEvent)
	}
	if purchaser.request.IdempotencyKey != "ai-sub-SUB_snapshot" || purchaser.request.UserMicro != 1_230_000 {
		t.Fatalf("unexpected debit replay: %+v", purchaser.request)
	}
}
