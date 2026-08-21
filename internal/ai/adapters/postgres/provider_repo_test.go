package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
)

var accountRepoTestDSNs = []string{
	os.Getenv("AI_TEST_DATABASE_URL"),
	"postgres://postgres:postgres@127.0.0.1:15432/dai_test?sslmode=disable",
	"postgres://postgres:postgres@127.0.0.1:5432/dai_test?sslmode=disable",
}

func openAccountRepoTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	ctx := context.Background()
	var lastErr error
	for _, dsn := range accountRepoTestDSNs {
		if dsn == "" {
			continue
		}

		bootstrapCfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			lastErr = err
			continue
		}
		bootstrapCfg.MaxConns = 1
		bootstrapPool, err := pgxpool.NewWithConfig(ctx, bootstrapCfg)
		if err != nil {
			lastErr = err
			continue
		}
		if err := bootstrapPool.Ping(ctx); err != nil {
			bootstrapPool.Close()
			lastErr = err
			continue
		}

		schema := fmt.Sprintf("provider_repo_test_%d", time.Now().UnixNano())
		if _, err := bootstrapPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
			bootstrapPool.Close()
			lastErr = err
			continue
		}

		stmts := []string{
			fmt.Sprintf(`CREATE TABLE %s.ai_groups (
					id UUID PRIMARY KEY
				)`, schema),
			fmt.Sprintf(`CREATE TABLE %s.ai_upstream_accounts (
				id UUID PRIMARY KEY,
				name TEXT NOT NULL UNIQUE
			)`, schema),
			fmt.Sprintf(`CREATE TABLE %s.ai_group_targets (
				id UUID PRIMARY KEY,
				group_id UUID NOT NULL,
				target_kind TEXT NOT NULL,
				target_id UUID NOT NULL
			)`, schema),
			fmt.Sprintf(`CREATE TABLE %s.ai_upstream_models (
				id UUID PRIMARY KEY,
				upstream_kind TEXT NOT NULL,
				upstream_id UUID NOT NULL
			)`, schema),
			fmt.Sprintf(`CREATE TABLE %s.ai_upstream_resource_tenant_policies (
				resource_kind TEXT NOT NULL,
				resource_id UUID NOT NULL,
				tenant_id TEXT NOT NULL,
				access_granted BOOLEAN NOT NULL DEFAULT false,
				tenant_multiplier_override NUMERIC
			)`, schema),
		}
		ok := true
		for _, stmt := range stmts {
			if _, err := bootstrapPool.Exec(ctx, stmt); err != nil {
				ok = false
				lastErr = err
				break
			}
		}
		if !ok {
			_, _ = bootstrapPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			bootstrapPool.Close()
			continue
		}

		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			_, _ = bootstrapPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			bootstrapPool.Close()
			lastErr = err
			continue
		}
		cfg.MaxConns = 1
		cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, `SET search_path TO `+schema)
			return err
		}

		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			_, _ = bootstrapPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			bootstrapPool.Close()
			lastErr = err
			continue
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			_, _ = bootstrapPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			bootstrapPool.Close()
			lastErr = err
			continue
		}

		t.Cleanup(func() {
			pool.Close()
			_, _ = bootstrapPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			bootstrapPool.Close()
		})
		return pool, ctx
	}

	t.Skipf("skip provider repo DB test: %v", lastErr)
	return nil, nil
}

func TestAccountRepoDeleteAccountCleansBindings(t *testing.T) {
	pool, ctx := openAccountRepoTestPool(t)
	repo := NewAccountRepo(dbgen.New(pool), pool)

	const (
		accountID = "11111111-1111-1111-1111-111111111111"
		groupID   = "33333333-3333-3333-3333-333333333333"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id) VALUES ($1::uuid)`, groupID); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (id, name)
		VALUES ($1::uuid, 'account-1')
	`, accountID); err != nil {
		t.Fatalf("insert upstream account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_group_targets (id, group_id, target_kind, target_id)
		VALUES (
			'22222222-2222-2222-2222-222222222222'::uuid,
				$2::uuid,
				'direct_upstream',
				$1::uuid
			)
	`, accountID, groupID); err != nil {
		t.Fatalf("insert group target: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_models (id, upstream_kind, upstream_id)
		VALUES (
			'44444444-4444-4444-4444-444444444444'::uuid,
			'direct_upstream',
			$1::uuid
		)
	`, accountID); err != nil {
		t.Fatalf("insert upstream model binding: %v", err)
	}

	if err := repo.DeleteAccount(ctx, accountID); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_upstream_accounts WHERE id = $1::uuid`, accountID).Scan(&count); err != nil {
		t.Fatalf("count upstream accounts: %v", err)
	}
	if count != 0 {
		t.Fatalf("upstream account rows = %d, want 0", count)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_group_targets WHERE target_kind = 'direct_upstream' AND target_id = $1::uuid`, accountID).Scan(&count); err != nil {
		t.Fatalf("count group targets: %v", err)
	}
	if count != 0 {
		t.Fatalf("group target rows = %d, want 0", count)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_upstream_models WHERE upstream_kind = 'direct_upstream' AND upstream_id = $1::uuid`, accountID).Scan(&count); err != nil {
		t.Fatalf("count upstream models: %v", err)
	}
	if count != 0 {
		t.Fatalf("upstream model rows = %d, want 0", count)
	}
}

func TestAccountSecretFromRowMapsManagementFields(t *testing.T) {
	row := dbgen.AiUpstreamAccount{
		ApiKeyCiphertext:  "ciphertext",
		BaseUrl:           "https://upstream.example",
		ExtraHeaders:      []byte(`{"X-Tenant":"tenant-1"}`),
		DefaultProtocol:   "anthropic",
		TenantDisplayName: "Shared Claude",
		TenantAccessMode:  "allow_all",
		Status:            "active",
	}

	got := accountSecretFromRow(row)
	if got.Ciphertext != row.ApiKeyCiphertext || got.BaseURL != row.BaseUrl || string(got.ExtraHeaders) != string(row.ExtraHeaders) {
		t.Fatalf("secret transport fields = %#v", got)
	}
	if got.DefaultProtocol != row.DefaultProtocol || got.TenantDisplayName != row.TenantDisplayName || got.TenantAccessMode != row.TenantAccessMode || got.Status != row.Status {
		t.Fatalf("secret account metadata = %#v", got)
	}
}
