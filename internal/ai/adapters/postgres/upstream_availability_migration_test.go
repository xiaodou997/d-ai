package postgres

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"os"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestAvailabilityMigrationPreservesDisabledAndBoundsLegacyFailures(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	invalid, disabled, legacy := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, item := range []struct{ id, status string }{{invalid, "invalid"}, {disabled, "disabled"}, {legacy, "active"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_accounts(id,name,tenant_display_name,api_key_ciphertext,status,invalid_at,invalid_reason) VALUES($1::uuid,$1::text,$1::text,'secret',$2,CASE WHEN $2='invalid' THEN now() END,'old rejection')`, item.id, item.status); err != nil {
			t.Fatal(err)
		}
	}
	// Reconstruct the pre-cutover additions inside this isolated test schema.
	if _, err = pool.Exec(ctx, `DROP TABLE ai_upstream_runtime_metadata;
 ALTER TABLE ai_request_attempts DROP COLUMN upstream_kind,DROP COLUMN upstream_resource_id,DROP COLUMN endpoint_id,DROP COLUMN credential_id,DROP COLUMN model_code,DROP COLUMN upstream_model,DROP COLUMN operation,DROP COLUMN stream,DROP COLUMN completed_at,DROP COLUMN outcome,DROP COLUMN cooldown_entered,DROP COLUMN latency_ms;
 UPDATE dai_schema_metadata SET version=42`); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../../db/changes/0043_20260914_upstream_availability.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM ai_upstream_accounts WHERE id=$1`, disabled).Scan(&status); err != nil || status != "disabled" {
		t.Fatal(status, err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM ai_upstream_accounts WHERE id=$1`, invalid).Scan(&status); err != nil || status != "active" {
		t.Fatal(status, err)
	}
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	a := routing.NewRedisAvailability(client)
	if err = client.SAdd(ctx, "dai:health:targets", legacy).Err(); err != nil {
		t.Fatal(err)
	}
	if err = client.HSet(ctx, "dai:health:target:"+legacy, "kind", 0, "state", 1, "next_probe_at_ms", time.Now().Add(time.Hour).UnixMilli()).Err(); err != nil {
		t.Fatal(err)
	}
	if err = ImportUpstreamAvailability(ctx, pool, client, a); err != nil {
		t.Fatal(err)
	}
	states, err := a.List(ctx, "direct_upstream", invalid)
	if err != nil || len(states) != 1 || states[0].Scope.Kind != "authentication" || states[0].Phase != routing.Cooling || states[0].VerifiedAt != 0 {
		t.Fatalf("states=%+v err=%v", states, err)
	}
	states, err = a.List(ctx, "direct_upstream", legacy)
	if err != nil || len(states) != 1 || states[0].RetryAt > time.Now().Add(5*time.Minute+time.Second).UnixMilli() {
		t.Fatalf("legacy=%+v err=%v", states, err)
	}
	if err = a.Resume(ctx, "direct_upstream", legacy); err != nil {
		t.Fatal(err)
	}
	if err = ImportUpstreamAvailability(ctx, pool, client, a); err != nil {
		t.Fatal(err)
	}
	states, err = a.List(ctx, "direct_upstream", legacy)
	if err != nil || states[0].Phase != routing.Recovering {
		t.Fatal("migration replay overwrote new state", states, err)
	}
}
