package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/dbtest"
)

func openLedgerPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 16})
	if err != nil {
		t.Skipf("ledger test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	return pool, ctx
}

// seedAccounts creates a tenant and one end user. The bill_accounts rows are
// created by the schema trigger, which this also asserts.
func seedAccounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (tenant, user ledger.Ref) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "t_" + suffix
	userID := "u_" + suffix

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status) VALUES ($1, $1, 'active')
	`, tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, $1, 'x', 4, 'active')
	`, userID, tenantID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	tenant = ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}
	user = ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}
	for _, ref := range []ledger.Ref{tenant, user} {
		if _, err := ledger.Balance(ctx, pool, ref); err != nil {
			t.Fatalf("account %s was not provisioned by the schema: %v", ref.ID, err)
		}
	}
	return tenant, user
}

func inTx(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return nil
}

func mustGrant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ref ledger.Ref, micro int64, expiresAt *time.Time, orderID string) {
	t.Helper()
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		_, err := ledger.Grant(ctx, tx, ref, micro, expiresAt, "ADMIN_RECHARGE", orderID)
		return err
	}); err != nil {
		t.Fatalf("grant %d to %s: %v", micro, ref.ID, err)
	}
}

func balanceOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ref ledger.Ref) int64 {
	t.Helper()
	balance, err := ledger.Balance(ctx, pool, ref)
	if err != nil {
		t.Fatalf("read balance of %s: %v", ref.ID, err)
	}
	return balance
}

// assertLotInvariant checks the property the whole design rests on: spendable
// balance and the sum of unspent lot remainders are two views of one number.
// If these ever disagree, a lot has become a second source of truth.
func assertLotInvariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ref ledger.Ref) {
	t.Helper()
	balance := balanceOf(t, ctx, pool, ref)
	var lotRemainder int64
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(granted_micro - consumed_micro), 0)
		FROM bill_credit_lots
		WHERE account_id = $1 AND expired_at IS NULL AND revoked_at IS NULL
	`, ref.ID).Scan(&lotRemainder); err != nil {
		t.Fatalf("read lot remainder: %v", err)
	}
	want := max(balance, 0)
	if lotRemainder != want {
		t.Fatalf("lot remainder %d != max(balance,0) %d for %s (balance=%d)", lotRemainder, want, ref.ID, balance)
	}
}

// The business rule, stated exactly: a $1 balance takes a $1.5 charge and lands
// at -$0.5. Settlement records what was already spent, so it never refuses.
func TestChargeGoesNegativeAndNeverFails(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	mustGrant(t, ctx, pool, user, 1_000_000, nil, "")

	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, user, 1_500_000)
	}); err != nil {
		t.Fatalf("settlement must not fail for lack of funds: %v", err)
	}

	if got := balanceOf(t, ctx, pool, user); got != -500_000 {
		t.Fatalf("balance = %d, want -500000", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// The other half of the rule: the overshoot is bounded by stopping the next
// request, which is the admission gate's job and is asserted here as the
// balance simply being readable as negative.
func TestChargedAccountReadsNegative(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	mustGrant(t, ctx, pool, user, 200_000, nil, "")
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, user, 250_000)
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	balances, err := ledger.Balances(ctx, pool, user)
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	if balances[user.ID] > 0 {
		t.Fatalf("balance = %d, want <= 0 so admission refuses", balances[user.ID])
	}
}

// Concurrency is where a two-column representation used to lose money. Fifty
// parallel charges must sum exactly.
func TestConcurrentChargesLoseNothing(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	mustGrant(t, ctx, pool, user, 1_000_000, nil, "")

	const n = 50
	const amount = int64(30_000)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			tx, err := pool.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback(ctx)
			if err := ledger.Charge(ctx, tx, user, amount); err != nil {
				errs <- err
				return
			}
			if err := tx.Commit(ctx); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent charge: %v", err)
	}

	want := 1_000_000 - n*amount
	if got := balanceOf(t, ctx, pool, user); got != want {
		t.Fatalf("balance = %d, want %d (lost or double-applied updates)", got, want)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// A top-up absorbs debt with the same addition that funds the account. The lot
// only carries what actually became spendable, or the cleared debt would
// reappear as money the user could spend twice.
func TestGrantAbsorbsDebtAndOnlyLotsTheSpendableRemainder(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, user, 500_000)
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	if got := balanceOf(t, ctx, pool, user); got != -500_000 {
		t.Fatalf("balance before top-up = %d, want -500000", got)
	}

	mustGrant(t, ctx, pool, user, 2_000_000, nil, "")

	if got := balanceOf(t, ctx, pool, user); got != 1_500_000 {
		t.Fatalf("balance after top-up = %d, want 1500000", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// A grant that only clears debt creates no spendable lot at all.
func TestGrantEntirelyConsumedByDebtCreatesNoLot(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, user, 900_000)
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	var lotID string
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		var err error
		lotID, err = ledger.Grant(ctx, tx, user, 400_000, nil, "ADMIN_RECHARGE", "")
		return err
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if lotID != "" {
		t.Fatalf("lot %q was created for a grant that only cleared debt", lotID)
	}
	if got := balanceOf(t, ctx, pool, user); got != -500_000 {
		t.Fatalf("balance = %d, want -500000", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// Pre-paid spending is the one case that may refuse, and refusing must move
// nothing at all.
func TestChargeIfFundedRefusesAndMovesNothing(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	mustGrant(t, ctx, pool, user, 100_000, nil, "")

	err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.ChargeIfFunded(ctx, tx, user, 100_001)
	})
	if !errors.Is(err, ledger.ErrInsufficientBalance) {
		t.Fatalf("error = %v, want ErrInsufficientBalance", err)
	}
	if got := balanceOf(t, ctx, pool, user); got != 100_000 {
		t.Fatalf("balance = %d, want untouched 100000", got)
	}

	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.ChargeIfFunded(ctx, tx, user, 100_000)
	}); err != nil {
		t.Fatalf("exact-balance charge should succeed: %v", err)
	}
	if got := balanceOf(t, ctx, pool, user); got != 0 {
		t.Fatalf("balance = %d, want 0", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// Expiry is a real deduction rather than a read-time filter, and repeating it
// must not deduct twice.
func TestExpireDueLotsDeductsOnceAndIsIdempotent(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)

	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)
	mustGrant(t, ctx, pool, user, 300_000, &past, "")
	mustGrant(t, ctx, pool, user, 700_000, &future, "")
	if got := balanceOf(t, ctx, pool, user); got != 1_000_000 {
		t.Fatalf("balance before expiry = %d, want 1000000", got)
	}

	settle := func() int {
		var settled int
		if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
			var err error
			settled, err = ledger.ExpireDueLots(ctx, tx, time.Now().UTC(), 100)
			return err
		}); err != nil {
			t.Fatalf("expire: %v", err)
		}
		return settled
	}

	if got := settle(); got != 1 {
		t.Fatalf("settled lots = %d, want 1", got)
	}
	if got := balanceOf(t, ctx, pool, user); got != 700_000 {
		t.Fatalf("balance after expiry = %d, want 700000", got)
	}
	if got := settle(); got != 0 {
		t.Fatalf("second expiry pass settled %d lots, want 0", got)
	}
	if got := balanceOf(t, ctx, pool, user); got != 700_000 {
		t.Fatalf("balance after repeated expiry = %d, want 700000", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// Expiry only removes what was left; money already spent out of the lot is not
// deducted a second time.
func TestExpireDueLotsOnlyReclaimsTheUnspentPart(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)

	past := time.Now().UTC().Add(-time.Hour)
	mustGrant(t, ctx, pool, user, 500_000, &past, "")
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, user, 200_000)
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}

	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		_, err := ledger.ExpireDueLots(ctx, tx, time.Now().UTC(), 100)
		return err
	}); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if got := balanceOf(t, ctx, pool, user); got != 0 {
		t.Fatalf("balance = %d, want 0 (300000 unspent reclaimed from 300000 left)", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// Reversing a top-up reclaims what is unspent and reports what was not.
func TestRevokeOrderLotsReclaimsOnlyUnspent(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	_, user := seedAccounts(t, ctx, pool)
	const orderID = "ORD_test_revoke"
	mustGrant(t, ctx, pool, user, 1_000_000, nil, orderID)
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, user, 400_000)
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}

	var rev ledger.Revocation
	if err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		var err error
		rev, err = ledger.RevokeOrderLots(ctx, tx, orderID)
		return err
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if rev.GrantedMicro != 1_000_000 || rev.ReclaimedMicro != 600_000 || rev.SpentMicro() != 400_000 {
		t.Fatalf("revocation = %+v, want granted=1000000 reclaimed=600000 spent=400000", rev)
	}
	if got := balanceOf(t, ctx, pool, user); got != 0 {
		t.Fatalf("balance = %d, want 0", got)
	}
	assertLotInvariant(t, ctx, pool, user)
}

// Balances answers for several accounts in one round trip so the admission gate
// cannot compare numbers read at different instants.
func TestBalancesReadsManyAccountsAtOnce(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	tenant, user := seedAccounts(t, ctx, pool)
	mustGrant(t, ctx, pool, tenant, 111, nil, "")
	mustGrant(t, ctx, pool, user, 222, nil, "")

	balances, err := ledger.Balances(ctx, pool, tenant, user, ledger.Ref{Kind: ledger.KindUser, ID: "missing"})
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	if balances[tenant.ID] != 111 || balances[user.ID] != 222 {
		t.Fatalf("balances = %#v", balances)
	}
	if _, present := balances["missing"]; present {
		t.Fatal("an unknown account must be absent, not reported as zero")
	}
}

func TestChargeUnknownAccountIsAnError(t *testing.T) {
	pool, ctx := openLedgerPool(t)
	err := inTx(t, ctx, pool, func(tx pgx.Tx) error {
		return ledger.Charge(ctx, tx, ledger.Ref{Kind: ledger.KindUser, ID: "nope"}, 1)
	})
	if !errors.Is(err, ledger.ErrAccountNotFound) {
		t.Fatalf("error = %v, want ErrAccountNotFound", err)
	}
}
