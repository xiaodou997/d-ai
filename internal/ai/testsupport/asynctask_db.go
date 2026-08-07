package testsupport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AsyncTaskPoolOptions tunes the async task test pool.
type AsyncTaskPoolOptions struct {
	// MaxConns must exceed the number of concurrent claimers a test runs, or a
	// contention test silently becomes a serial one and proves nothing.
	MaxConns int32
}

// OpenAsyncTaskTestPool provisions an isolated schema loaded from the real
// db/init.sql and returns a pool bound to it.
//
// It loads the canonical schema rather than hand-copying the DDL, so these
// tests exercise the same table definition a deployment gets, including the
// stable CHECK constraints and partial indexes that a hand-copy tends to omit.
//
// Returns an error when no database is reachable; callers skip rather than fail,
// matching the other database-backed test harnesses.
func OpenAsyncTaskTestPool(ctx context.Context, opts AsyncTaskPoolOptions) (*pgxpool.Pool, func(context.Context) error, error) {
	if opts.MaxConns <= 0 {
		opts.MaxConns = 8
	}

	schemaSQL, err := loadCanonicalSchema()
	if err != nil {
		return nil, nil, err
	}

	dsns := []string{}
	if dsn := os.Getenv("DAI_TEST_DATABASE_URL"); dsn != "" {
		dsns = append(dsns, dsn)
	}
	dsns = append(dsns, defaultDSNs...)

	var lastErr error
	for _, dsn := range dsns {
		pool, cleanup, err := openAsyncTaskWithDSN(ctx, dsn, schemaSQL, opts)
		if err == nil {
			return pool, cleanup, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no database url configured")
	}
	return nil, nil, lastErr
}

var defaultDSNs = []string{
	"postgres://postgres:postgres@127.0.0.1:15432/dai_test?sslmode=disable",
	"postgres://postgres:postgres@127.0.0.1:5432/dai_test?sslmode=disable",
}

func openAsyncTaskWithDSN(ctx context.Context, dsn, schemaSQL string, opts AsyncTaskPoolOptions) (*pgxpool.Pool, func(context.Context) error, error) {
	schema := fmt.Sprintf("asynctask_test_%d", time.Now().UnixNano())

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

// loadCanonicalSchema reads internal/db/init.sql, found by walking up from the working
// directory so any package's tests can call this regardless of their depth.
func loadCanonicalSchema() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "internal", "db", "init.sql")
		if _, err := os.Stat(candidate); err == nil {
			raw, err := os.ReadFile(candidate)
			if err != nil {
				return "", err
			}
			return string(raw), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate internal/db/init.sql above the working directory")
		}
		dir = parent
	}
}
