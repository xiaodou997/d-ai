package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/billing"
	shared "xiaodou/dai/internal/domain"
)

// TestConfirmAllowOverdraftOverflow 验证 flex Confirm 可以超过原冻结额确认，
// 不足部分进入 overdraft，且响应显式标记 exhausted。
func TestConfirmAllowOverdraftOverflow(t *testing.T) {
	dsn := os.Getenv("DAI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DAI_TEST_DATABASE_URL to run this DB-backed test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("cannot ping db: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "t_flex_" + suffix
	userID := "u_flex_" + suffix
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nsql=%s", err, sql)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM bill_events WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_packages WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id=$1`, tenantID)
	})

	mustExec(`INSERT INTO iam_tenants (tenant_id, tenant_name, status, overdraft_limit, current_overdraft, frozen_credits)
	          VALUES ($1, 'flex-test', 'active', 20, 0, 0)`, tenantID)
	mustExec(`INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, overdraft_limit, current_overdraft, frozen_credits)
	          VALUES ($1, $2, $3, 'x', 4, 20, 0, 0)`, userID, tenantID, "u_user-"+suffix)
	mustExec(`INSERT INTO bill_credit_packages (package_id, package_type, tenant_id, user_id, total_credits, remaining_credits, source, status)
	          VALUES ($1, 'user', $2, $3, 10, 10, 'ADMIN_RECHARGE', 'available')`, "pkg-"+suffix, tenantID, userID)

	svc := NewDeductionService(pool, nil, zap.NewNop())
	freeze, err := svc.Freeze(FreezeParams{
		IdempotencyKey: "freeze-" + suffix,
		TenantID:       tenantID,
		UserID:         userID,
		UserAmount:     5,
		AllowOverdraft: true,
	})
	if err != nil {
		t.Fatalf("freeze should succeed, got %v", err)
	}
	protected, err := svc.Freeze(FreezeParams{
		IdempotencyKey: "protected-freeze-" + suffix,
		TenantID:       tenantID,
		UserID:         userID,
		UserAmount:     2,
		AllowOverdraft: true,
	})
	if err != nil {
		t.Fatalf("protected freeze should succeed, got %v", err)
	}
	res, err := svc.Confirm(ConfirmParams{
		EventID:          freeze.EventID,
		ActualUserAmount: 33,
		AllowOverdraft:   true,
	})
	if err != nil {
		t.Fatalf("confirm should succeed, got %v", err)
	}
	if res.UserDeducted != 8 || res.UserOverdraftAdd != 25 {
		t.Fatalf("unexpected confirm result: %+v", res)
	}
	if res.AccountState != accountStateExhausted || res.AllowFurtherUsage {
		t.Fatalf("expected exhausted=false/blocked result, got %+v", res)
	}
	assertInt64(t, pool, ctx, `SELECT current_overdraft FROM iam_accounts WHERE user_id=$1`, userID, 25)
	assertInt64(t, pool, ctx, `SELECT remaining_credits FROM bill_credit_packages WHERE user_id=$1`, userID, 2)
	assertInt64(t, pool, ctx, `SELECT frozen_credits FROM iam_accounts WHERE user_id=$1`, userID, 2)
	if _, err := svc.Freeze(FreezeParams{
		IdempotencyKey: "blocked-" + suffix,
		TenantID:       tenantID,
		UserID:         userID,
		UserAmount:     1,
		AllowOverdraft: true,
	}); !errors.Is(err, shared.ErrUserInOverdraft) {
		t.Fatalf("new authorization with debt error = %v, want ErrUserInOverdraft", err)
	}
	if _, err := svc.Cancel(CancelParams{EventID: protected.EventID}); err != nil {
		t.Fatalf("cancel protected authorization: %v", err)
	}
	assertInt64(t, pool, ctx, `SELECT frozen_credits FROM iam_accounts WHERE user_id=$1`, userID, 0)
}

// TestFreezeAllowOverdraftSoftReserve 验证 flex Freeze 在未 exhausted 时允许 soft reservation。
func TestFreezeAllowOverdraftSoftReserve(t *testing.T) {
	dsn := os.Getenv("DAI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DAI_TEST_DATABASE_URL to run this DB-backed test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("cannot ping db: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "t_soft_" + suffix
	userID := "u_soft_" + suffix
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nsql=%s", err, sql)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM bill_events WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_packages WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id=$1`, tenantID)
	})

	mustExec(`INSERT INTO iam_tenants (tenant_id, tenant_name, status, overdraft_limit, current_overdraft, frozen_credits)
	          VALUES ($1, 'soft-test', 'active', 20, 0, 0)`, tenantID)
	mustExec(`INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, overdraft_limit, current_overdraft, frozen_credits)
	          VALUES ($1, $2, $3, 'x', 4, 20, 0, 0)`, userID, tenantID, "u_user-"+suffix)
	mustExec(`INSERT INTO bill_credit_packages (package_id, package_type, tenant_id, user_id, total_credits, remaining_credits, source, status)
	          VALUES ($1, 'user', $2, $3, 3, 3, 'ADMIN_RECHARGE', 'available')`, "pkg-"+suffix, tenantID, userID)

	svc := NewDeductionService(pool, nil, zap.NewNop())
	res, err := svc.Freeze(FreezeParams{
		IdempotencyKey: "soft-freeze-" + suffix,
		TenantID:       tenantID,
		UserID:         userID,
		UserAmount:     5,
		AllowOverdraft: true,
	})
	if err != nil {
		t.Fatalf("soft freeze should succeed, got %v", err)
	}
	if res.EventID == "" || res.FrozenUser != 3 {
		t.Fatalf("expected partial grant of 3, got %+v", res)
	}
	assertInt64(t, pool, ctx, `SELECT frozen_credits FROM iam_accounts WHERE user_id=$1`, userID, 3)
}

func TestAuthorizationCanOnlyBeSettledByOwningClient(t *testing.T) {
	dsn := os.Getenv("DAI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DAI_TEST_DATABASE_URL to run this DB-backed test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("cannot ping db: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "t_owner_" + suffix
	userID := "u_owner_" + suffix
	ownerClientID := "dai-test-" + suffix
	otherClientID := "other-client-" + suffix
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nsql=%s", err, sql)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM bill_events WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_packages WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id=$1`, tenantID)
	})

	mustExec(`INSERT INTO iam_tenants (tenant_id, tenant_name, status, current_overdraft, frozen_credits)
	          VALUES ($1, 'owner-test', 'active', 0, 0)`, tenantID)
	mustExec(`INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, current_overdraft, frozen_credits)
	          VALUES ($1, $2, $3, 'x', 4, 0, 0)`, userID, tenantID, "u_user-"+suffix)
	mustExec(`INSERT INTO bill_credit_packages (package_id, package_type, tenant_id, user_id, total_credits, remaining_credits, source, status)
	          VALUES ($1, 'user', $2, $3, 10, 10, 'ADMIN_RECHARGE', 'available')`, "pkg-"+suffix, tenantID, userID)
	svc := NewDeductionService(pool, nil, zap.NewNop())
	freeze, err := svc.Freeze(FreezeParams{
		IdempotencyKey: "owner-freeze-" + suffix,
		ClientID:       ownerClientID,
		TenantID:       tenantID,
		UserID:         userID,
		UserAmount:     5,
	})
	if err != nil {
		t.Fatalf("freeze should succeed: %v", err)
	}

	if _, err := svc.Confirm(ConfirmParams{
		EventID: freeze.EventID, ClientID: otherClientID, ActualUserAmount: 5,
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("cross-client confirm error = %v, want ErrForbidden", err)
	}
	if _, err := svc.Cancel(CancelParams{
		EventID: freeze.EventID, ClientID: otherClientID,
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("cross-client cancel error = %v, want ErrForbidden", err)
	}
	assertInt64(t, pool, ctx, `SELECT frozen_credits FROM iam_accounts WHERE user_id=$1`, userID, 5)

	if _, err := svc.Cancel(CancelParams{EventID: freeze.EventID, ClientID: ownerClientID}); err != nil {
		t.Fatalf("owning client cancel should succeed: %v", err)
	}
	assertInt64(t, pool, ctx, `SELECT frozen_credits FROM iam_accounts WHERE user_id=$1`, userID, 0)
}

func TestConfirmAndCancelSerializeOnAuthorization(t *testing.T) {
	dsn := os.Getenv("DAI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DAI_TEST_DATABASE_URL to run this DB-backed test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("cannot ping db: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "t_settle_race_" + suffix
	userID := "u_settle_race_" + suffix
	ownerClientID := "dai-test-" + suffix
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nsql=%s", err, sql)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM bill_events WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_packages WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id=$1`, tenantID)
	})

	mustExec(`INSERT INTO iam_tenants (tenant_id, tenant_name, status, current_overdraft, frozen_credits)
	          VALUES ($1, 'settlement-race-test', 'active', 0, 0)`, tenantID)
	mustExec(`INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, current_overdraft, frozen_credits)
	          VALUES ($1, $2, $3, 'x', 4, 0, 0)`, userID, tenantID, "u_user-"+suffix)
	mustExec(`INSERT INTO bill_credit_packages (package_id, package_type, tenant_id, user_id, total_credits, remaining_credits, source, status)
	          VALUES ($1, 'user', $2, $3, 100, 100, 'ADMIN_RECHARGE', 'available')`, "pkg-"+suffix, tenantID, userID)
	svc := NewDeductionService(pool, nil, zap.NewNop())
	freeze, err := svc.Freeze(FreezeParams{
		IdempotencyKey: "settlement-race-freeze-" + suffix,
		ClientID:       ownerClientID,
		TenantID:       tenantID,
		UserID:         userID,
		UserAmount:     5,
	})
	if err != nil {
		t.Fatalf("freeze should succeed: %v", err)
	}

	// Hold the account row so both settlement paths reach their locking point.
	// The bill_events row lock must then allow exactly one terminal transition.
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `SELECT user_id FROM iam_accounts WHERE user_id=$1 FOR UPDATE`, userID); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("lock user account: %v", err)
	}

	type settlementResult struct {
		operation string
		err       error
	}
	start := make(chan struct{})
	results := make(chan settlementResult, 2)
	go func() {
		<-start
		_, err := svc.Confirm(ConfirmParams{
			EventID: freeze.EventID, ClientID: ownerClientID, ActualUserAmount: 5,
		})
		results <- settlementResult{operation: "confirm", err: err}
	}()
	go func() {
		<-start
		_, err := svc.Cancel(CancelParams{EventID: freeze.EventID, ClientID: ownerClientID})
		results <- settlementResult{operation: "cancel", err: err}
	}()
	close(start)
	time.Sleep(100 * time.Millisecond)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release account lock: %v", err)
	}

	successes := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("confirm/cancel successes = %d, want exactly one", successes)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bill_events WHERE event_id=$1`, freeze.EventID).Scan(&status); err != nil {
		t.Fatalf("query event status: %v", err)
	}
	switch status {
	case billing.EventStatusSucceeded:
		assertInt64(t, pool, ctx, `SELECT remaining_credits FROM bill_credit_packages WHERE tenant_id=$1`, tenantID, 95)
	case billing.EventStatusCancelled:
		assertInt64(t, pool, ctx, `SELECT remaining_credits FROM bill_credit_packages WHERE tenant_id=$1`, tenantID, 100)
	default:
		t.Fatalf("terminal event status = %q", status)
	}
	assertInt64(t, pool, ctx, `SELECT frozen_credits FROM iam_accounts WHERE user_id=$1`, userID, 0)
}
