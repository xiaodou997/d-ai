// Package invariants contains the database-level money invariant checker.
//
// It is deliberately read-only: production repair commands must not hide a
// discrepancy by changing rows while they inspect them. Tests and a future
// reconciliation job can run the same checker against a pool or transaction.
package invariants

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Querier is implemented by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Violation describes one row-level invariant failure. The checker returns
// all violations in one pass so an operator gets a useful reconciliation
// report instead of fixing one row per run.
type Violation struct {
	Invariant string
	Subject   string
	Detail    string
}

// Report is the result of one read-only pass over the billing schema. Callers
// that need a single MVCC snapshot should invoke Check with a pgx.Tx.
// Callers should treat a non-empty Violations slice as an operational failure.
type Report struct {
	InvariantsChecked    int
	AccountsChecked      int
	LotsChecked          int
	OutboxChecked        int
	OrdersChecked        int
	SubscriptionsChecked int
	Violations           []Violation
}

// Healthy reports whether every checked invariant held.
func (r Report) Healthy() bool { return len(r.Violations) == 0 }

// Err converts violations into a compact error suitable for a health check or
// reconciliation alert. A healthy report returns nil.
func (r Report) Err() error {
	if r.Healthy() {
		return nil
	}
	parts := make([]string, 0, len(r.Violations))
	for _, violation := range r.Violations {
		parts = append(parts, fmt.Sprintf("%s[%s]: %s", violation.Invariant, violation.Subject, violation.Detail))
	}
	return fmt.Errorf("billing invariant violations: %s", strings.Join(parts, "; "))
}

// Check performs one read-only invariant pass. It intentionally does not
// require a special database extension or a generated query package so it can
// be used against a production release database during reconciliation.
func Check(ctx context.Context, q Querier) (Report, error) {
	var report Report
	checks := []func(context.Context, Querier, *Report) error{
		checkAccountLotConservation,
		checkLotStates,
		checkRechargeOrders,
		checkOutboxLinkage,
		checkRefundEffects,
		checkSubscriptionOrders,
		checkSubscriptionQuotas,
	}
	for _, check := range checks {
		if err := check(ctx, q, &report); err != nil {
			return report, err
		}
		report.InvariantsChecked++
	}
	return report, nil
}

func checkAccountLotConservation(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT a.account_id, a.balance_micro,
		       COALESCE(SUM(GREATEST(l.granted_micro - l.consumed_micro, 0))
		           FILTER (WHERE l.expired_at IS NULL AND l.revoked_at IS NULL), 0)::bigint
		FROM bill_accounts a
		LEFT JOIN bill_credit_lots l ON l.account_id = a.account_id
		GROUP BY a.account_id, a.balance_micro
		ORDER BY a.account_id
	`)
	if err != nil {
		return fmt.Errorf("check account/lot conservation: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var accountID string
		var balance, availableLots int64
		if err := rows.Scan(&accountID, &balance, &availableLots); err != nil {
			return fmt.Errorf("scan account/lot conservation: %w", err)
		}
		report.AccountsChecked++
		if availableLots != max(balance, 0) {
			report.Violations = append(report.Violations, Violation{
				Invariant: "account_lot_conservation",
				Subject:   accountID,
				Detail:    fmt.Sprintf("available lots=%d, expected=%d from balance=%d", availableLots, max(balance, 0), balance),
			})
		}
	}
	return rows.Err()
}

func checkLotStates(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT lot_id, account_id, granted_micro, consumed_micro,
		       expired_at IS NOT NULL, expired_unused_micro, revoked_at IS NOT NULL
		FROM bill_credit_lots
		WHERE consumed_micro < 0
		   OR consumed_micro > granted_micro
		   OR (expired_at IS NULL AND expired_unused_micro IS NOT NULL)
		   OR (expired_at IS NOT NULL AND
		       (expired_unused_micro IS NULL OR expired_unused_micro <> granted_micro - consumed_micro))
		   OR (revoked_at IS NOT NULL AND consumed_micro <> granted_micro)
		ORDER BY lot_id
	`)
	if err != nil {
		return fmt.Errorf("check credit lot states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var lotID, accountID string
		var granted, consumed int64
		var expired bool
		var expiredUnused *int64
		var revoked bool
		if err := rows.Scan(&lotID, &accountID, &granted, &consumed, &expired, &expiredUnused, &revoked); err != nil {
			return fmt.Errorf("scan credit lot state: %w", err)
		}
		report.LotsChecked++
		report.Violations = append(report.Violations, Violation{
			Invariant: "credit_lot_state",
			Subject:   lotID,
			Detail:    fmt.Sprintf("account=%s granted=%d consumed=%d expired=%t expired_unused=%v revoked=%t", accountID, granted, consumed, expired, expiredUnused, revoked),
		})
	}
	return rows.Err()
}

func checkRechargeOrders(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT r.order_id, r.status, r.credit_amount,
		       COALESCE(SUM(l.granted_micro), 0)::bigint,
		       COUNT(l.id) FILTER (WHERE l.expired_at IS NULL AND l.revoked_at IS NULL)
		FROM bill_recharge_orders r
		LEFT JOIN bill_credit_lots l ON l.recharge_order_id = r.order_id
		GROUP BY r.order_id, r.status, r.credit_amount
		HAVING COALESCE(SUM(l.granted_micro), 0) > r.credit_amount
		    OR (r.status = 'reversed' AND COUNT(l.id) FILTER (WHERE l.expired_at IS NULL AND l.revoked_at IS NULL) > 0)
		ORDER BY r.order_id
	`)
	if err != nil {
		return fmt.Errorf("check recharge orders: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var orderID, status string
		var credit, granted, activeLots int64
		if err := rows.Scan(&orderID, &status, &credit, &granted, &activeLots); err != nil {
			return fmt.Errorf("scan recharge order: %w", err)
		}
		report.OrdersChecked++
		report.Violations = append(report.Violations, Violation{
			Invariant: "recharge_order_lots",
			Subject:   orderID,
			Detail:    fmt.Sprintf("status=%s credit=%d granted=%d active_lots=%d", status, credit, granted, activeLots),
		})
	}
	return rows.Err()
}

func checkOutboxLinkage(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT o.request_id, o.status, o.tenant_id, COALESCE(o.user_id, ''),
		       o.tenant_micro, o.user_micro,
		       COALESCE(u.tenant_payable, 0), COALESCE(u.user_charged, 0),
		       COALESCE(u.billing_status, '')
		FROM bill_charge_outbox o
		LEFT JOIN ai_usage_logs u ON u.request_id = o.request_id
		WHERE u.request_id IS NULL
		   OR o.tenant_micro < 0 OR o.user_micro < 0
		   OR (o.tenant_micro = 0 AND o.user_micro = 0)
		   OR (o.user_micro > 0 AND COALESCE(o.user_id, '') = '')
		   OR o.tenant_micro <> COALESCE(u.tenant_payable, 0)
		   OR o.user_micro <> COALESCE(u.user_charged, 0)
		   OR (o.status = 'done' AND u.billing_status <> 'settled')
		   OR (o.status = 'failed' AND u.billing_status <> 'failed')
		   OR (o.status = 'pending' AND u.billing_status <> 'pending')
		ORDER BY o.request_id
	`)
	if err != nil {
		return fmt.Errorf("check outbox linkage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var requestID, status, tenantID, userID, usageStatus string
		var tenantMicro, userMicro, usageTenant, usageUser int64
		if err := rows.Scan(&requestID, &status, &tenantID, &userID, &tenantMicro, &userMicro, &usageTenant, &usageUser, &usageStatus); err != nil {
			return fmt.Errorf("scan outbox linkage: %w", err)
		}
		report.OutboxChecked++
		report.Violations = append(report.Violations, Violation{
			Invariant: "outbox_usage_linkage",
			Subject:   requestID,
			Detail:    fmt.Sprintf("status=%s tenant=%s user=%s charge=(%d,%d) usage=(%d,%d,%s)", status, tenantID, userID, tenantMicro, userMicro, usageTenant, usageUser, usageStatus),
		})
	}
	return rows.Err()
}

func checkRefundEffects(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT reversal_id, refund_id, recharge_order_id, credit_amount_micro,
		       available_reclaimed_micro, non_available_debit_micro,
		       expired_amount_micro, account_debit_micro
		FROM bill_refund_reversal_effects
		WHERE available_reclaimed_micro + non_available_debit_micro + expired_amount_micro <> credit_amount_micro
		   OR available_reclaimed_micro + non_available_debit_micro <> account_debit_micro
		ORDER BY reversal_id
	`)
	if err != nil {
		return fmt.Errorf("check refund effects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var reversalID, refundID, orderID string
		var credit, available, nonAvailable, expired, accountDebit int64
		if err := rows.Scan(&reversalID, &refundID, &orderID, &credit, &available, &nonAvailable, &expired, &accountDebit); err != nil {
			return fmt.Errorf("scan refund effect: %w", err)
		}
		report.OrdersChecked++
		report.Violations = append(report.Violations, Violation{
			Invariant: "refund_effect_components",
			Subject:   reversalID,
			Detail:    fmt.Sprintf("refund=%s order=%s credit=%d available=%d non_available=%d expired=%d account_debit=%d", refundID, orderID, credit, available, nonAvailable, expired, accountDebit),
		})
	}
	return rows.Err()
}

func checkSubscriptionOrders(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT order_no, status, COALESCE(debit_reference, ''), paid_at
		FROM ai_sub_orders
		WHERE (status = 'paid' AND (debit_reference IS NULL OR paid_at IS NULL))
		   OR (status <> 'paid' AND paid_at IS NOT NULL)
		ORDER BY order_no
	`)
	if err != nil {
		return fmt.Errorf("check subscription orders: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var orderNo, status, debitReference string
		var paidAt any
		if err := rows.Scan(&orderNo, &status, &debitReference, &paidAt); err != nil {
			return fmt.Errorf("scan subscription order: %w", err)
		}
		report.SubscriptionsChecked++
		report.Violations = append(report.Violations, Violation{
			Invariant: "subscription_order_state",
			Subject:   orderNo,
			Detail:    fmt.Sprintf("status=%s debit_reference=%s paid_at=%v", status, debitReference, paidAt),
		})
	}
	return rows.Err()
}

func checkSubscriptionQuotas(ctx context.Context, q Querier, report *Report) error {
	rows, err := q.Query(ctx, `
		SELECT id::text, tenant_id, user_id, status,
		       total_used_micro, total_limit_micro,
		       win5h_used_micro, window_5h_limit_micro,
		       win7d_used_micro, window_7d_limit_micro
		FROM ai_sub_subscriptions
		WHERE total_used_micro > total_limit_micro
		   OR (window_5h_limit_micro IS NOT NULL AND win5h_used_micro > window_5h_limit_micro)
		   OR (window_7d_limit_micro IS NOT NULL AND win7d_used_micro > window_7d_limit_micro)
		ORDER BY id
	`)
	if err != nil {
		return fmt.Errorf("check subscription quotas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, tenantID, userID, status string
		var totalUsed, totalLimit, win5hUsed int64
		var win5hLimit, win7dUsed, win7dLimit *int64
		if err := rows.Scan(&id, &tenantID, &userID, &status, &totalUsed, &totalLimit, &win5hUsed, &win5hLimit, &win7dUsed, &win7dLimit); err != nil {
			return fmt.Errorf("scan subscription quota: %w", err)
		}
		report.SubscriptionsChecked++
		report.Violations = append(report.Violations, Violation{
			Invariant: "subscription_quota_bounds",
			Subject:   id,
			Detail:    fmt.Sprintf("tenant=%s user=%s status=%s total=%d/%d win5h=%d/%v win7d=%d/%v", tenantID, userID, status, totalUsed, totalLimit, win5hUsed, win5hLimit, win7dUsed, win7dLimit),
		})
	}
	return rows.Err()
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
