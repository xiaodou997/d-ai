package dbtest

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolOptions tunes an isolated-schema test pool.
type PoolOptions struct {
	// MaxConns must exceed the number of concurrent claimers a test runs, or a
	// contention test silently becomes a serial one and proves nothing. Set it
	// to 1 for tests built on TEMP tables, which are session-scoped.
	MaxConns int32
}

// OpenIsolatedSchemaPool provisions a throwaway PostgreSQL schema loaded from
// the real internal/db/init.sql and returns a pool bound to it.
//
// Loading the canonical schema rather than hand-copying DDL is the point: these
// tests then exercise the same CHECK constraints, partial indexes and triggers
// a deployment gets, and a schema change that breaks them fails here instead of
// in production.
//
// When DAI_TEST_DATABASE_URL is set it is the only DSN tried. Silently falling
// through to some other local database is how a suite ends up green against
// nothing. Set DAI_TEST_DATABASE_STRICT to turn an unreachable database into a
// hard failure rather than a skip; CI sets it so these tests cannot go quiet.
func OpenIsolatedSchemaPool(ctx context.Context, opts PoolOptions) (*pgxpool.Pool, func(context.Context) error, error) {
	if opts.MaxConns <= 0 {
		opts.MaxConns = 4
	}

	schemaSQL, err := loadCanonicalSchema()
	if err != nil {
		return nil, nil, err
	}

	dsns := defaultDSNs
	if dsn := os.Getenv("DAI_TEST_DATABASE_URL"); dsn != "" {
		dsns = []string{dsn}
	}

	var lastErr error
	for _, dsn := range dsns {
		pool, cleanup, err := openWithRetry(ctx, dsn, schemaSQL, opts)
		if err == nil {
			return pool, cleanup, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no database url configured")
	}
	return nil, nil, RequireDatabase(lastErr)
}

// RequireDatabase converts an unreachable-database error into a panic when
// DAI_TEST_DATABASE_STRICT is set. Without the variable it returns the error
// unchanged and callers keep skipping, which is what a local run without
// Docker wants.
func RequireDatabase(err error) error {
	if err == nil || os.Getenv("DAI_TEST_DATABASE_STRICT") == "" {
		return err
	}
	panic(fmt.Sprintf("DAI_TEST_DATABASE_STRICT is set but the test database is unusable: %v", err))
}

var defaultDSNs = []string{
	"postgres://postgres:postgres@127.0.0.1:15432/dai_test?sslmode=disable",
	"postgres://postgres:postgres@127.0.0.1:5432/dai_test?sslmode=disable",
}

// openWithRetry rides out transient connection-limit exhaustion, which parallel
// packages each opening a pool can otherwise hit against a small local server.
func openWithRetry(ctx context.Context, dsn, schemaSQL string, opts PoolOptions) (*pgxpool.Pool, func(context.Context) error, error) {
	var lastErr error
	backoff := 100 * time.Millisecond
	for attempt := range 4 {
		pool, cleanup, err := openWithDSN(ctx, dsn, schemaSQL, opts)
		if err == nil {
			return pool, cleanup, nil
		}
		lastErr = err
		if attempt == 3 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, nil, lastErr
}

func openWithDSN(ctx context.Context, dsn, schemaSQL string, opts PoolOptions) (*pgxpool.Pool, func(context.Context) error, error) {
	schema := fmt.Sprintf("dai_test_%d", time.Now().UnixNano())

	bootstrapCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, nil, err
	}
	bootstrapCfg.MaxConns = 1
	bootstrapPool, err := pgxpool.NewWithConfig(ctx, bootstrapCfg)
	if err != nil {
		return nil, nil, err
	}
	defer bootstrapPool.Close()
	if err := bootstrapPool.Ping(ctx); err != nil {
		return nil, nil, err
	}

	cleanupSchema := func(ctx context.Context) error {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return err
		}
		cfg.MaxConns = 1
		p, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return err
		}
		defer p.Close()
		_, err = p.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		return err
	}

	if _, err := bootstrapPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		return nil, nil, err
	}
	// init.sql has no psql meta-commands and no qualified references, so setting
	// search_path is enough to relocate the whole canonical schema. Exec with no
	// arguments goes over the simple protocol, which allows multiple statements.
	if _, err := bootstrapPool.Exec(ctx, `SET search_path TO `+schema+`;
`+schemaSQL); err != nil {
		_ = cleanupSchema(ctx)
		return nil, nil, fmt.Errorf("load canonical schema into %s: %w", schema, err)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		_ = cleanupSchema(ctx)
		return nil, nil, err
	}
	cfg.MaxConns = opts.MaxConns
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET search_path TO `+schema)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		_ = cleanupSchema(ctx)
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = cleanupSchema(ctx)
		return nil, nil, err
	}

	cleanup := func(ctx context.Context) error {
		pool.Close()
		return cleanupSchema(ctx)
	}
	return pool, cleanup, nil
}
