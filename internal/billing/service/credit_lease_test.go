package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"xiaodou/dai/internal/dbtest"
	shared "xiaodou/dai/internal/domain"
)

func TestCreditLeaseCanSettleIdempotentlyAfterEscrowRelease(t *testing.T) {
	dsn := os.Getenv("URM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set URM_TEST_DATABASE_URL to run this DB-backed test")
	}
	applyURMTestMigrations(t, dsn)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "lease-tenant-" + suffix
	userID := "lease-user-" + suffix
	clientID := "lease-client-" + suffix
	packageID := "lease-package-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_credit_leases WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_events WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_packages WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM gov_subject_service_access WHERE subject_type='tenant' AND subject_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM gov_clients WHERE client_id=$1`, clientID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_users WHERE user_id=$1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id=$1`, tenantID)
	})
	mustLeaseExec(t, ctx, pool, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status, frozen_credits, current_overdraft)
		VALUES ($1, $2, 'active', 0, 0)
	`, tenantID, tenantID)
	mustLeaseExec(t, ctx, pool, `
		INSERT INTO iam_users (user_id, tenant_id, username, password_hash, status, frozen_credits, current_overdraft)
		VALUES ($1, $2, $3, 'x', 'active', 0, 0)
	`, userID, tenantID, userID)
	mustLeaseExec(t, ctx, pool, `
		INSERT INTO bill_credit_packages (
		  package_id, package_type, tenant_id, user_id,
		  total_credits, remaining_credits, status, source
		) VALUES ($1, 'user', $2, $3, 100, 100, 'available', 'ADMIN_RECHARGE')
	`, packageID, tenantID, userID)
	mustLeaseExec(t, ctx, pool, `
		INSERT INTO gov_clients (client_id, display_name, portal_enabled, status)
		VALUES ($1, $1, true, 'active')
	`, clientID)
	mustLeaseExec(t, ctx, pool, `
		INSERT INTO gov_subject_service_access (
		  subject_type, subject_id, access_mode, service_ids, version, created_by, updated_by
		) VALUES ('tenant', $1, 'selected', ARRAY[$2], 1, 'test', 'test')
	`, tenantID, clientID)

	now := time.Now().UTC()
	service := NewCreditLeaseService(pool, zap.NewNop())
	service.now = func() time.Time { return now }
	lease, err := service.Acquire(ctx, AcquireLeaseParams{
		ClientID: clientID, ClientWindowID: "window-" + suffix,
		TenantID: tenantID, UserID: userID, RequestedUserMicro: 100,
		TTL: 30 * time.Second, Grace: time.Minute,
	})
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if lease.AccountState != accountStateExhausted || lease.AllowFurtherUsage {
		t.Fatalf("acquire account state = %s allow=%v, want exhausted/false",
			lease.AccountState, lease.AllowFurtherUsage)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 100)

	now = now.Add(31 * time.Second)
	if released, err := service.ReapExpired(ctx, 10); err != nil || released != 0 {
		t.Fatalf("move lease to grace: released=%d err=%v", released, err)
	}
	inGrace, err := service.Get(ctx, lease.LeaseID, clientID)
	if err != nil || inGrace.EscrowState != LeaseEscrowGrace {
		t.Fatalf("lease in grace = %#v, %v", inGrace, err)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 100)

	now = now.Add(60 * time.Second)
	if released, err := service.ReapExpired(ctx, 10); err != nil || released != 1 {
		t.Fatalf("release expired escrow: released=%d err=%v", released, err)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 0)

	secondLease, err := service.Acquire(ctx, AcquireLeaseParams{
		ClientID: clientID, ClientWindowID: "window-second-" + suffix,
		TenantID: tenantID, UserID: userID, RequestedUserMicro: 1,
		TTL: 30 * time.Second, Grace: time.Minute,
	})
	if err != nil {
		t.Fatalf("acquire second lease: %v", err)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 1)

	deduction := NewDeductionService(pool, nil, zap.NewNop())
	if _, err := deduction.Consume(ConsumeParams{
		IdempotencyKey:    "strict-debit-" + suffix,
		TenantID:          tenantID,
		UserID:            userID,
		UserAmount:        100,
		DisallowOverdraft: true,
	}); !errors.Is(err, shared.ErrInsufficientBalance) {
		t.Fatalf("strict debit with protected escrow error = %v, want insufficient balance", err)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT remaining_credits FROM bill_credit_packages WHERE package_id=$1`, packageID, 100)
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 1)

	settled, err := service.Settle(ctx, SettleLeaseParams{
		LeaseID: lease.LeaseID, ClientID: clientID, SettlementID: "settlement-" + suffix,
		ActualUserMicro: 120,
	})
	if err != nil {
		t.Fatalf("late settle: %v", err)
	}
	if settled.SettlementState != LeaseSettlementSettled || settled.UserDeducted != 99 ||
		settled.UserDebtAdded != 21 || settled.SettledEventID == "" {
		t.Fatalf("unexpected settlement receipt: %#v", settled)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT remaining_credits FROM bill_credit_packages WHERE package_id=$1`, packageID, 1)
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 1)
	assertLeaseAccountInt(t, ctx, pool, `SELECT current_overdraft FROM iam_users WHERE user_id=$1`, userID, 21)

	replayed, err := service.Settle(ctx, SettleLeaseParams{
		LeaseID: lease.LeaseID, ClientID: clientID, SettlementID: "settlement-" + suffix,
		ActualUserMicro: 120,
	})
	if err != nil || replayed.SettledEventID != settled.SettledEventID ||
		replayed.UserDeducted != 99 || replayed.UserDebtAdded != 21 {
		t.Fatalf("idempotent settlement replay = %#v, %v", replayed, err)
	}
	if _, err := service.Settle(ctx, SettleLeaseParams{
		LeaseID: lease.LeaseID, ClientID: clientID, SettlementID: "settlement-" + suffix,
		ActualUserMicro: 121,
	}); !errors.Is(err, shared.ErrCreditLeaseSettlement) {
		t.Fatalf("different replay error = %v, want settlement conflict", err)
	}
	if _, err := service.Settle(ctx, SettleLeaseParams{
		LeaseID: secondLease.LeaseID, ClientID: clientID, SettlementID: "settlement-" + suffix,
		ActualUserMicro: 1,
	}); !errors.Is(err, shared.ErrCreditLeaseSettlement) {
		t.Fatalf("cross-lease settlement ID reuse error = %v, want settlement conflict", err)
	}
	secondSettled, err := service.Settle(ctx, SettleLeaseParams{
		LeaseID: secondLease.LeaseID, ClientID: clientID, SettlementID: "settlement-second-" + suffix,
		ActualUserMicro: 1,
	})
	if err != nil {
		t.Fatalf("settle second lease: %v", err)
	}
	if secondSettled.UserDeducted != 1 || secondSettled.UserDebtAdded != 0 {
		t.Fatalf("second settlement consumed reserved credit incorrectly: %#v", secondSettled)
	}
	assertLeaseAccountInt(t, ctx, pool, `SELECT remaining_credits FROM bill_credit_packages WHERE package_id=$1`, packageID, 0)
	assertLeaseAccountInt(t, ctx, pool, `SELECT frozen_credits FROM iam_users WHERE user_id=$1`, userID, 0)
	assertLeaseAccountInt(t, ctx, pool, `SELECT current_overdraft FROM iam_users WHERE user_id=$1`, userID, 21)
	assertLeaseAccountInt(t, ctx, pool, `SELECT count(*) FROM bill_events WHERE idempotency_key=$1`, "credit-lease:settlement-"+suffix, 1)
}

func applyURMTestMigrations(t *testing.T, dsn string) {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database url: %v", err)
	}
	sqlDB := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := dbtest.EnsureCanonicalSchema(context.Background(), sqlDB); err != nil {
		t.Fatalf("initialize canonical test schema: %v", err)
	}
}

func mustLeaseExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("seed credit lease test: %v", err)
	}
}

func assertLeaseAccountInt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, arg any, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, query, arg).Scan(&got); err != nil {
		t.Fatalf("query account state: %v", err)
	}
	if got != want {
		t.Fatalf("account state = %d, want %d", got, want)
	}
}
