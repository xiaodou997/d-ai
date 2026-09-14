package postgres

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
)

func TestRequestStoreAtomicSettlementAndIndependentRefund(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	seedDirectBillingAccounts(t, ctx, pool, "record-tenant", "record-user", "records")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "record-tenant", TenantID: "record-tenant"}, 10000)
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: "record-user", TenantID: "record-tenant"}, 10000)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 100, UserPayableMicro: 200, UserChargedMicro: 200, APIKeyQuotaCostMicro: 200, CatalogBaseMicro: 50}}, nil)
	req := usageCompletionRequest("record-request", "record-tenant", "record-user")
	req.UsageEvidence = domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10, "output_tokens": 2}}
	if err = store.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := accountBalance(t, ctx, pool, "record-user"); got != 10000 {
		t.Fatalf("relay path charged %d", got)
	}
	if _, err = store.DrainSettlements(ctx, 10); err != nil {
		t.Fatal(err)
	}
	req.ClientDeliveryState = domain.ClientDeliveryDisconnected
	req.CancellationOrigin = domain.CancellationClient
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DrainSettlements(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE bill_settlements SET user_due=user_due+1 WHERE request_id=$1`, req.RequestID); err == nil {
		t.Fatal("sealed price evidence was mutable")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM bill_journal WHERE operation_id=$1`, req.RequestID); err == nil {
		t.Fatal("financial journal could be erased")
	}
	if got := accountBalance(t, ctx, pool, "record-user"); got != 9800 {
		t.Fatalf("charge = %d", got)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM ai_requests WHERE request_id=$1`, req.RequestID); err != nil {
		t.Fatal(err)
	}
	if err = store.Refund(ctx, req.RequestID, "test correction", "admin"); err != nil {
		t.Fatal(err)
	}
	if err = store.Refund(ctx, req.RequestID, "duplicate", "admin"); err != nil {
		t.Fatal(err)
	}
	if got := accountBalance(t, ctx, pool, "record-user"); got != 10000 {
		t.Fatalf("refund = %d", got)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM bill_journal WHERE operation_id IN($1,$2)`, req.RequestID, "refund:"+req.RequestID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("journal entries=%d", count)
	}
	summary, e := store.RecordSummary(ctx, domain.RecordQuery{})
	if e != nil || summary.Interruptions != 1 {
		t.Fatalf("late delivery fact lost or double-counted: %+v %v", summary, e)
	}
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestRequestStoreWaivesBothLayersOnExplicitError(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 100, UserPayableMicro: 200, UserChargedMicro: 200, APIKeyQuotaCostMicro: 300}}, nil)
	req := usageCompletionRequest("record-error", "tenant", "user")
	req.ProviderTerminalState = domain.ProviderTerminalFailed
	req.ErrorCode = "provider_terminal_error"
	req.UsageEvidence = domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	var state string
	var tenant, user, key, sub int64
	if err = pool.QueryRow(ctx, `SELECT state,tenant_due,user_due,key_due,subscription_due FROM bill_settlements WHERE request_id=$1`, req.RequestID).Scan(&state, &tenant, &user, &key, &sub); err != nil {
		t.Fatal(err)
	}
	if state != "waived" || tenant+user+key+sub != 0 {
		t.Fatalf("state=%s meters=%d", state, tenant+user+key+sub)
	}
}

func TestRequestStoreRejectsDuplicateExecutionAndForeignOwner(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	store := NewRequestStore(pool, nil, nil)
	first := usageCompletionRequest("stable-task", "tenant-a", "user-a")
	if err = store.Execute(ctx, first); err != nil {
		t.Fatal(err)
	}
	defer first.RecordLeaseStop()
	duplicate := usageCompletionRequest("stable-task", "tenant-a", "user-a")
	if err = store.Execute(ctx, duplicate); err == nil {
		t.Fatal("duplicate execution admitted")
	}
	if err = store.Log(ctx, duplicate); err != nil {
		t.Fatal(err)
	}
	var sealed bool
	if err = pool.QueryRow(ctx, `SELECT sealed_at IS NOT NULL FROM ai_request_keys WHERE request_id='stable-task'`).Scan(&sealed); err != nil || sealed {
		t.Fatalf("rejected execution sealed original: %v %v", sealed, err)
	}
	foreign := usageCompletionRequest("stable-task", "tenant-b", "user-b")
	if err = store.Log(ctx, foreign); err == nil {
		t.Fatal("foreign owner reused financial identity")
	}
	if err = store.Log(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err = store.Execute(ctx, usageCompletionRequest("stable-task", "tenant-a", "user-a")); err == nil {
		t.Fatal("sealed execution admitted")
	}
}

func TestRequestStoreRollbackThenConcurrentSettlement(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	seedDirectBillingAccounts(t, ctx, pool, "atomic-tenant", "atomic-user", "atomic")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "atomic-tenant", TenantID: "atomic-tenant"}, 1000)
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: "atomic-user", TenantID: "atomic-tenant"}, 1000)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 100, UserChargedMicro: 200}}, nil)
	req := usageCompletionRequest("atomic-request", "atomic-tenant", "atomic-user")
	req.UsageEvidence = domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	// Fail after the tenant debit, forcing the entire transaction to roll back.
	mustExecUsageRepo(t, ctx, pool, `CREATE FUNCTION reject_user_charge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.account_id='atomic-user' AND NEW.balance_micro<OLD.balance_micro THEN RAISE EXCEPTION 'injected ledger failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_user_charge BEFORE UPDATE ON bill_accounts FOR EACH ROW EXECUTE FUNCTION reject_user_charge()`)
	if _, err = store.DrainSettlements(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if accountBalance(t, ctx, pool, "atomic-tenant") != 1000 || accountBalance(t, ctx, pool, "atomic-user") != 1000 {
		t.Fatal("partial debit survived rollback")
	}
	mustExecUsageRepo(t, ctx, pool, `DROP TRIGGER reject_user_charge ON bill_accounts; UPDATE bill_settlements SET available_at=now()`)
	done := make(chan error, 8)
	for range 8 {
		go func() { _, err := store.DrainSettlements(ctx, 1); done <- err }()
	}
	for range 8 {
		if err = <-done; err != nil {
			t.Fatal(err)
		}
	}
	if accountBalance(t, ctx, pool, "atomic-tenant") != 900 || accountBalance(t, ctx, pool, "atomic-user") != 800 {
		t.Fatal("concurrent workers posted more than once")
	}
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRequestStoreRefundPreservesSubscriptionWindowIdentity(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	seedDirectBillingAccounts(t, ctx, pool, "quota-tenant", "quota-user", "quota")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "quota-tenant", TenantID: "quota-tenant"}, 1000)
	group, key, sub := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_groups(id,tenant_id,name,retail_price_book_id) VALUES($1::uuid,'quota-tenant','quota',gen_random_uuid())`, group)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_api_keys(id,owner_type,tenant_id,user_id,group_id,key_hash,key_ciphertext,name) VALUES($1::uuid,'user','quota-tenant','quota-user',$2::uuid,$1,'test','quota')`, key, group)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_sub_subscriptions(id,tenant_id,user_id,plan_id,order_id,plan_name_snapshot,duration_days,total_limit_micro,window_5h_limit_micro,window_7d_limit_micro) VALUES($1::uuid,'quota-tenant','quota-user',gen_random_uuid(),gen_random_uuid(),'quota',30,10000,1000,5000)`, sub)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 5, RetailBaseMicro: 100, APIKeyQuotaCostMicro: 10}}, nil)
	req := usageCompletionRequest("quota-request", "quota-tenant", "quota-user")
	req.Subject.APIKeyID = key
	req.BillingSource = "subscription"
	req.SubscriptionID = sub
	req.Candidate.GroupID = group
	req.SubscriptionGroupQuotaDebitMultipliers = map[string]float64{group: 2}
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DrainSettlements(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var total, five, seven int64
	if err = pool.QueryRow(ctx, `SELECT total_used_micro,win5h_used_micro,win7d_used_micro FROM ai_sub_subscriptions WHERE id=$1::uuid`, sub).Scan(&total, &five, &seven); err != nil || total != 200 || five != 200 || seven != 200 {
		t.Fatalf("subscription debit %d/%d/%d %v", total, five, seven, err)
	}
	// The five-hour window rolled; unrelated new usage must survive this refund.
	mustExecUsageRepo(t, ctx, pool, `UPDATE ai_sub_subscriptions SET win5h_start=now()+interval '1 second',win5h_used_micro=25 WHERE id=$1::uuid`, sub)
	if err = store.Refund(ctx, req.RequestID, "quota refund", "admin"); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT total_used_micro,win5h_used_micro,win7d_used_micro FROM ai_sub_subscriptions WHERE id=$1::uuid`, sub).Scan(&total, &five, &seven); err != nil || total != 0 || five != 25 || seven != 0 {
		t.Fatalf("refund changed unrelated window: %d/%d/%d %v", total, five, seven, err)
	}
	var quota int64
	if err = pool.QueryRow(ctx, `SELECT quota_used FROM ai_api_keys WHERE id=$1::uuid`, key).Scan(&quota); err != nil || quota != 0 {
		t.Fatalf("key refund=%d %v", quota, err)
	}
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRequestStoreDatabaseDisconnectAndUnknownCommitOutcome(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	seedDirectBillingAccounts(t, ctx, pool, "disconnect-tenant", "disconnect-user", "db-disconnect")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "disconnect-tenant", TenantID: "disconnect-tenant"}, 1000)
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: "disconnect-user", TenantID: "disconnect-tenant"}, 1000)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 100, UserChargedMicro: 200}}, nil)
	for _, id := range []string{"transaction-disconnected", "commit-ack-lost"} {
		if err = store.Log(ctx, usageCompletionRequest(id, "disconnect-tenant", "disconnect-user")); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var pid int
		if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		w, err := scanSettlement(tx.QueryRow(ctx, `SELECT `+settlementWorkColumns+` FROM bill_settlements WHERE request_id=$1 FOR UPDATE`, id))
		if err != nil {
			t.Fatal(err)
		}
		if err = store.postSettlement(ctx, tx, w); err != nil {
			t.Fatal(err)
		}
		if id == "transaction-disconnected" {
			if _, err = pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err == nil {
				t.Fatal("terminated transaction unexpectedly committed")
			}
			if accountBalance(t, ctx, pool, "disconnect-user") != 1000 {
				t.Fatal("disconnect left a partial charge")
			}
		} else {
			// Commit happened but no acknowledgement is available to the replacement worker.
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		replacement := NewRequestStore(pool, nil, nil)
		if _, err = replacement.DrainSettlements(ctx, 10); err != nil {
			t.Fatal(err)
		}
	}
	if accountBalance(t, ctx, pool, "disconnect-user") != 600 || accountBalance(t, ctx, pool, "disconnect-tenant") != 800 {
		t.Fatal("restart or uncertain commit duplicated a debit")
	}
}
