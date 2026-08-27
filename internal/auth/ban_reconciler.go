package auth

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/lifecycle"
)

// BanReconciler periodically re-syncs the Redis ban keys (dai:banned:user:*,
// dai:banned:tenant:*) against Postgres ground truth (iam_accounts and
// iam_tenants). Redis is treated as the fast-path source of
// truth for BanUser/BanTenant/UnbanUser/UnbanTenant, but it has no built-in
// recovery from data loss (Redis restart without AOF, a wrong snapshot
// restore, an accidental FLUSHALL, a missed write during a network blip).
// This job is the backstop: within one reconcile interval, any drift in
// either direction (a ban that should exist but doesn't, or a stale ban that
// should have been cleared) self-heals without operator intervention.
type BanReconciler struct {
	pool         *pgxpool.Pool
	redis        *redis.Client
	logger       *zap.Logger
	interval     time.Duration
	stopChan     chan struct{}
	lifecycleMu  sync.Mutex
	stopMu       sync.Mutex
	started      bool
	stopped      bool
	stopClosed   bool
	workerCtx    context.Context
	workerCancel context.CancelFunc
	wg           sync.WaitGroup
}

var _ lifecycle.Component = (*BanReconciler)(nil)

// NewBanReconciler constructs a reconciler. interval <= 0 defaults to 5 minutes.
func NewBanReconciler(pool *pgxpool.Pool, redisClient *redis.Client, logger *zap.Logger, interval time.Duration) *BanReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BanReconciler{pool: pool, redis: redisClient, logger: logger, interval: interval, stopChan: make(chan struct{})}
}

// Start launches the periodic reconcile loop in a background goroutine.
// No-op if redis is nil (ban enforcement itself is disabled in that case).
func (r *BanReconciler) Start(ctxs ...context.Context) {
	if r == nil || r.redis == nil || r.pool == nil {
		return
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	r.lifecycleMu.Lock()
	if r.started || r.stopped {
		r.lifecycleMu.Unlock()
		return
	}
	r.started = true
	r.workerCtx, r.workerCancel = context.WithCancel(ctx)
	workerCtx := r.workerCtx
	r.wg.Add(1)
	r.lifecycleMu.Unlock()
	r.logger.Info("ban reconciler started", zap.Duration("interval", r.interval))
	go func() {
		defer r.wg.Done()
		r.run(workerCtx)
	}()
}

// Stop signals the reconcile loop to exit and waits using the caller's
// deadline. A later call can continue waiting after an earlier deadline.
func (r *BanReconciler) Stop(ctxs ...context.Context) error {
	if r == nil {
		return nil
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	r.stopMu.Lock()
	defer r.stopMu.Unlock()

	r.lifecycleMu.Lock()
	if !r.stopped {
		r.stopped = true
	}
	started := r.started
	cancel := r.workerCancel
	stopChan := r.stopChan
	firstStop := false
	if started && !r.stopClosed {
		r.stopClosed = true
		firstStop = true
	}
	r.lifecycleMu.Unlock()
	if !started {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	if firstStop && stopChan != nil {
		// stopClosed is set while holding lifecycleMu; stopMu serializes the
		// actual close with retrying Stop calls.
		close(stopChan)
	}
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Health returns a lock-safe lifecycle snapshot for management probes.
func (r *BanReconciler) Health() lifecycle.HealthSnapshot {
	if r == nil {
		return lifecycle.HealthSnapshot{}
	}
	r.lifecycleMu.Lock()
	started, stopped := r.started, r.stopped
	r.lifecycleMu.Unlock()
	return lifecycle.HealthSnapshot{Started: started, Stopped: stopped}
}

func (r *BanReconciler) run(ctx context.Context) {
	// Reconcile once immediately so a fresh deploy/restart doesn't wait a
	// full interval before Redis matches Postgres.
	if err := r.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
		r.logger.Warn("ban reconcile failed", zap.Error(err))
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
				r.logger.Warn("ban reconcile failed", zap.Error(err))
			}
		}
	}
}

// ReconcileOnce runs a single reconcile pass. Exported for tests and for an
// optional startup call (so a fresh deploy doesn't wait a full interval
// before Redis matches Postgres).
//
// Order matters: Redis is scanned FIRST, Postgres truth is queried SECOND.
// BanUser/BanTenant always commit the Postgres status change before writing
// the Redis key (see BlacklistService), so querying truth after the Redis
// scan guarantees truth is at least as fresh as whatever was in Redis at
// scan time. Querying Postgres first (the reverse order) would open a race:
// a ban that commits to Postgres and writes its Redis key entirely between
// the truth query and the Redis scan would look like a stale/orphaned key
// (present in Redis, absent from the earlier truth snapshot) and get
// immediately deleted, un-banning an account moments after it was banned.
func (r *BanReconciler) ReconcileOnce(ctx context.Context) error {
	redisUsers, err := r.scanIDs(ctx, banUserPrefix+"*", len(banUserPrefix))
	if err != nil {
		return err
	}
	redisTenants, err := r.scanIDs(ctx, banTenantPrefix+"*", len(banTenantPrefix))
	if err != nil {
		return err
	}

	truthUsers, err := r.trueBannedUsers(ctx)
	if err != nil {
		return err
	}
	truthTenants, err := r.trueBannedTenants(ctx)
	if err != nil {
		return err
	}

	added, removed := 0, 0
	a, rm, err := r.reconcileSet(ctx, banUserPrefix, truthUsers, redisUsers)
	if err != nil {
		return err
	}
	added += a
	removed += rm

	a, rm, err = r.reconcileSet(ctx, banTenantPrefix, truthTenants, redisTenants)
	if err != nil {
		return err
	}
	added += a
	removed += rm

	if added > 0 || removed > 0 {
		r.logger.Warn("ban reconcile corrected drift",
			zap.Int("keys_restored", added),
			zap.Int("keys_cleared", removed),
		)
	}
	return nil
}

// reconcileSet sets missing keys (truth says banned, Redis doesn't have the
// key) and deletes stale keys (Redis has the key, truth says active).
func (r *BanReconciler) reconcileSet(ctx context.Context, prefix string, truth, current map[string]struct{}) (added, removed int, err error) {
	for id := range truth {
		if _, ok := current[id]; !ok {
			if err := r.redis.Set(ctx, prefix+id, "1", 0).Err(); err != nil {
				return added, removed, err
			}
			added++
		}
	}
	for id := range current {
		if _, ok := truth[id]; !ok {
			if err := r.redis.Del(ctx, prefix+id).Err(); err != nil {
				return added, removed, err
			}
			removed++
		}
	}
	return added, removed, nil
}

func (r *BanReconciler) trueBannedUsers(ctx context.Context) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	rows, err := r.pool.Query(ctx, `
		SELECT user_id FROM iam_accounts
		WHERE status IN ('disabled', 'inherited_disabled', 'deleted')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (r *BanReconciler) trueBannedTenants(ctx context.Context) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	rows, err := r.pool.Query(ctx, `SELECT tenant_id FROM iam_tenants WHERE status = 'disabled'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// scanIDs returns the set of IDs currently present in Redis under keys
// matching pattern, stripping the first prefixLen characters of each key.
// Uses SCAN (cursor-based, non-blocking) rather than KEYS.
func (r *BanReconciler) scanIDs(ctx context.Context, pattern string, prefixLen int) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	var cursor uint64
	for {
		keys, next, err := r.redis.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if len(k) > prefixLen {
				out[k[prefixLen:]] = struct{}{}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return out, nil
}
