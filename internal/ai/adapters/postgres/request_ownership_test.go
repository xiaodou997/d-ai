package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
)

func TestRequestStoreSeparatedDatabaseRoles(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	runtimeRole := fmt.Sprintf("record_runtime_%d", time.Now().UnixNano())
	billingRole := fmt.Sprintf("record_billing_%d", time.Now().UnixNano())
	var schema string
	if err = pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{runtimeRole, billingRole} {
		if _, err = pool.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	// Role cleanup uses an independent connection after the schema pool closes.
	baseCfg := pool.Config().Copy()
	baseCfg.AfterConnect = nil
	baseCfg.ConnConfig.RuntimeParams["search_path"] = "public"
	defer func() {
		_ = cleanup(ctx)
		base, e := pgxpool.NewWithConfig(ctx, baseCfg)
		if e == nil {
			defer base.Close()
			for _, role := range []string{runtimeRole, billingRole} {
				_, _ = base.Exec(ctx, `DROP OWNED BY `+pgx.Identifier{role}.Sanitize())
				_, _ = base.Exec(ctx, `DROP ROLE `+pgx.Identifier{role}.Sanitize())
			}
		}
	}()
	raw, err := os.ReadFile("../../../db/ownership.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ReplaceAll(string(raw), "\\set ON_ERROR_STOP on", "")
	for name, value := range map[string]string{"schema_name": schema, "runtime_role": runtimeRole, "billing_role": billingRole} {
		sql = strings.ReplaceAll(sql, `:"`+name+`"`, pgx.Identifier{value}.Sanitize())
		sql = strings.ReplaceAll(sql, `:'`+name+`'`, "'"+value+"'")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, sql)
	conn.Release()
	if err != nil {
		t.Fatal(err)
	}
	rolePool := func(role string) *pgxpool.Pool {
		cfg := pool.Config().Copy()
		original := cfg.AfterConnect
		cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
			if original != nil {
				if err := original(ctx, c); err != nil {
					return err
				}
			}
			_, err := c.Exec(ctx, `SET ROLE `+pgx.Identifier{role}.Sanitize())
			return err
		}
		p, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	runtime, financial := rolePool(runtimeRole), rolePool(billingRole)
	defer runtime.Close()
	defer financial.Close()
	// Account provisioning executes through identity triggers under the runtime role.
	seedDirectBillingAccounts(t, ctx, runtime, "role-tenant", "role-user", "role")
	grantTestBalance(t, ctx, financial, ledger.Ref{Kind: ledger.KindTenant, ID: "role-tenant", TenantID: "role-tenant"}, 1000)
	grantTestBalance(t, ctx, financial, ledger.Ref{Kind: ledger.KindUser, ID: "role-user", TenantID: "role-tenant"}, 1000)
	if _, err = runtime.Exec(ctx, `UPDATE bill_accounts SET balance_micro=0`); err == nil {
		t.Fatal("runtime role can mutate money")
	}
	store := NewRequestStore(runtime, fixedUsageBiller{result: domain.BillingResult{TenantPayableMicro: 100, UserChargedMicro: 200}}, nil).WithFinancialPool(financial)
	req := usageCompletionRequest("role-request", "role-tenant", "role-user")
	if err = store.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	defer req.RecordLeaseStop()
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DrainSettlements(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if got := accountBalance(t, ctx, pool, "role-user"); got != 800 {
		t.Fatalf("role settlement balance=%d", got)
	}
	if _, err = runtime.Exec(ctx, `UPDATE bill_settlements SET state='review' WHERE request_id=$1`, req.RequestID); err == nil {
		t.Fatal("runtime role can mutate financial processing state")
	}
	req.ClientDeliveryState = domain.ClientDeliveryWriteFailed
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err = store.Refund(ctx, req.RequestID, "role test", "admin"); err != nil {
		t.Fatal(err)
	}
	if err = store.MaintainRecords(ctx); err != nil {
		t.Fatal(err)
	}
}
