package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
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
				name TEXT NOT NULL UNIQUE,
				status TEXT NOT NULL DEFAULT 'disabled'
			)`, schema),
			fmt.Sprintf(`CREATE TABLE %s.ai_upstream_account_endpoints (
				id UUID PRIMARY KEY,
				account_id UUID NOT NULL REFERENCES %s.ai_upstream_accounts(id) ON DELETE CASCADE,
				api_format TEXT NOT NULL,
				base_url TEXT NOT NULL,
				UNIQUE (account_id, api_format)
			)`, schema, schema),
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
		INSERT INTO ai_upstream_account_endpoints (id, account_id, api_format, base_url)
		VALUES ('55555555-5555-5555-5555-555555555555'::uuid, $1::uuid, 'openai_responses', 'https://example.test')
	`, accountID); err != nil {
		t.Fatalf("insert upstream endpoint: %v", err)
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
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_upstream_account_endpoints WHERE account_id = $1::uuid`, accountID).Scan(&count); err != nil {
		t.Fatalf("count upstream endpoints: %v", err)
	}
	if count != 0 {
		t.Fatalf("upstream endpoint rows = %d, want 0", count)
	}
}

func TestAccountSecretFromRowMapsManagementFields(t *testing.T) {
	row := dbgen.AiUpstreamAccount{
		ApiKeyCiphertext:  "ciphertext",
		TenantDisplayName: "Shared Claude",
		TenantAccessMode:  "allow_all",
		Status:            "active",
	}

	got := accountSecretFromRow(row)
	if got.Ciphertext != row.ApiKeyCiphertext {
		t.Fatalf("secret transport fields = %#v", got)
	}
	if got.TenantDisplayName != row.TenantDisplayName || got.TenantAccessMode != row.TenantAccessMode || got.Status != row.Status {
		t.Fatalf("secret account metadata = %#v", got)
	}
}

func TestAccountRepoSerializesEndpointAvailabilityInvariant(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("open endpoint invariant test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	repo := NewAccountRepo(NewQueries(pool), pool)

	const (
		deleteAccountID   = "71000000-0000-0000-0000-000000000001"
		deleteActiveID    = "71000000-0000-0000-0000-000000000002"
		deleteDormantID   = "71000000-0000-0000-0000-000000000003"
		raceAccountID     = "72000000-0000-0000-0000-000000000001"
		raceFirstID       = "72000000-0000-0000-0000-000000000002"
		raceSecondID      = "72000000-0000-0000-0000-000000000003"
		activateAccountID = "73000000-0000-0000-0000-000000000001"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, api_key_ciphertext, status)
		VALUES
			($1::uuid, 'delete-invariant', 'Delete Invariant', 'cipher', 'active'),
			($2::uuid, 'race-invariant', 'Race Invariant', 'cipher', 'active'),
			($3::uuid, 'activate-invariant', 'Activate Invariant', 'cipher', 'disabled')
	`, deleteAccountID, raceAccountID, activateAccountID); err != nil {
		t.Fatalf("seed endpoint invariant accounts: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_account_endpoints (id, account_id, api_format, base_url, status)
		VALUES
			($3::uuid, $1::uuid, 'openai_responses', 'https://example.test', 'active'),
			($4::uuid, $1::uuid, 'openai_chat', 'https://example.test', 'disabled'),
			($5::uuid, $2::uuid, 'openai_responses', 'https://example.test', 'active'),
			($6::uuid, $2::uuid, 'openai_chat', 'https://example.test', 'active'),
			(gen_random_uuid(), $7::uuid, 'openai_responses', 'https://example.test', 'active')
	`, deleteAccountID, raceAccountID, deleteActiveID, deleteDormantID, raceFirstID, raceSecondID, activateAccountID); err != nil {
		t.Fatalf("seed endpoint invariant endpoints: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_models (upstream_kind, upstream_id, model_code, capability_type, upstream_model_name, status)
		VALUES ('direct_upstream', $1::uuid, 'image-model', 'image', 'image-model', 'active')
	`, activateAccountID); err != nil {
		t.Fatalf("seed endpoint invariant model: %v", err)
	}

	if err := repo.DeleteEndpoint(ctx, deleteAccountID, deleteActiveID); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("DeleteEndpoint(last active) error = %v, want validation", err)
	}

	results := make(chan error, 2)
	for _, endpoint := range []struct {
		id       string
		protocol domain.UpstreamProtocol
	}{{raceFirstID, domain.ProtocolOpenAIResponses}, {raceSecondID, domain.ProtocolOpenAIChat}} {
		endpoint := endpoint
		go func() {
			_, err := repo.UpdateEndpoint(ctx, raceAccountID, endpoint.id, domain.UpstreamAccountEndpointWrite{
				APIFormat: endpoint.protocol, BaseURL: "https://example.test",
				AuthScheme: domain.EndpointAuthFormatDefault, ExtraHeaders: []byte(`{}`),
				Status: domain.EndpointStatusDisabled,
			})
			results <- err
		}()
	}
	var succeeded, rejected int
	for range 2 {
		if err := <-results; err == nil {
			succeeded++
		} else if errors.Is(err, domain.ErrValidation) {
			rejected++
		} else {
			t.Fatalf("concurrent UpdateEndpoint() error = %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent endpoint updates succeeded/rejected = %d/%d, want 1/1", succeeded, rejected)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_upstream_account_endpoints WHERE account_id = $1::uuid AND status = 'active'`, raceAccountID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active endpoints after concurrent updates = %d, want 1", active)
	}
	if _, err := repo.UpdateAccountStatus(ctx, activateAccountID, domain.UpstreamAccountStatusActive); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("UpdateAccountStatus(incompatible bindings) error = %v, want validation", err)
	}
}
