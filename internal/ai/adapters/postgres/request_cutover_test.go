package postgres

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/ai/recordcutover"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/db"
)

func TestRequestCutoverPreservesAssetsAndReleasesHistory(t *testing.T) {
	ctx := context.Background()
	base, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	schema := "cutover_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = base.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer base.Exec(ctx, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
	cfg := base.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.AfterConnect = nil
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(filepath.Join(string(bytesTrimSpace(root)), "internal/db/testdata/schema_0040.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(old)); err != nil {
		t.Fatal(err)
	}
	seedDirectBillingAccounts(t, ctx, pool, "cutover-tenant", "cutover-user", "cutover")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "cutover-tenant", TenantID: "cutover-tenant"}, 700)
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: "cutover-user", TenantID: "cutover-tenant"}, 900)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = ledger.Charge(ctx, tx, ledger.Ref{Kind: ledger.KindUser, ID: "cutover-user", TenantID: "cutover-tenant"}, 1000); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	group := uuid.NewString()
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_groups(id,tenant_id,name,retail_price_book_id) VALUES($1::uuid,'cutover-tenant','retained',gen_random_uuid())`, group)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_api_keys(id,owner_type,tenant_id,user_id,group_id,key_hash,key_ciphertext,name,quota_limit,quota_used) VALUES($1::uuid,'user','cutover-tenant','cutover-user',$2::uuid,$1,'secret','kept-key',1000,350)`, key, group)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_sub_subscriptions(id,tenant_id,user_id,plan_id,order_id,plan_name_snapshot,duration_days,total_limit_micro,total_used_micro,win5h_start,win5h_used_micro,group_quota_debit_multipliers) VALUES(gen_random_uuid(),'cutover-tenant','cutover-user',gen_random_uuid(),gen_random_uuid(),'retained',30,10000,900,now(),400,jsonb_build_object($1::text,1.5))`, group)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_request_payloads(request_id,client_protocol,request_path,request_model,request_status,request_messages)
 SELECT 'old-'||n,'openai_chat','/v1/chat/completions','model','failed',jsonb_build_object('content',(SELECT string_agg(md5(random()::text),'') FROM generate_series(1,1024))) FROM generate_series(1,500) n`)
	before, err := recordcutover.Inspect(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := recordcutover.Assets(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(string(bytesTrimSpace(root)), "internal/db/changes/0041_20260914_request_ledger_rebuild.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	after, err := recordcutover.Release(ctx, pool, hash)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := recordcutover.Assets(ctx, pool)
	if err != nil || got != hash {
		t.Fatalf("asset equality failed: %s / %s, %v", hash, got, err)
	}
	if err = db.VerifySchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var oldRelation *string
	if err = pool.QueryRow(ctx, `SELECT to_regclass('ai_usage_logs')::text`).Scan(&oldRelation); err != nil || oldRelation != nil {
		t.Fatalf("old usage table survived: %v, %v", oldRelation, err)
	}
	if got := accountBalance(t, ctx, pool, "cutover-user"); got != -100 {
		t.Fatalf("debt changed: %d", got)
	}
	t.Logf("isolated rehearsal: before=%d after=%d reclaimed=%d bytes; assets identical", before.SchemaBytes, after.SchemaBytes, before.SchemaBytes-after.SchemaBytes)
}
func bytesTrimSpace(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
