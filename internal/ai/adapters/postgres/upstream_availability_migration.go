package postgres

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"strconv"
	"time"
	"xiaodou/dai/internal/ai/routing"
)

// ImportUpstreamAvailability is restart-safe. Existing v2 records are never
// overwritten, and pre-cutover unknown health is never imported as verified.
func ImportUpstreamAvailability(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, a *routing.RedisAvailability) error {
	if pool == nil || redisClient == nil {
		return routing.ErrAvailabilityUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if done, err := redisClient.Exists(ctx, "dai:availability:v2:migrated").Result(); err != nil {
		return err
	} else if done > 0 {
		return nil
	}
	var started time.Time
	if err := pool.QueryRow(ctx, `SELECT started_at FROM ai_upstream_runtime_metadata WHERE singleton`).Scan(&started); err != nil {
		return err
	}
	rows, err := pool.Query(ctx, `SELECT id::text,COALESCE(invalid_reason,'') FROM ai_upstream_accounts WHERE status='active' AND invalid_at IS NOT NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, reason string
		if err = rows.Scan(&id, &reason); err != nil {
			rows.Close()
			return err
		}
		scope := routing.NewFaultScope("authentication", "direct_upstream", id, "", "", "", "")
		if err = a.ImportCooldown(ctx, scope, started.Add(30*time.Minute), "unauthorized"); err != nil {
			rows.Close()
			return err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = pool.Query(ctx, `SELECT id::text,pool_id::text,cooldown_until FROM ai_provider_oauth_credentials WHERE status='active' AND cooldown_until IS NOT NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, poolID string
		var until time.Time
		if err = rows.Scan(&id, &poolID, &until); err != nil {
			rows.Close()
			return err
		}
		if err = a.ImportCooldown(ctx, routing.NewFaultScope("authentication", "oauth_pool", poolID, "", id, "", ""), until, "legacy_credential_cooldown"); err != nil {
			rows.Close()
			return err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	ids, err := redisClient.SMembers(ctx, "dai:health:targets").Result()
	if err != nil {
		return err
	}
	for _, id := range ids {
		old, e := redisClient.HGetAll(ctx, "dai:health:target:"+id).Result()
		if e != nil {
			return e
		}
		if old["state"] != "1" && old["state"] != "2" {
			continue
		}
		resourceKind, resourceID, endpoint := "direct_upstream", id, ""
		if old["kind"] == "1" {
			resourceKind = "oauth_pool"
		} else if old["kind"] == "2" {
			if e = pool.QueryRow(ctx, `SELECT account_id::text FROM ai_upstream_account_endpoints WHERE id=$1::uuid`, id).Scan(&resourceID); e != nil {
				continue
			}
			endpoint = id
		}
		ms, _ := strconv.ParseInt(old["next_probe_at_ms"], 10, 64)
		until := time.UnixMilli(ms)
		cap := time.Now().Add(5 * time.Minute)
		if until.After(cap) {
			until = cap
		}
		if old["state"] == "2" {
			until = time.Now()
		}
		scopeKind := "transport"
		if resourceKind == "direct_upstream" && endpoint == "" {
			scopeKind = "authentication"
		}
		scope := routing.NewFaultScope(scopeKind, resourceKind, resourceID, endpoint, "", "", "")
		if e = a.ImportCooldown(ctx, scope, until, "legacy_cooldown"); e != nil {
			return e
		}
	}
	return redisClient.Set(ctx, "dai:availability:v2:migrated", started.UTC().Format(time.RFC3339), 0).Err()
}
