package billingledger

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/testsupport"
)

type memoryLeasePort struct {
	mu              sync.Mutex
	now             func() time.Time
	byWindow        map[string]*CreditLease
	byID            map[string]*CreditLease
	acquireCalls    int
	renewCalls      int
	settleCalls     int
	failAcquireOnce bool
	failRenewOnce   bool
	failSettleOnce  bool
}

func newMemoryLeasePort(now func() time.Time) *memoryLeasePort {
	return &memoryLeasePort{
		now: now, byWindow: map[string]*CreditLease{},
		byID: map[string]*CreditLease{},
	}
}

func (p *memoryLeasePort) AcquireCreditLease(_ context.Context, req AcquireLease) (*CreditLease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.acquireCalls++
	if existing := p.byWindow[req.ClientWindowID]; existing != nil {
		return cloneLease(existing), nil
	}
	now := p.now()
	res := &CreditLease{
		LeaseID: "CL_" + req.ClientWindowID, ClientWindowID: req.ClientWindowID,
		TenantID: req.TenantID, UserID: req.UserID,
		GrantedTenantMicro: req.RequestedTenantMicro,
		GrantedUserMicro:   req.RequestedUserMicro,
		EscrowState:        "active", SettlementState: "unsettled", Version: 1,
		ExpiresAt:    now.Add(time.Duration(req.TTLSeconds) * time.Second),
		GraceUntil:   now.Add(time.Duration(req.TTLSeconds+req.GraceSeconds) * time.Second),
		AccountState: AccountStateOK, AllowFurtherUsage: true,
	}
	p.byWindow[req.ClientWindowID] = res
	p.byID[res.LeaseID] = res
	if p.failAcquireOnce {
		p.failAcquireOnce = false
		return nil, errors.New("acquire response lost")
	}
	return cloneLease(res), nil
}

func (p *memoryLeasePort) RenewCreditLease(_ context.Context, leaseID string, req RenewLease) (*CreditLease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.renewCalls++
	res := p.byID[leaseID]
	if res == nil {
		return nil, ErrProtocolViolation
	}
	if res.Version != req.Version || res.SettlementState == "settled" {
		return nil, ErrAdmissionConflict
	}
	res.Version++
	res.EscrowState = "active"
	res.ExpiresAt = p.now().Add(time.Duration(req.TTLSeconds) * time.Second)
	res.GraceUntil = res.ExpiresAt.Add(time.Duration(req.GraceSeconds) * time.Second)
	if p.failRenewOnce {
		p.failRenewOnce = false
		return nil, errors.New("renew response lost")
	}
	return cloneLease(res), nil
}

func (p *memoryLeasePort) SettleCreditLease(_ context.Context, leaseID string, req SettleLease) (*CreditLease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settleCalls++
	res := p.byID[leaseID]
	if res == nil {
		return nil, ErrProtocolViolation
	}
	if res.SettlementState == "settled" {
		if res.SettlementID != req.SettlementID || res.ActualTenantMicro == nil ||
			res.ActualUserMicro == nil || *res.ActualTenantMicro != req.ActualTenantMicro ||
			*res.ActualUserMicro != req.ActualUserMicro {
			return nil, ErrAdmissionConflict
		}
		return cloneLease(res), nil
	}
	now := p.now()
	res.EscrowState = "released"
	res.SettlementState = "settled"
	res.Version++
	res.SettlementID = req.SettlementID
	res.ActualTenantMicro = int64TestPtr(req.ActualTenantMicro)
	res.ActualUserMicro = int64TestPtr(req.ActualUserMicro)
	res.TenantDeductedMicro = req.ActualTenantMicro
	res.UserDeductedMicro = req.ActualUserMicro
	res.SettledEventID = "EV_" + req.SettlementID
	res.SettledAt = &now
	if p.failSettleOnce {
		p.failSettleOnce = false
		return nil, errors.New("settlement response lost")
	}
	return cloneLease(res), nil
}

func (p *memoryLeasePort) GetCreditLease(_ context.Context, leaseID string) (*CreditLease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	res := p.byID[leaseID]
	if res == nil {
		return nil, ErrProtocolViolation
	}
	return cloneLease(res), nil
}

func TestCoordinatorAdmitCompleteAndSettleThroughItsInterface(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	port := newMemoryLeasePort(func() time.Time { return now })
	coordinator := New(pool, port, Config{
		LeaseTTL: 10 * time.Minute, LeaseGrace: 15 * time.Minute,
		WindowMaxAge: 5 * time.Minute, AdmissionHeadroom: time.Second,
	}, nil)
	coordinator.now = func() time.Time { return now }

	intent := Intent{
		RequestID: "request-interface", OwnerType: "user",
		TenantID: "tenant-interface", UserID: "user-interface",
		WantTenant: true, WantUser: true, RequestTTL: time.Minute,
	}
	admission, err := coordinator.Admit(ctx, intent)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if admission.WindowID == "" || admission.LeaseID == "" {
		t.Fatalf("invalid admission: %#v", admission)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mismatched completion: %v", err)
	}
	if err := coordinator.Complete(ctx, tx, Completion{
		RequestID: intent.RequestID, ExpectedLeaseID: "CL_wrong",
		TenantMicro: 700, UserMicro: 900,
	}); !errors.Is(err, ErrProtocolViolation) {
		_ = tx.Rollback(ctx)
		t.Fatalf("mismatched completion lease error = %v, want protocol violation", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback mismatched completion: %v", err)
	}
	completeBillingRequest(t, ctx, coordinator, intent.RequestID, 700, 900)
	completeBillingRequest(t, ctx, coordinator, intent.RequestID, 700, 900) // idempotent replay
	if _, err := coordinator.Admit(ctx, intent); !errors.Is(err, ErrAdmissionConflict) {
		t.Fatalf("completed request re-admission error = %v, want conflict", err)
	}

	if err := coordinator.beginDrain(ctx, admission.WindowID, "", ""); err != nil {
		t.Fatalf("begin drain: %v", err)
	}
	coordinator.runOnce(ctx)

	var state, admissionStatus, batchStatus, outboxStatus string
	var tenantMicro, userMicro int64
	if err := pool.QueryRow(ctx, `
		SELECT w.state, a.status, w.accrued_tenant_micro, w.accrued_user_micro
		FROM ai_billing_windows w
		JOIN ai_billing_request_admissions a ON a.window_id=w.window_id
		WHERE a.request_id=$1
	`, intent.RequestID).Scan(&state, &admissionStatus, &tenantMicro, &userMicro); err != nil {
		t.Fatalf("read completed window: %v", err)
	}
	if state != "settled" || admissionStatus != "completed" || tenantMicro != 700 || userMicro != 900 {
		t.Fatalf("unexpected completion state: state=%s admission=%s amount=(%d,%d)",
			state, admissionStatus, tenantMicro, userMicro)
	}
	if err := pool.QueryRow(ctx, `
		SELECT b.status, o.status
		FROM ai_billing_settlement_batches b
		JOIN ai_billing_settlement_outbox o ON o.batch_id=b.batch_id
		WHERE b.window_id=$1
	`, admission.WindowID).Scan(&batchStatus, &outboxStatus); err != nil {
		t.Fatalf("read settlement state: %v", err)
	}
	if batchStatus != "delivered" || outboxStatus != "delivered" || port.settleCalls != 1 {
		t.Fatalf("settlement = batch:%s outbox:%s calls:%d", batchStatus, outboxStatus, port.settleCalls)
	}
}

func TestRenewFailureDrainsOnlyAtHardBoundaryOrTerminalResponse(t *testing.T) {
	now := time.Now().UTC()
	safe := &window{ExpiresAt: timePtr(now.Add(2 * time.Minute))}
	if shouldDrainAfterRenewFailure(errors.New("temporary network failure"), safe, now, 30*time.Second) {
		t.Fatal("transient renewal failure drained a lease with safe headroom")
	}
	terminal := ErrAdmissionConflict
	if !shouldDrainAfterRenewFailure(terminal, safe, now, 30*time.Second) {
		t.Fatal("terminal renewal conflict did not drain")
	}
	unsafe := &window{ExpiresAt: timePtr(now.Add(20 * time.Second))}
	if !shouldDrainAfterRenewFailure(errors.New("temporary network failure"), unsafe, now, 30*time.Second) {
		t.Fatal("renewal failure inside admission headroom did not drain")
	}
}

func TestRenewRecoversACommittedResponseLoss(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	port := newMemoryLeasePort(func() time.Time { return now })
	coordinator := New(pool, port, Config{
		LeaseTTL: 10 * time.Minute, LeaseGrace: 15 * time.Minute,
		WindowMaxAge: 5 * time.Minute, AdmissionHeadroom: time.Second,
	}, nil)
	coordinator.now = func() time.Time { return now }
	admission, err := coordinator.Admit(ctx, Intent{
		RequestID: "request-renew-loss", OwnerType: "tenant",
		TenantID: "tenant-renew-loss", WantTenant: true, RequestTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	win, err := coordinator.readWindow(ctx, admission.WindowID)
	if err != nil {
		t.Fatalf("read window: %v", err)
	}
	port.failRenewOnce = true
	if err := coordinator.renewWindow(ctx, win); err != nil {
		t.Fatalf("recover committed renew response loss: %v", err)
	}
	current, err := coordinator.readWindow(ctx, admission.WindowID)
	if err != nil {
		t.Fatalf("read renewed window: %v", err)
	}
	if current.LeaseVersion != 2 || port.renewCalls != 1 {
		t.Fatalf("renewed version=%d calls=%d, want version 2 with one remote mutation",
			current.LeaseVersion, port.renewCalls)
	}
}

func TestExpiredAdmissionIsQuarantinedAndManuallyReconciled(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	port := newMemoryLeasePort(func() time.Time { return now })
	coordinator := New(pool, port, Config{
		LeaseTTL: 10 * time.Minute, LeaseGrace: 15 * time.Minute,
		WindowMaxAge: 5 * time.Minute, AdmissionHeadroom: time.Second,
	}, nil)
	coordinator.now = func() time.Time { return now }
	intent := Intent{
		RequestID: "request-reconcile", OwnerType: "user",
		TenantID: "tenant-reconcile", UserID: "user-reconcile",
		WantTenant: true, WantUser: true, RequestTTL: time.Minute,
	}
	admission, err := coordinator.Admit(ctx, intent)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	now = now.Add(61 * time.Second)
	if count, err := coordinator.quarantineExpiredAdmissions(ctx); err != nil || count != 1 {
		t.Fatalf("quarantine expired admission: count=%d err=%v", count, err)
	}
	items, err := coordinator.ListReconciliations(ctx)
	if err != nil || len(items) != 1 || items[0].RequestID != intent.RequestID {
		t.Fatalf("reconciliation list = %#v, %v", items, err)
	}
	if _, err := coordinator.Admit(ctx, intent); !errors.Is(err, ErrAdmissionConflict) {
		t.Fatalf("reconciling request re-admission error = %v", err)
	}
	if err := coordinator.ResolveReconciliation(ctx, Completion{
		RequestID: intent.RequestID, TenantMicro: 11, UserMicro: 13,
		Note: "operator verified upstream usage record",
	}); err != nil {
		t.Fatalf("resolve reconciliation: %v", err)
	}
	coordinator.runOnce(ctx)

	var state, status, source, note string
	var tenantMicro, userMicro int64
	if err := pool.QueryRow(ctx, `
		SELECT w.state, a.status, a.completion_source, a.completion_note,
		       a.actual_tenant_micro, a.actual_user_micro
		FROM ai_billing_windows w
		JOIN ai_billing_request_admissions a ON a.window_id=w.window_id
		WHERE a.request_id=$1
	`, intent.RequestID).Scan(&state, &status, &source, &note, &tenantMicro, &userMicro); err != nil {
		t.Fatalf("read reconciled admission: %v", err)
	}
	if state != "settled" || status != "completed" || source != "manual" ||
		note == "" || tenantMicro != 11 || userMicro != 13 {
		t.Fatalf("reconciled state=%s status=%s source=%s note=%q amounts=(%d,%d), admission=%s",
			state, status, source, note, tenantMicro, userMicro, admission.WindowID)
	}
}

func TestCoordinatorRecoversLostAcquireAndSettlementResponses(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	port := newMemoryLeasePort(func() time.Time { return now })
	port.failAcquireOnce = true
	port.failSettleOnce = true
	coordinator := New(pool, port, Config{
		LeaseTTL: 10 * time.Minute, LeaseGrace: 15 * time.Minute,
		WindowMaxAge: 5 * time.Minute, AdmissionHeadroom: time.Second,
	}, nil)
	coordinator.now = func() time.Time { return now }
	intent := Intent{
		RequestID: "request-lost-response", OwnerType: "user",
		TenantID: "tenant-lost-response", UserID: "user-lost-response",
		WantTenant: true, WantUser: true, RequestTTL: time.Minute,
	}
	if _, err := coordinator.Admit(ctx, intent); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("first lost acquire error = %v", err)
	}
	admission, err := coordinator.Admit(ctx, intent)
	if err != nil {
		t.Fatalf("idempotent acquire recovery: %v", err)
	}
	if port.acquireCalls != 2 || len(port.byID) != 1 {
		t.Fatalf("acquire calls=%d leases=%d, want two calls and one lease", port.acquireCalls, len(port.byID))
	}
	completeBillingRequest(t, ctx, coordinator, intent.RequestID, 50, 75)
	if err := coordinator.beginDrain(ctx, admission.WindowID, "", ""); err != nil {
		t.Fatalf("begin drain: %v", err)
	}
	coordinator.runOnce(ctx)
	var outboxStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM ai_billing_settlement_outbox WHERE lease_id=$1
	`, admission.LeaseID).Scan(&outboxStatus); err != nil {
		t.Fatalf("read retry outbox: %v", err)
	}
	if outboxStatus != "pending" {
		t.Fatalf("outbox after lost response = %s, want pending", outboxStatus)
	}
	now = now.Add(3 * time.Second)
	coordinator.runOnce(ctx)
	if err := pool.QueryRow(ctx, `
		SELECT status FROM ai_billing_settlement_outbox WHERE lease_id=$1
	`, admission.LeaseID).Scan(&outboxStatus); err != nil {
		t.Fatalf("read delivered outbox: %v", err)
	}
	if outboxStatus != "delivered" || port.settleCalls != 2 {
		t.Fatalf("recovered settlement = status:%s calls:%d", outboxStatus, port.settleCalls)
	}
}

func TestOpeningWindowRecoversRemoteGraceAndReleasedStates(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	port := newMemoryLeasePort(func() time.Time { return now })
	coordinator := New(pool, port, Config{
		LeaseTTL: 10 * time.Minute, LeaseGrace: 15 * time.Minute,
		WindowMaxAge: 5 * time.Minute, AdmissionHeadroom: time.Second,
	}, nil)
	coordinator.now = func() time.Time { return now }

	port.failAcquireOnce = true
	graceIntent := Intent{
		RequestID: "request-opening-grace", OwnerType: "tenant",
		TenantID: "tenant-opening-grace", WantTenant: true, RequestTTL: time.Minute,
	}
	if _, err := coordinator.Admit(ctx, graceIntent); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("lost grace acquire response error = %v", err)
	}
	var graceRemote *CreditLease
	for _, lease := range port.byWindow {
		if lease.TenantID == graceIntent.TenantID {
			graceRemote = lease
			break
		}
	}
	if graceRemote == nil {
		t.Fatal("missing remote grace fixture")
	}
	graceRemote.EscrowState = "grace"
	graceRemote.Version++
	graceRemote.ExpiresAt = now.Add(-time.Second)
	graceRemote.GraceUntil = now.Add(time.Minute)
	graceAdmission, err := coordinator.Admit(ctx, graceIntent)
	if err != nil {
		t.Fatalf("recover opening window from grace: %v", err)
	}
	if graceAdmission.LeaseID != graceRemote.LeaseID || graceRemote.EscrowState != "active" {
		t.Fatalf("grace recovery admission=%#v remote=%#v", graceAdmission, graceRemote)
	}

	port.failAcquireOnce = true
	releasedIntent := Intent{
		RequestID: "request-opening-released", OwnerType: "tenant",
		TenantID: "tenant-opening-released", WantTenant: true, RequestTTL: time.Minute,
	}
	if _, err := coordinator.Admit(ctx, releasedIntent); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("lost released acquire response error = %v", err)
	}
	var releasedRemote *CreditLease
	for _, lease := range port.byWindow {
		if lease.TenantID == releasedIntent.TenantID {
			releasedRemote = lease
			break
		}
	}
	if releasedRemote == nil {
		t.Fatal("missing remote released fixture")
	}
	releasedLeaseID := releasedRemote.LeaseID
	releasedRemote.EscrowState = "released"
	releasedRemote.Version++
	releasedRemote.ExpiresAt = now.Add(-2 * time.Minute)
	releasedRemote.GraceUntil = now.Add(-time.Minute)
	replacement, err := coordinator.Admit(ctx, releasedIntent)
	if err != nil {
		t.Fatalf("recover opening window from released escrow: %v", err)
	}
	if replacement.LeaseID == releasedLeaseID {
		t.Fatalf("released lease was reused for admission: %#v", replacement)
	}
	coordinator.runOnce(ctx)
	if releasedRemote.SettlementState != "settled" ||
		releasedRemote.ActualTenantMicro == nil || *releasedRemote.ActualTenantMicro != 0 {
		t.Fatalf("released orphan lease was not settled at zero: %#v", releasedRemote)
	}
}

func TestOutboxAttemptFenceRejectsStaleWorkerAndInvalidReceipt(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	port := newMemoryLeasePort(func() time.Time { return now })
	coordinator := New(pool, port, Config{
		LeaseTTL: 10 * time.Minute, LeaseGrace: 15 * time.Minute,
		WindowMaxAge: 5 * time.Minute, AdmissionHeadroom: time.Second,
		DispatchLease: 10 * time.Second,
	}, nil)
	coordinator.now = func() time.Time { return now }
	admission, err := coordinator.Admit(ctx, Intent{
		RequestID: "request-outbox-fence", OwnerType: "tenant",
		TenantID: "tenant-outbox-fence", WantTenant: true, RequestTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	completeBillingRequest(t, ctx, coordinator, "request-outbox-fence", 37, 0)
	if err := coordinator.beginDrain(ctx, admission.WindowID, "", ""); err != nil {
		t.Fatalf("begin drain: %v", err)
	}
	if err := coordinator.createSettlementBatches(ctx); err != nil {
		t.Fatalf("create settlement batch: %v", err)
	}
	stale, err := coordinator.claimOutbox(ctx)
	if err != nil {
		t.Fatalf("claim stale attempt: %v", err)
	}
	now = now.Add(11 * time.Second)
	current, err := coordinator.claimOutbox(ctx)
	if err != nil {
		t.Fatalf("reclaim expired attempt: %v", err)
	}
	if stale.AttemptCount != 1 || current.AttemptCount != 2 {
		t.Fatalf("attempts stale=%d current=%d, want 1/2", stale.AttemptCount, current.AttemptCount)
	}
	if err := coordinator.retryOutbox(ctx, stale, errors.New("stale worker failure")); !errors.Is(err, errOutboxClaimLost) {
		t.Fatalf("stale retry error = %v, want claim lost", err)
	}
	var status string
	var attemptCount int
	if err := pool.QueryRow(ctx, `
		SELECT status, attempt_count FROM ai_billing_settlement_outbox WHERE outbox_id=$1::uuid
	`, current.OutboxID).Scan(&status, &attemptCount); err != nil {
		t.Fatalf("read current outbox claim: %v", err)
	}
	if status != "processing" || attemptCount != 2 {
		t.Fatalf("outbox after stale retry = status:%s attempt:%d", status, attemptCount)
	}

	actualTenant := current.ActualTenantMicro
	actualUser := current.ActualUserMicro
	settledAt := now
	receipt := &CreditLease{
		LeaseID: current.LeaseID, EscrowState: "released", SettlementState: "settled",
		Version: 2, SettlementID: current.SettlementID,
		ActualTenantMicro: &actualTenant, ActualUserMicro: &actualUser,
		TenantDeductedMicro: actualTenant, UserDeductedMicro: actualUser,
		SettledEventID: "EV_outbox-fence", SettledAt: &settledAt,
	}
	invalid := cloneLease(receipt)
	invalid.ActualTenantMicro = int64TestPtr(actualTenant + 1)
	if err := coordinator.deliverOutbox(ctx, current, invalid); !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("invalid receipt error = %v, want protocol violation", err)
	}
	if err := coordinator.deliverOutbox(ctx, current, receipt); err != nil {
		t.Fatalf("deliver current claim: %v", err)
	}
	if err := coordinator.retryOutbox(ctx, stale, errors.New("late stale failure")); !errors.Is(err, errOutboxClaimLost) {
		t.Fatalf("late stale retry error = %v, want claim lost", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT status FROM ai_billing_settlement_outbox WHERE outbox_id=$1::uuid
	`, current.OutboxID).Scan(&status); err != nil {
		t.Fatalf("read delivered outbox: %v", err)
	}
	if status != "delivered" {
		t.Fatalf("stale worker changed delivered outbox to %q", status)
	}
}

func completeBillingRequest(t *testing.T, ctx context.Context, coordinator *Coordinator, requestID string, tenantMicro, userMicro int64) {
	t.Helper()
	tx, err := coordinator.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin completion: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := coordinator.Complete(ctx, tx, Completion{
		RequestID: requestID, TenantMicro: tenantMicro, UserMicro: userMicro,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit completion: %v", err)
	}
}

func cloneLease(src *CreditLease) *CreditLease {
	if src == nil {
		return nil
	}
	dst := *src
	if src.ActualTenantMicro != nil {
		dst.ActualTenantMicro = int64TestPtr(*src.ActualTenantMicro)
	}
	if src.ActualUserMicro != nil {
		dst.ActualUserMicro = int64TestPtr(*src.ActualUserMicro)
	}
	return &dst
}

func int64TestPtr(v int64) *int64    { return &v }
func timePtr(v time.Time) *time.Time { return &v }
