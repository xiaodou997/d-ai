package subscription

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
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
	request DebitRequest
}

func (p *reconcilePurchaser) DebitStrict(_ context.Context, request DebitRequest) (*DebitReceipt, error) {
	p.request = request
	return &DebitReceipt{AuthorizationID: "auth-snapshot"}, nil
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
		PriceMicroUSD:                      123,
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
	if purchaser.request.IdempotencyKey != "ai-sub-SUB_snapshot" || purchaser.request.UserMicro != 123 {
		t.Fatalf("unexpected debit replay: %+v", purchaser.request)
	}
}

func TestSubscriptionJanitorLifecycleIsIdempotent(t *testing.T) {
	service := NewService(nil, nil, zap.NewNop())
	service.Start(context.Background())
	service.Start(context.Background())
	if got := service.Health(); !got.Started || got.Stopped {
		t.Fatalf("running subscription health = %+v", got)
	}

	shortCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Stop(shortCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := service.Stop(shortCtx); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	if got := service.Health(); !got.Stopped {
		t.Fatalf("stopped subscription health = %+v", got)
	}
}

func TestSubscriptionJanitorCannotStartAfterStop(t *testing.T) {
	service := NewService(nil, nil, zap.NewNop())
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start error = %v", err)
	}
	service.Start(context.Background())
	if got := service.Health(); got.Started || !got.Stopped {
		t.Fatalf("subscription health after stop-before-start = %+v", got)
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("repeated Stop error = %v", err)
	}
}
