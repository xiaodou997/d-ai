package postgres

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
)

func TestRequestLifecycleRecoveryRetentionAndReplayFence(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	store := NewRequestStore(pool, nil, nil)
	req := usageCompletionRequest("lost-runtime", "t", "u")
	if err = store.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.RecordLeaseStop()
	mustExecUsageRepo(t, ctx, pool, `UPDATE ai_request_keys SET lease_until=now()-interval '1 second' WHERE request_id='lost-runtime'`)
	if err = store.RecoverRequests(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Record(ctx, domain.RecordScope{Admin: true}, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Charge.State != "waived" || recovered.Error == nil || recovered.Error.Code != "runtime_lost" {
		t.Fatalf("unsafe crash recovery: %+v", recovered)
	}
	// A late completion cannot replace a recovery decision with an automatic debit.
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -430)
	mustExecUsageRepo(t, ctx, pool, `SELECT ensure_request_record_partitions($1)`, old)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_request_keys(request_id,registered_at,epoch,lease_until,sealed_at,tenant_id,user_id) SELECT 'old-history',$1,epoch,$1,$1,'t','u' FROM bill_record_control WHERE singleton`, old)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO bill_settlements(created_at,request_id,tenant_id,user_id,state,reason) VALUES($1,'old-history','t','u','pending','reported_usage')`, old)
	if err = store.MaintainRecords(ctx); err == nil {
		t.Fatal("cleanup crossed pending settlement")
	}
	mustExecUsageRepo(t, ctx, pool, `UPDATE bill_settlements SET state='waived' WHERE request_id='old-history'`)
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM bill_settlements WHERE request_id='old-history'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old partition retained: %d %v", count, err)
	}
	replay := usageCompletionRequest("old-history", "t", "u")
	replay.StartedAt = time.Now() // Gateway ingress is new; task identity remains old.
	replay.BillingOriginAt = old
	if err = store.Execute(ctx, replay); err == nil {
		t.Fatal("historical replay was admitted after its identity expired")
	}
	// The finalizer must not recreate an identity rejected at admission.
	if err = store.Log(ctx, replay); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM ai_request_keys WHERE request_id='old-history'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected replay recreated financial identity: count=%d err=%v", count, err)
	}
	replay.RecordRegistrationRejected = false
	if err = store.Log(ctx, replay); err == nil {
		t.Fatal("historical replay finalizer reopened a closed financial period")
	}
}

func TestRequestSettlementTenantContentionLoad(t *testing.T) {
	if os.Getenv("DAI_RUN_LEDGER_LOAD") != "1" {
		t.Skip("explicit ledger load run")
	}
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	seedDirectBillingAccounts(t, ctx, pool, "load-tenant", "load-user", "load")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "load-tenant", TenantID: "load-tenant"}, 1)
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: "load-user", TenantID: "load-tenant"}, 1)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 100, UserChargedMicro: 200}}, nil)
	const count = 128
	started := time.Now()
	durations := make(chan time.Duration, count)
	errs := make(chan error, count)
	gate := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			at := time.Now()
			err := store.Log(ctx, usageCompletionRequest(fmt.Sprintf("load-%03d", i), "load-tenant", "load-user"))
			durations <- time.Since(at)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(durations)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sealed := time.Since(started)
	if accountBalance(t, ctx, pool, "load-user") != 1 {
		t.Fatal("sealing touched account balance")
	}
	completed := make(chan error, 16)
	started = time.Now()
	for range 16 {
		go func() { _, e := store.DrainSettlements(ctx, count); completed <- e }()
	}
	for range 16 {
		if err = <-completed; err != nil {
			t.Fatal(err)
		}
	}
	if accountBalance(t, ctx, pool, "load-user") != 1-count*200 || accountBalance(t, ctx, pool, "load-tenant") != 1-count*100 {
		t.Fatal("concurrent charge conservation failed")
	}
	sorted := make([]time.Duration, 0, count)
	for d := range durations {
		sorted = append(sorted, d)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	t.Logf("%d same-tenant settlements: seal total=%s p95=%s; 16-worker post total=%s; balances conserved including debt", count, sealed, sorted[count*95/100], time.Since(started))
}

func TestRequestFinancialRollforwardPreservesConservation(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	old := time.Now().UTC().AddDate(0, 0, -430)
	mustExecUsageRepo(t, ctx, pool, `SELECT ensure_request_record_partitions($1)`, old)
	// A historical account fixture: opening 500, past movement +500, current 1000.
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO bill_accounts(account_id,account_kind,tenant_id,balance_micro) VALUES('rollforward',1,'rollforward',1000); DELETE FROM bill_checkpoints WHERE account_id='rollforward'`)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source) VALUES('rollforward','balance',$1,501,'historical_opening')`, old.AddDate(0, 0, -2))
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO bill_journal(created_at,operation_id,account_id,meter,delta,before_value,after_value,reason) VALUES($1,'historical-credit','rollforward','balance',500,500,1000,'recharge')`, old)
	store := NewRequestStore(pool, nil, nil)
	if err = store.MaintainRecords(ctx); err == nil {
		t.Fatal("unverified opening discrepancy was silently discarded")
	}
	mustExecUsageRepo(t, ctx, pool, `UPDATE bill_checkpoints SET value=500 WHERE account_id='rollforward'`)
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal(err)
	}
	var value int64
	if err = pool.QueryRow(ctx, `SELECT value FROM bill_checkpoints WHERE account_id='rollforward' ORDER BY through_at DESC LIMIT 1`).Scan(&value); err != nil || value != 1000 {
		t.Fatalf("rollforward value=%d %v", value, err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM bill_journal WHERE operation_id='historical-credit'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("historical journal not retired: %d %v", count, err)
	}
	mustExecUsageRepo(t, ctx, pool, `UPDATE bill_accounts SET balance_micro=1001 WHERE account_id='rollforward'`)
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal("post-checkpoint movement broke conservation", err)
	}
}
