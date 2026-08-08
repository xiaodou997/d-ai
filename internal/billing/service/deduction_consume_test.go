package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	shared "xiaodou/dai/internal/domain"
)

// TestConsumeDisallowOverdraft 验证 DisallowOverdraft（订阅严格扣款）语义：
//  1. 余额不足 + DisallowOverdraft ⇒ 整单失败 ErrInsufficientBalance，不转透支、不写流水；
//  2. 余额不足 + 显式允许尾差透支 ⇒ 照常成交并记透支；
//  3. 同幂等键重放 ⇒ 返回同一 event，不重复扣减。
//
// 需要真实 Postgres：设置 DAI_TEST_DATABASE_URL 后运行，否则跳过。
func TestConsumeDisallowOverdraft(t *testing.T) {
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
	tenantID := "t_sub_" + suffix
	userID := "u_sub_" + suffix

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nsql=%s", err, sql)
		}
	}
	// 用完清理，避免污染共享测试库
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM bill_events WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_packages WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id=$1`, tenantID)
	})

	mustExec(`INSERT INTO iam_tenants (tenant_id, tenant_name, status, overdraft_limit, current_overdraft)
	          VALUES ($1, 'sub-test', 'active', 1000000, 0)`, tenantID)
	mustExec(`INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, overdraft_limit, current_overdraft)
	          VALUES ($1, $2, $3, 'x', 4, 1000000, 0)`, userID, tenantID, "u_user-"+suffix)
	// 用户额度包仅 50 micro-USD 额度
	mustExec(`INSERT INTO bill_credit_packages (package_id, package_type, tenant_id, user_id, total_credits, remaining_credits, source, status)
	          VALUES ($1, 'user', $2, $3, 50, 50, 'ADMIN_RECHARGE', 'available')`, "pkg-"+suffix, tenantID, userID)

	svc := NewDeductionService(pool, zap.NewNop())

	// ---- 1) 不足额 + DisallowOverdraft ⇒ 整单失败，不透支不记账 ----
	_, err = svc.Consume(ConsumeParams{
		IdempotencyKey:    "strict-" + suffix,
		TenantID:          tenantID,
		UserID:            userID,
		Description:       "订阅严格扣款(不足额)",
		UserAmount:        100, // > 50
		DisallowOverdraft: true,
	})
	if err != shared.ErrInsufficientBalance {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
	assertInt64(t, pool, ctx, `SELECT remaining_credits FROM bill_credit_packages WHERE tenant_id=$1`, tenantID, 50) // 未扣
	assertInt64(t, pool, ctx, `SELECT current_overdraft FROM iam_accounts WHERE user_id=$1`, userID, 0)              // 未透支
	assertInt64(t, pool, ctx, `SELECT count(*) FROM bill_events WHERE idempotency_key=$1`, "strict-"+suffix, 0)      // 未记账

	// ---- 2) 不足额 + 允许透支（settle 老路径）⇒ 照常成交，扣 50 + 透支 50 ----
	res2, err := svc.Consume(ConsumeParams{
		IdempotencyKey: "lax-" + suffix,
		TenantID:       tenantID,
		UserID:         userID,
		Description:    "按量聚合扣款(允许透支)",
		UserAmount:     100,
		AllowOverdraft: true,
	})
	if err != nil {
		t.Fatalf("lax consume should succeed, got %v", err)
	}
	if res2.UserDeducted != 50 || res2.UserOverdraftAdd != 50 {
		t.Fatalf("expected deducted=50 overdraft=50, got deducted=%d overdraft=%d", res2.UserDeducted, res2.UserOverdraftAdd)
	}
	assertInt64(t, pool, ctx, `SELECT remaining_credits FROM bill_credit_packages WHERE tenant_id=$1`, tenantID, 0)
	assertInt64(t, pool, ctx, `SELECT current_overdraft FROM iam_accounts WHERE user_id=$1`, userID, 50)

	// ---- 3) 同幂等键重放 ⇒ 返回同一 event，不重复扣减 ----
	res3, err := svc.Consume(ConsumeParams{
		IdempotencyKey: "lax-" + suffix,
		TenantID:       tenantID,
		UserID:         userID,
		Description:    "重放",
		UserAmount:     100,
		AllowOverdraft: true,
	})
	if err != nil {
		t.Fatalf("replay should succeed, got %v", err)
	}
	if res3.EventID != res2.EventID {
		t.Fatalf("replay should return same event: %s != %s", res3.EventID, res2.EventID)
	}
	// 透支不因重放而翻倍
	assertInt64(t, pool, ctx, `SELECT current_overdraft FROM iam_accounts WHERE user_id=$1`, userID, 50)
	assertInt64(t, pool, ctx, `SELECT count(*) FROM bill_events WHERE idempotency_key=$1`, "lax-"+suffix, 1)
}

func assertInt64(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql, arg string, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, sql, arg).Scan(&got); err != nil {
		t.Fatalf("assert query failed: %v\nsql=%s", err, sql)
	}
	if got != want {
		t.Fatalf("assert mismatch: want %d got %d\nsql=%s", want, got, sql)
	}
}
