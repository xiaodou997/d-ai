package testsupport

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/dbtest"
)

// AsyncTaskPoolOptions tunes the async task test pool.
type AsyncTaskPoolOptions struct {
	// MaxConns must exceed the number of concurrent claimers a test runs, or a
	// contention test silently becomes a serial one and proves nothing.
	MaxConns int32
}

// OpenAsyncTaskTestPool provisions an isolated schema loaded from the real
// internal/db/init.sql and returns a pool bound to it.
//
// It is a thin alias for dbtest.OpenIsolatedSchemaPool so the AI packages and
// the platform packages share one harness: two copies would drift, and the one
// nobody runs is the one that rots.
//
// Returns an error when no database is configured; callers skip rather than
// fail. Set DAI_TEST_DATABASE_STRICT to make an unreachable database a hard
// failure instead.
func OpenAsyncTaskTestPool(ctx context.Context, opts AsyncTaskPoolOptions) (*pgxpool.Pool, func(context.Context) error, error) {
	return dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: opts.MaxConns})
}
