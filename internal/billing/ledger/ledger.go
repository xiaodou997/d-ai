// Package ledger owns account balance. It is the only place in D-AI that may
// read or write bill_accounts.balance_micro.
//
// Balance is one signed integer per account. A negative balance is debt; there
// is no separate debt column, no credit limit, and no reservation state. That
// is the whole model, and it is what makes the three business rules total:
//
//	admission  balance > 0                 (ledger.Balances)
//	settlement balance -= cost, may go < 0  (ledger.Charge, never fails on funds)
//	top-up     balance += amount            (ledger.Grant, absorbs debt for free)
//
// Credit lots record where a grant came from and when it expires. They are an
// attribution detail settled behind the balance, never a second source of truth
// for it. Callers that need "how much money is there" call Balances and nothing
// else; reassembling a balance from lots is what this package exists to prevent.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Kind distinguishes the two account owners that can hold a balance.
type Kind int16

const (
	KindTenant Kind = 1
	KindUser   Kind = 2
)

func (k Kind) Valid() bool { return k == KindTenant || k == KindUser }

// Ref addresses one account. TenantID is required when provisioning a user
// account row and is ignored for lookups, which key on ID alone.
type Ref struct {
	Kind     Kind
	ID       string
	TenantID string
}

// ErrAccountNotFound is returned when an account has no bill_accounts row.
// Every tenant and end user gets one at creation time, so this means the
// caller addressed something that is not a billable account.
var ErrAccountNotFound = errors.New("billing account not found")

// ErrInsufficientBalance is returned only by ChargeIfFunded. Charge cannot
// produce it: recording what was already spent is not allowed to fail.
var ErrInsufficientBalance = errors.New("insufficient balance")

// Querier is the read surface shared by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Execer adds writes. pgx.Tx satisfies it; charging outside a transaction is
// intentionally not offered, because every balance move belongs with the record
// that justifies it.
type Execer interface {
	Querier
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ============================================================================
// Reads
// ============================================================================

// Balance returns one account's signed balance in micro-USD.
func Balance(ctx context.Context, q Querier, ref Ref) (int64, error) {
	balances, err := Balances(ctx, q, ref)
	if err != nil {
		return 0, err
	}
	balance, ok := balances[ref.ID]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrAccountNotFound, ref.ID)
	}
	return balance, nil
}

// Balances resolves several accounts in one round trip. The admission gate uses
// it to read the tenant and the end user together; display paths use the same
// function, so a page can never show a number the gate would disagree with.
//
// Accounts with no row are simply absent from the result. Callers that require
// presence must check, which keeps "unknown account" distinct from "zero".
func Balances(ctx context.Context, q Querier, refs ...Ref) (map[string]int64, error) {
	if len(refs) == 0 {
		return map[string]int64{}, nil
	}
	ids := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref.ID == "" || seen[ref.ID] {
			continue
		}
		seen[ref.ID] = true
		ids = append(ids, ref.ID)
	}
	if len(ids) == 0 {
		return map[string]int64{}, nil
	}

	rows, err := q.Query(ctx, `
		SELECT account_id, balance_micro
		FROM bill_accounts
		WHERE account_id = ANY($1)
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("read account balances: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(ids))
	for rows.Next() {
		var id string
		var balance int64
		if err := rows.Scan(&id, &balance); err != nil {
			return nil, fmt.Errorf("scan account balance: %w", err)
		}
		out[id] = balance
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read account balances: %w", err)
	}
	return out, nil
}

// ============================================================================
// Writes
// ============================================================================

// EnsureAccount creates the account row if it is missing. Tenant and end-user
// provisioning call it; it is idempotent so replaying a signup is harmless.
func EnsureAccount(ctx context.Context, tx Execer, ref Ref) error {
	if !ref.Kind.Valid() || ref.ID == "" {
		return fmt.Errorf("invalid account ref %+v", ref)
	}
	tenantID := ref.TenantID
	if ref.Kind == KindTenant {
		tenantID = ref.ID
	}
	if tenantID == "" {
		return fmt.Errorf("tenant id is required for account %s", ref.ID)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_accounts (account_id, account_kind, tenant_id, balance_micro)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (account_id) DO NOTHING
	`, ref.ID, int16(ref.Kind), tenantID); err != nil {
		return fmt.Errorf("ensure billing account %s: %w", ref.ID, err)
	}
	return nil
}

// Charge debits an account for work already performed.
//
// It never fails for lack of funds and never inspects account status: by the
// time a charge is raised the upstream cost has been incurred, and refusing to
// record it would lose real money rather than protect it. Admission is what
// stops an unfunded account, and it runs before the work, not after.
//
// The balance move is a single-row UPDATE, so concurrent charges against one
// tenant serialise on that row only for the instant of the write instead of
// holding a lock for the whole settlement.
func Charge(ctx context.Context, tx Execer, ref Ref, micro int64) error {
	if micro < 0 {
		return fmt.Errorf("charge amount must not be negative: %d", micro)
	}
	if micro == 0 {
		return nil
	}
	if ref.ID == "" {
		return fmt.Errorf("charge requires an account id")
	}
	tag, err := tx.Exec(ctx, `
		UPDATE bill_accounts
		SET balance_micro = balance_micro - $1, updated_at = now()
		WHERE account_id = $2
	`, micro, ref.ID)
	if err != nil {
		return fmt.Errorf("charge account %s: %w", ref.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAccountNotFound, ref.ID)
	}
	return consumeLots(ctx, tx, ref.ID, micro)
}

// ChargeIfFunded debits only when the balance covers the whole amount, and
// returns ErrInsufficientBalance otherwise without moving anything.
//
// This is for money spent up front — buying a subscription, withdrawing cash —
// where the platform is free to say no because nothing has been delivered yet.
// AI settlement must use Charge instead: by then the cost is already real, and
// refusing to record it destroys the receivable rather than preventing it.
//
// The balance test lives in the UPDATE's WHERE clause, so the check and the
// debit are one statement and concurrent callers cannot both pass it.
func ChargeIfFunded(ctx context.Context, tx Execer, ref Ref, micro int64) error {
	if micro < 0 {
		return fmt.Errorf("charge amount must not be negative: %d", micro)
	}
	if micro == 0 {
		return nil
	}
	if ref.ID == "" {
		return fmt.Errorf("charge requires an account id")
	}
	tag, err := tx.Exec(ctx, `
		UPDATE bill_accounts
		SET balance_micro = balance_micro - $1, updated_at = now()
		WHERE account_id = $2 AND balance_micro >= $1
	`, micro, ref.ID)
	if err != nil {
		return fmt.Errorf("charge account %s: %w", ref.ID, err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM bill_accounts WHERE account_id = $1)
		`, ref.ID).Scan(&exists); err != nil {
			return fmt.Errorf("inspect account %s: %w", ref.ID, err)
		}
		if !exists {
			return fmt.Errorf("%w: %s", ErrAccountNotFound, ref.ID)
		}
		return fmt.Errorf("%w: account %s", ErrInsufficientBalance, ref.ID)
	}
	return consumeLots(ctx, tx, ref.ID, micro)
}

// Grant credits an account and records the lot the money came from.
//
// A negative balance is absorbed by the same addition that funds the account —
// there is no separate "clear the debt first" step, because there is no
// separate debt. lotID is empty when the whole grant went to clearing debt.
func Grant(ctx context.Context, tx Execer, ref Ref, micro int64, expiresAt *time.Time, source, rechargeOrderID string) (lotID string, err error) {
	if micro <= 0 {
		return "", fmt.Errorf("grant amount must be positive: %d", micro)
	}
	if ref.ID == "" {
		return "", fmt.Errorf("grant requires an account id")
	}

	var balanceBefore int64
	if err := tx.QueryRow(ctx, `
		UPDATE bill_accounts
		SET balance_micro = balance_micro + $1, updated_at = now()
		WHERE account_id = $2
		RETURNING balance_micro - $1
	`, micro, ref.ID).Scan(&balanceBefore); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", ErrAccountNotFound, ref.ID)
		}
		return "", fmt.Errorf("grant to account %s: %w", ref.ID, err)
	}

	// The lot only carries the part that became spendable balance. Money that
	// went to clearing debt was already consumed by past usage and must not
	// reappear as a fresh spendable lot.
	lotMicro := micro
	if balanceBefore < 0 {
		lotMicro = micro + balanceBefore
	}
	if lotMicro <= 0 {
		return "", nil
	}

	lotID = "LOT_" + uuid.New().String()[:24]
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_credit_lots
		(lot_id, account_id, granted_micro, consumed_micro, expires_at, source, recharge_order_id)
		VALUES ($1, $2, $3, 0, $4, $5, NULLIF($6, ''))
	`, lotID, ref.ID, lotMicro, expiresAt, source, rechargeOrderID); err != nil {
		return "", fmt.Errorf("record credit lot for %s: %w", ref.ID, err)
	}
	return lotID, nil
}

// Revocation reports what a reversal was able to take back.
type Revocation struct {
	LotIDs         []string
	GrantedMicro   int64 // originally credited
	ReclaimedMicro int64 // still unspent and therefore recovered
}

// SpentMicro is the part of the grant that had already been consumed and so
// could not be reclaimed.
func (r Revocation) SpentMicro() int64 { return r.GrantedMicro - r.ReclaimedMicro }

// RevokeOrderLots reverses a recharge by reclaiming whatever its lots have not
// been spent on. Money already consumed stays consumed; the caller decides
// whether a partial reversal is acceptable.
func RevokeOrderLots(ctx context.Context, tx Execer, rechargeOrderID string) (Revocation, error) {
	var out Revocation
	if rechargeOrderID == "" {
		return out, fmt.Errorf("revoke requires a recharge order id")
	}
	rows, err := tx.Query(ctx, `
		SELECT lot_id, account_id, granted_micro, consumed_micro
		FROM bill_credit_lots
		WHERE recharge_order_id = $1 AND revoked_at IS NULL AND expired_at IS NULL
		FOR UPDATE
	`, rechargeOrderID)
	if err != nil {
		return out, fmt.Errorf("load lots for order %s: %w", rechargeOrderID, err)
	}
	type lotRow struct {
		lotID     string
		accountID string
		remaining int64
		granted   int64
	}
	var lots []lotRow
	for rows.Next() {
		var l lotRow
		var grantedMicro, consumedMicro int64
		if err := rows.Scan(&l.lotID, &l.accountID, &grantedMicro, &consumedMicro); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan lot for order %s: %w", rechargeOrderID, err)
		}
		l.granted = grantedMicro
		l.remaining = max(grantedMicro-consumedMicro, 0)
		lots = append(lots, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("load lots for order %s: %w", rechargeOrderID, err)
	}

	for _, l := range lots {
		out.LotIDs = append(out.LotIDs, l.lotID)
		out.GrantedMicro += l.granted
		if l.remaining > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE bill_accounts
				SET balance_micro = balance_micro - $1, updated_at = now()
				WHERE account_id = $2
			`, l.remaining, l.accountID); err != nil {
				return out, fmt.Errorf("reclaim balance from %s: %w", l.accountID, err)
			}
			out.ReclaimedMicro += l.remaining
		}
		if _, err := tx.Exec(ctx, `
			UPDATE bill_credit_lots
			SET revoked_at = now(), consumed_micro = granted_micro, updated_at = now()
			WHERE lot_id = $1
		`, l.lotID); err != nil {
			return out, fmt.Errorf("mark lot %s revoked: %w", l.lotID, err)
		}
	}
	return out, nil
}

// ExpireDueLots settles lots whose validity window has closed: the unspent
// remainder leaves the balance in one write and the lot is stamped.
//
// Expiry is an explicit deduction rather than a filter applied at read time.
// That is the point — it keeps balance a real number that only changes when
// something writes it, instead of one that quietly shrinks as the clock moves
// while debt does not, which is what made the two halves incomparable before.
//
// expired_at is the idempotency anchor, so re-running is a no-op.
func ExpireDueLots(ctx context.Context, tx Execer, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := tx.Query(ctx, `
		SELECT lot_id, account_id, granted_micro - consumed_micro
		FROM bill_credit_lots
		WHERE expired_at IS NULL
		  AND revoked_at IS NULL
		  AND expires_at IS NOT NULL
		  AND expires_at <= $1
		ORDER BY expires_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, now, limit)
	if err != nil {
		return 0, fmt.Errorf("load due lots: %w", err)
	}
	type dueLot struct {
		lotID     string
		accountID string
		remaining int64
	}
	var due []dueLot
	for rows.Next() {
		var l dueLot
		if err := rows.Scan(&l.lotID, &l.accountID, &l.remaining); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan due lot: %w", err)
		}
		due = append(due, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("load due lots: %w", err)
	}

	for _, l := range due {
		if l.remaining > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE bill_accounts
				SET balance_micro = balance_micro - $1, updated_at = now()
				WHERE account_id = $2
			`, l.remaining, l.accountID); err != nil {
				return 0, fmt.Errorf("expire balance for %s: %w", l.accountID, err)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE bill_credit_lots
			SET expired_at = $1, consumed_micro = granted_micro, updated_at = now()
			WHERE lot_id = $2
		`, now, l.lotID); err != nil {
			return 0, fmt.Errorf("mark lot %s expired: %w", l.lotID, err)
		}
	}
	return len(due), nil
}

// consumeLots attributes a charge to lots in FIFO order (soonest expiry first,
// permanent grants last). It is bookkeeping for the account statement, not a
// funds check: the balance has already moved, so a charge that outruns the
// remaining lots simply leaves the excess unattributed.
func consumeLots(ctx context.Context, tx Execer, accountID string, micro int64) error {
	rows, err := tx.Query(ctx, `
		SELECT lot_id, granted_micro - consumed_micro
		FROM bill_credit_lots
		WHERE account_id = $1
		  AND expired_at IS NULL
		  AND revoked_at IS NULL
		  AND consumed_micro < granted_micro
		ORDER BY expires_at NULLS LAST, created_at
		FOR UPDATE
	`, accountID)
	if err != nil {
		return fmt.Errorf("load lots for %s: %w", accountID, err)
	}
	type openLot struct {
		lotID     string
		remaining int64
	}
	var lots []openLot
	for rows.Next() {
		var l openLot
		if err := rows.Scan(&l.lotID, &l.remaining); err != nil {
			rows.Close()
			return fmt.Errorf("scan lot for %s: %w", accountID, err)
		}
		lots = append(lots, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load lots for %s: %w", accountID, err)
	}

	outstanding := micro
	for _, l := range lots {
		if outstanding <= 0 {
			break
		}
		applied := min(outstanding, l.remaining)
		if applied <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE bill_credit_lots
			SET consumed_micro = consumed_micro + $1, updated_at = now()
			WHERE lot_id = $2
		`, applied, l.lotID); err != nil {
			return fmt.Errorf("attribute charge to lot %s: %w", l.lotID, err)
		}
		outstanding -= applied
	}
	return nil
}
