// Package ledger implements the Phase 2 分账层 (local credit aggregation
// ledger). It accumulates each request's micro-credit consumption in a
// per-(owner, tenant, user) row of ai_user_credit_ledger, then periodically
// settles the integer-credit portion against URM via the single-stage Consume
// API, keeping sub-credit remainders in the local ledger for the next window.
//
// This eliminates two pre-existing problems:
//   - URM RPC explosion: every request used to call Freeze + Confirm; now we
//     aggregate per user over a 30~60s window (configurable per deployment).
//   - Floor-to-1 inflation: cheap models on short prompts no longer round 0.03
//     credit up to 1 credit at deduction time; remainders stay accurate in
//     micro-credit precision until they aggregate into a real integer credit.
package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/urm"
)

// Key identifies one ledger row (one aggregation account).
// For tenant-owned API keys, UserID is the empty string.
type Key struct {
	OwnerType domain.OwnerType
	TenantID  string
	UserID    string
}

// AddChargeParams describes a single request's accrued cost in micro-credits.
// For tenant-owned keys, UserMicro should be 0 and UserID empty.
type AddChargeParams struct {
	RequestID   string
	OwnerType   domain.OwnerType
	TenantID    string
	UserID      string
	TenantMicro int64
	UserMicro   int64
}

// SettleResult summarizes what one settle round actually pushed to URM.
type SettleResult struct {
	EventID             string
	TenantCredits       int64 // integer credits actually deducted via URM
	UserCredits         int64
	TenantLeftoverMicro int64 // remainder kept in the ledger for next window
	UserLeftoverMicro   int64
	// NoOp = true when pending was below 1 credit and nothing was sent to URM.
	NoOp bool
}

// ConsumeClient is the subset of urm.Client the settler uses; lets tests inject
// a fake implementation without touching the network.
type ConsumeClient interface {
	Consume(ctx context.Context, req urm.ConsumeRequest) (*urm.ConsumeResponse, error)
}

// Ledger is the concrete implementation, backed by PostgreSQL and a URM client.
type Ledger struct {
	pool   *pgxpool.Pool
	urm    ConsumeClient
	logger *zap.Logger
}

// New constructs a Ledger. logger may be nil (a no-op logger is used).
func New(pool *pgxpool.Pool, client ConsumeClient, logger *zap.Logger) *Ledger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Ledger{pool: pool, urm: client, logger: logger}
}

// ============================================================================
// AddCharge — accrue a single request's micro-credit cost
// ============================================================================

// AddCharge writes the request's tenant + user side cost into the ledger row.
// Atomic at the row level via UPSERT; safe under concurrent callers.
// Zero amounts on both sides are a no-op (no row created).
func (l *Ledger) AddCharge(ctx context.Context, p AddChargeParams) error {
	if p.TenantMicro < 0 || p.UserMicro < 0 {
		return fmt.Errorf("ledger: negative micro amount")
	}
	if p.TenantMicro == 0 && p.UserMicro == 0 {
		return nil
	}
	if p.OwnerType != domain.OwnerTenant && p.OwnerType != domain.OwnerUser {
		return fmt.Errorf("ledger: invalid owner type %q", p.OwnerType)
	}
	if p.OwnerType == domain.OwnerTenant && p.UserMicro != 0 {
		return fmt.Errorf("ledger: tenant-owned key must not carry user cost")
	}

	if _, err := l.pool.Exec(ctx, `
		INSERT INTO ai_user_credit_ledger
		  (owner_type, tenant_id, user_id, pending_tenant_micro, pending_user_micro, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (owner_type, tenant_id, user_id) DO UPDATE
		SET pending_tenant_micro = ai_user_credit_ledger.pending_tenant_micro + EXCLUDED.pending_tenant_micro,
		    pending_user_micro   = ai_user_credit_ledger.pending_user_micro   + EXCLUDED.pending_user_micro,
		    updated_at           = NOW()
	`, string(p.OwnerType), p.TenantID, p.UserID, p.TenantMicro, p.UserMicro); err != nil {
		return fmt.Errorf("ledger: upsert pending micro: %w", err)
	}
	return nil
}

// ============================================================================
// PickPending — scan for accounts due to be settled (SKIP LOCKED, multi-instance)
// ============================================================================

// PickOptions controls which ledger rows are eligible for settlement.
type PickOptions struct {
	// MinTotalMicro: rows whose pending_tenant_micro+pending_user_micro >= MinTotalMicro
	// are eligible immediately. 0 means "any nonzero pending".
	MinTotalMicro int64
	// MaxIdleAge: rows whose last_settled_at is older than now-MaxIdleAge are
	// eligible even if below MinTotalMicro. Zero disables age-based pick.
	MaxIdleAge time.Duration
	// Limit caps the batch size per call to avoid one worker grabbing everything.
	Limit int
}

// PickPending returns up to Limit keys that should be settled, each backed by
// a SELECT FOR UPDATE SKIP LOCKED row lock inside the returned transaction.
//
// The caller OWNS the returned tx — it must call SettleOne(tx, key) for each
// key (or skip), then tx.Commit/tx.Rollback. Holding the tx open blocks other
// workers from re-picking the same rows.
func (l *Ledger) PickPending(ctx context.Context, opts PickOptions) (pgx.Tx, []Key, error) {
	if opts.Limit <= 0 {
		opts.Limit = 32
	}
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("ledger: begin pick tx: %w", err)
	}

	cutoffApplies := opts.MaxIdleAge > 0
	cutoff := time.Now().Add(-opts.MaxIdleAge)
	rows, err := tx.Query(ctx, `
		SELECT owner_type, tenant_id, user_id
		FROM ai_user_credit_ledger
		WHERE (pending_tenant_micro + pending_user_micro) > 0
		  AND settle_window_id IS NULL
		  AND (
		    (pending_tenant_micro + pending_user_micro) >= $1
		    OR ($2::boolean AND (last_settled_at IS NULL OR last_settled_at < $3))
		  )
		ORDER BY last_settled_at NULLS FIRST, (pending_tenant_micro + pending_user_micro) DESC
		LIMIT $4
		FOR UPDATE SKIP LOCKED
	`, opts.MinTotalMicro, cutoffApplies, cutoff, opts.Limit)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("ledger: select for update: %w", err)
	}
	defer rows.Close()

	var keys []Key
	for rows.Next() {
		var owner string
		var k Key
		if err := rows.Scan(&owner, &k.TenantID, &k.UserID); err != nil {
			_ = tx.Rollback(ctx)
			return nil, nil, fmt.Errorf("ledger: scan key: %w", err)
		}
		k.OwnerType = domain.OwnerType(owner)
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("ledger: iterate keys: %w", err)
	}
	return tx, keys, nil
}

// ============================================================================
// SettleOne — floor pending_micro to integer credits, push to URM Consume
// ============================================================================

// SettleOne settles one ledger row inside the caller-supplied transaction.
// Caller is expected to have picked this key via PickPending (FOR UPDATE).
//
// Behaviour:
//  1. Read pending_tenant_micro / pending_user_micro for the row.
//  2. Compute integer credits per side via floor(micro / 10000). If both sides
//     floor to 0, only touch last_settled_at and return NoOp=true (caller can
//     still commit the tx to push this row to the back of the queue).
//  3. Persist a settle_window_id ULID on the row plus the locked-in window
//     micro amounts so a crash before URM responds is idempotent on retry.
//  4. Call URM Consume with idempotencyKey = "ai-settle-" + settle_window_id.
//  5. On success: subtract window_micro from pending_*_micro, accrue into
//     settled_*_micro, clear the window fields, refresh last_settled_at.
//
// All DB writes happen inside the passed-in tx. The URM HTTP call happens
// while the row lock is held — keep PickOptions.Limit small so a single worker
// doesn't hold many locks across many slow HTTP roundtrips.
func (l *Ledger) SettleOne(ctx context.Context, tx pgx.Tx, k Key) (*SettleResult, error) {
	var pendingTenant, pendingUser int64
	var windowID *string
	var windowTenant, windowUser int64
	if err := tx.QueryRow(ctx, `
		SELECT pending_tenant_micro, pending_user_micro,
		       settle_window_id, settle_window_tenant_micro, settle_window_user_micro
		FROM ai_user_credit_ledger
		WHERE owner_type = $1 AND tenant_id = $2 AND user_id = $3
	`, string(k.OwnerType), k.TenantID, k.UserID).Scan(
		&pendingTenant, &pendingUser, &windowID, &windowTenant, &windowUser,
	); err != nil {
		return nil, fmt.Errorf("ledger: read ledger row: %w", err)
	}

	// Decide how much to settle this round (pure logic — see settle_calc.go).
	reuseWindow := windowID != nil && *windowID != ""
	var reuseT, reuseU int64
	if reuseWindow {
		reuseT, reuseU = windowTenant, windowUser
	}
	action := computeSettleAction(pendingTenant, pendingUser, reuseT, reuseU)
	settleTenantMicro := action.SettleTenantMicro
	settleUserMicro := action.SettleUserMicro
	tenantCredits := action.TenantCredits
	userCredits := action.UserCredits

	if action.NoOp {
		// Below 1 credit on both sides — keep the remainder, just bump cursor.
		if _, err := tx.Exec(ctx, `
			UPDATE ai_user_credit_ledger
			SET last_settled_at = NOW(), updated_at = NOW()
			WHERE owner_type = $1 AND tenant_id = $2 AND user_id = $3
		`, string(k.OwnerType), k.TenantID, k.UserID); err != nil {
			return nil, fmt.Errorf("ledger: touch last_settled_at: %w", err)
		}
		return &SettleResult{
			NoOp:                true,
			TenantLeftoverMicro: pendingTenant,
			UserLeftoverMicro:   pendingUser,
		}, nil
	}

	// Persist the settle window if we are opening a new one. This row write is
	// inside the caller's tx but committed only when caller commits — that's
	// fine because the row lock is held until then; no other worker can pick.
	wid := ""
	if reuseWindow {
		wid = *windowID
	} else {
		wid = "win_" + uuid.New().String()[:24]
		if _, err := tx.Exec(ctx, `
			UPDATE ai_user_credit_ledger
			SET settle_window_id           = $1,
			    settle_window_tenant_micro = $2,
			    settle_window_user_micro   = $3,
			    settle_window_opened_at    = NOW(),
			    updated_at                 = NOW()
			WHERE owner_type = $4 AND tenant_id = $5 AND user_id = $6
		`, wid, settleTenantMicro, settleUserMicro,
			string(k.OwnerType), k.TenantID, k.UserID); err != nil {
			return nil, fmt.Errorf("ledger: open settle window: %w", err)
		}
	}

	// URM expects integer credits, not micro. User-owned keys send both fields;
	// tenant-owned keys send only tenantAmount (userCredits is always 0).
	consumeReq := urm.ConsumeRequest{
		IdempotencyKey: "ai-settle-" + wid,
		TenantID:       k.TenantID,
		UserID:         k.UserID,
		Description:    fmt.Sprintf("ai-gateway 聚合扣款 owner=%s window=%s", k.OwnerType, wid),
		TenantAmount:   tenantCredits,
		UserAmount:     userCredits,
	}
	resp, err := l.urm.Consume(ctx, consumeReq)
	if err != nil {
		// Don't clear the window on error — next worker will retry under the
		// same window_id, URM Consume's idempotencyKey dedups so no double-charge.
		return nil, fmt.Errorf("ledger: urm consume: %w", err)
	}

	// Success: subtract settled amounts, accrue, clear window.
	if _, err := tx.Exec(ctx, `
		UPDATE ai_user_credit_ledger
		SET pending_tenant_micro       = pending_tenant_micro - $1,
		    pending_user_micro         = pending_user_micro   - $2,
		    settled_tenant_micro       = settled_tenant_micro + $1,
		    settled_user_micro         = settled_user_micro   + $2,
		    settle_window_id           = NULL,
		    settle_window_tenant_micro = 0,
		    settle_window_user_micro   = 0,
		    settle_window_opened_at    = NULL,
		    last_settled_at            = NOW(),
		    updated_at                 = NOW()
		WHERE owner_type = $3 AND tenant_id = $4 AND user_id = $5
	`, settleTenantMicro, settleUserMicro,
		string(k.OwnerType), k.TenantID, k.UserID); err != nil {
		return nil, fmt.Errorf("ledger: finalize settle: %w", err)
	}

	leftoverTenant := pendingTenant - settleTenantMicro
	leftoverUser := pendingUser - settleUserMicro

	l.logger.Info("ledger settle succeeded",
		zap.String("event_id", resp.EventID),
		zap.String("owner_type", string(k.OwnerType)),
		zap.String("tenant_id", k.TenantID),
		zap.String("user_id", k.UserID),
		zap.Int64("tenant_credits", tenantCredits),
		zap.Int64("user_credits", userCredits),
		zap.Int64("tenant_leftover_micro", leftoverTenant),
		zap.Int64("user_leftover_micro", leftoverUser),
	)

	return &SettleResult{
		EventID:             resp.EventID,
		TenantCredits:       tenantCredits,
		UserCredits:         userCredits,
		TenantLeftoverMicro: leftoverTenant,
		UserLeftoverMicro:   leftoverUser,
	}, nil
}
