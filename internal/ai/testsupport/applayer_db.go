package testsupport

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AppLayerFixture 预置应用层测试数据(发布模型语义):
//   - TenantAAgentID:tenant-a 应用,已发布给 tenant-a 终端用户;
//   - TenantAImageGenAgentID / TenantAImageEditAgentID:tenant-a 应用,未发布给用户;
//   - TenantBAgentID:tenant-b 应用,未发布;
//   - UserAgentID:tenant-a 用户 user-a 自建应用(仅自用)。
type AppLayerFixture struct {
	TenantAAgentID          string
	TenantBAgentID          string
	TenantAImageGenAgentID  string
	TenantAImageEditAgentID string
	UserAgentID             string
	UserAgentOwnerUserID    string
}

var defaultDSNs = []string{
	"postgres://postgres:postgres@127.0.0.1:15432/dai_test?sslmode=disable",
	"postgres://postgres:postgres@127.0.0.1:5432/dai_test?sslmode=disable",
}

func OpenAppLayerTestPool(ctx context.Context) (*pgxpool.Pool, AppLayerFixture, func(context.Context) error, error) {
	dsns := []string{}
	if dsn := os.Getenv("AI_APP_LAYER_TEST_DATABASE_URL"); dsn != "" {
		dsns = append(dsns, dsn)
	}
	dsns = append(dsns, defaultDSNs...)

	var lastErr error
	for _, dsn := range dsns {
		pool, fixture, cleanup, err := openWithDSN(ctx, dsn)
		if err == nil {
			return pool, fixture, cleanup, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no database url configured")
	}
	return nil, AppLayerFixture{}, nil, lastErr
}

func openWithDSN(ctx context.Context, dsn string) (*pgxpool.Pool, AppLayerFixture, func(context.Context) error, error) {
	bootstrapCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, AppLayerFixture{}, nil, err
	}
	bootstrapCfg.MaxConns = 1
	bootstrapPool, err := pgxpool.NewWithConfig(ctx, bootstrapCfg)
	if err != nil {
		return nil, AppLayerFixture{}, nil, err
	}
	defer bootstrapPool.Close()
	if err := bootstrapPool.Ping(ctx); err != nil {
		return nil, AppLayerFixture{}, nil, err
	}

	schema := fmt.Sprintf("app_layer_test_%d", time.Now().UnixNano())
	if _, err := bootstrapPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		return nil, AppLayerFixture{}, nil, err
	}
	cleanupSchema := func(ctx context.Context) error {
		_, err := bootstrapPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		return err
	}

	statements := []string{
		fmt.Sprintf(`CREATE TABLE %s.ai_app_prompts (
			id UUID PRIMARY KEY,
			owner_type TEXT NOT NULL CHECK (owner_type IN ('tenant', 'user')),
			owner_tenant_id TEXT NOT NULL,
			owner_user_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			current_version INTEGER NOT NULL DEFAULT 1,
			created_by TEXT,
			updated_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`, schema),
		fmt.Sprintf(`CREATE TABLE %s.ai_app_prompt_versions (
			id UUID PRIMARY KEY,
			prompt_id UUID NOT NULL,
			version INTEGER NOT NULL,
			template_text TEXT NOT NULL,
			variables JSONB NOT NULL DEFAULT '[]'::jsonb,
			notes TEXT NOT NULL DEFAULT '',
			created_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`, schema),
		fmt.Sprintf(`CREATE TABLE %s.ai_apps (
			id UUID PRIMARY KEY,
			owner_type TEXT NOT NULL CHECK (owner_type IN ('tenant', 'user')),
			owner_tenant_id TEXT NOT NULL,
			owner_user_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			capability TEXT NOT NULL,
			prompt_strategy TEXT NOT NULL DEFAULT 'caller_variables',
			group_id UUID NOT NULL,
			model_code TEXT NOT NULL,
			default_options JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by TEXT,
			updated_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`, schema),
		fmt.Sprintf(`CREATE TABLE %s.ai_app_prompt_bindings (
			app_id UUID NOT NULL,
			prompt_id UUID NOT NULL,
			binding_role TEXT NOT NULL,
			display_order INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (app_id, prompt_id)
		)`, schema),
		fmt.Sprintf(`CREATE TABLE %s.ai_app_publications (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			app_id UUID NOT NULL,
			publisher_scope TEXT NOT NULL DEFAULT 'tenant' CHECK (publisher_scope = 'tenant'),
			publisher_tenant_id TEXT NOT NULL,
			audience TEXT NOT NULL DEFAULT 'tenant_users' CHECK (audience = 'tenant_users'),
			status TEXT NOT NULL DEFAULT 'active',
			created_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (app_id, publisher_scope, publisher_tenant_id)
		)`, schema),
		fmt.Sprintf(`INSERT INTO %s.ai_app_prompts (id, owner_type, owner_tenant_id, owner_user_id, name) VALUES
			('00000000-0000-0000-0000-000000000003', 'tenant', 'tenant-a', '', 'tenant-a-prompt'),
			('00000000-0000-0000-0000-000000000004', 'tenant', 'tenant-b', '', 'tenant-b-prompt'),
			('00000000-0000-0000-0000-000000000005', 'user', 'tenant-a', 'user-a', 'user-a-prompt')`, schema),
		fmt.Sprintf(`INSERT INTO %s.ai_app_prompt_versions (id, prompt_id, version, template_text, variables, notes) VALUES
			('10000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000003', 1, 'tenant a private {{case_id}}', '["case_id"]'::jsonb, ''),
			('10000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000004', 1, 'tenant b private', '[]'::jsonb, ''),
			('10000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000005', 1, 'user a own {{topic}}', '["topic"]'::jsonb, '')`, schema),
		fmt.Sprintf(`INSERT INTO %s.ai_apps (
			id, owner_type, owner_tenant_id, owner_user_id, name, description, status, capability, prompt_strategy, group_id, model_code, default_options
		) VALUES
			('20000000-0000-0000-0000-000000000003', 'tenant', 'tenant-a', '', 'tenant-a-agent', '', 'active', 'chat', 'caller_variables', '30000000-0000-0000-0000-000000000003', 'gpt-tenant-a', '{}'::jsonb),
			('20000000-0000-0000-0000-000000000004', 'tenant', 'tenant-b', '', 'tenant-b-agent', '', 'active', 'chat', 'caller_variables', '30000000-0000-0000-0000-000000000004', 'gpt-tenant-b', '{}'::jsonb),
			('20000000-0000-0000-0000-000000000005', 'tenant', 'tenant-a', '', 'tenant-a-image-gen', '', 'active', 'image_generation', 'caller_variables', '30000000-0000-0000-0000-000000000003', 'gpt-image-tenant-a', '{"image":{"resolution":"1k","aspect_ratio":"1:1"}}'::jsonb),
			('20000000-0000-0000-0000-000000000006', 'tenant', 'tenant-a', '', 'tenant-a-image-edit', '', 'active', 'image_edit', 'caller_variables', '30000000-0000-0000-0000-000000000003', 'gpt-image-edit-tenant-a', '{"image":{"resolution":"1k","aspect_ratio":"1:1"}}'::jsonb),
			('20000000-0000-0000-0000-000000000007', 'user', 'tenant-a', 'user-a', 'user-a-agent', '', 'active', 'chat', 'caller_variables', '30000000-0000-0000-0000-000000000003', 'gpt-user-a', '{}'::jsonb)`, schema),
		fmt.Sprintf(`INSERT INTO %s.ai_app_prompt_bindings (app_id, prompt_id, binding_role, display_order) VALUES
			('20000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000003', 'primary', 0),
			('20000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000004', 'primary', 0),
			('20000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000003', 'primary', 0),
			('20000000-0000-0000-0000-000000000006', '00000000-0000-0000-0000-000000000003', 'primary', 0),
			('20000000-0000-0000-0000-000000000007', '00000000-0000-0000-0000-000000000005', 'primary', 0)`, schema),
		fmt.Sprintf(`INSERT INTO %s.ai_app_publications (id, app_id, publisher_scope, publisher_tenant_id, audience) VALUES
			('70000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000003', 'tenant', 'tenant-a', 'tenant_users')`, schema),
	}
	for _, stmt := range statements {
		if _, err := bootstrapPool.Exec(ctx, stmt); err != nil {
			_ = cleanupSchema(ctx)
			return nil, AppLayerFixture{}, nil, err
		}
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		_ = cleanupSchema(ctx)
		return nil, AppLayerFixture{}, nil, err
	}
	cfg.MaxConns = 1
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET search_path TO `+schema)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		_ = cleanupSchema(ctx)
		return nil, AppLayerFixture{}, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = cleanupSchema(ctx)
		return nil, AppLayerFixture{}, nil, err
	}

	fixture := AppLayerFixture{
		TenantAAgentID:          "20000000-0000-0000-0000-000000000003",
		TenantBAgentID:          "20000000-0000-0000-0000-000000000004",
		TenantAImageGenAgentID:  "20000000-0000-0000-0000-000000000005",
		TenantAImageEditAgentID: "20000000-0000-0000-0000-000000000006",
		UserAgentID:             "20000000-0000-0000-0000-000000000007",
		UserAgentOwnerUserID:    "user-a",
	}
	cleanup := func(ctx context.Context) error {
		pool.Close()
		return cleanupSchema(ctx)
	}
	return pool, fixture, cleanup, nil
}
