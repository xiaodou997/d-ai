package billingledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/urm"
)

const (
	// LegacyUsageRecoveryStream contains V1 completion envelopes whose
	// authorization semantics can only be processed by a V2 AI binary.
	LegacyUsageRecoveryStream = "ai:usage:completion:v1"
	// UsageRecoveryStream contains V3-compatible completion envelopes.
	UsageRecoveryStream = "ai:usage:completion:v2"
	// UsageRecoveryQuarantineStream durably holds malformed V3 envelopes for
	// operator inspection without allowing them to starve the live stream.
	UsageRecoveryQuarantineStream = "ai:usage:completion:v2:quarantine"
)

type LegacyAuthorizationPort interface {
	SettleLegacyAuthorization(context.Context, string, urm.LegacyAuthorizationSettlementRequest) (*urm.LegacyAuthorizationSettlementResponse, error)
}

type LegacyReconciler struct {
	pool   *pgxpool.Pool
	port   LegacyAuthorizationPort
	logger *zap.Logger
}

type LegacyReconcileItem struct {
	OwnerType       string `json:"owner_type"`
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id,omitempty"`
	WindowReference string `json:"window_reference,omitempty"`
	AuthorizationID string `json:"authorization_id,omitempty"`
	TenantMicro     int64  `json:"tenant_micro"`
	UserMicro       int64  `json:"user_micro"`
	Status          string `json:"status"`
	Detail          string `json:"detail,omitempty"`
}

type LegacyReconcileReport struct {
	DryRun                     bool                  `json:"dry_run"`
	Resolved                   int                   `json:"resolved"`
	Unresolved                 int                   `json:"unresolved"`
	LegacyUsageRecoveryEntries int64                 `json:"legacy_usage_recovery_entries"`
	LegacyLedgerDropped        bool                  `json:"legacy_ledger_dropped"`
	Items                      []LegacyReconcileItem `json:"items"`
}

func NewLegacyReconciler(pool *pgxpool.Pool, port LegacyAuthorizationPort, logger *zap.Logger) *LegacyReconciler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LegacyReconciler{pool: pool, port: port, logger: logger}
}

// EnsureLegacyDrained prevents a V3 binary from silently ignoring money still
// owned by the V2 local ledger.
func EnsureLegacyDrained(ctx context.Context, pool *pgxpool.Pool) error {
	exists, err := legacyLedgerExists(ctx, pool)
	if err != nil {
		return fmt.Errorf("inspect legacy billing ledger table: %w", err)
	}
	if !exists {
		return nil
	}
	var count int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_user_credit_ledger
		WHERE settle_window_id IS NOT NULL
		   OR pending_tenant_micro > 0
		   OR pending_user_micro > 0
	`).Scan(&count); err != nil {
		return fmt.Errorf("inspect legacy billing ledger: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("legacy billing ledger contains %d unresolved row(s); run billing-v3-reconcile before starting V3", count)
	}
	return nil
}

// LegacyUsageRecoveryEntries returns the number of V1 recovery envelopes that
// must be replayed by the old AI binary before a V3 cutover.
func LegacyUsageRecoveryEntries(ctx context.Context, client *redis.Client) (int64, error) {
	if client == nil {
		return 0, errors.New("redis is required to inspect the legacy usage recovery stream")
	}
	count, err := client.XLen(ctx, LegacyUsageRecoveryStream).Result()
	if err != nil {
		return 0, fmt.Errorf("inspect legacy usage recovery stream: %w", err)
	}
	return count, nil
}

// EnsureLegacyUsageRecoveryDrained prevents V3 from silently abandoning V1
// completion envelopes. Those envelopes must be replayed by a V2 binary
// because they reference the legacy authorization/local-ledger protocol.
func EnsureLegacyUsageRecoveryDrained(ctx context.Context, client *redis.Client) error {
	count, err := LegacyUsageRecoveryEntries(ctx, client)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf(
			"legacy usage recovery stream %q contains %d unresolved message(s); drain it with the V2 AI recovery worker before starting V3",
			LegacyUsageRecoveryStream,
			count,
		)
	}
	return nil
}

// DropLegacyLedger removes the retired V2 AI-local aggregate after every
// financial row has been reconciled. It is deliberately an explicit
// post-cutover operation because dropping the table closes the V2 rollback
// path unless the database backup is restored.
func DropLegacyLedger(ctx context.Context, pool *pgxpool.Pool) error {
	if err := EnsureLegacyDrained(ctx, pool); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS ai_user_credit_ledger`); err != nil {
		return fmt.Errorf("drop legacy billing ledger: %w", err)
	}
	return nil
}

// Reconcile inventories every V2 row first. Apply mode settles an authorization
// through URM and only then clears the matching local window in one transaction.
func (r *LegacyReconciler) Reconcile(ctx context.Context, apply bool) (*LegacyReconcileReport, error) {
	items, err := r.inspect(ctx)
	if err != nil {
		return nil, err
	}
	report := &LegacyReconcileReport{DryRun: !apply, Items: items}
	for i := range report.Items {
		item := &report.Items[i]
		if item.AuthorizationID == "" {
			item.Status = "quarantined"
			item.Detail = "legacy row has pending value or provisional window without an authorization id"
			report.Unresolved++
			continue
		}
		if !apply {
			item.Status = "ready"
			report.Unresolved++
			continue
		}
		if r.port == nil {
			return nil, errors.New("legacy reconciliation port is unavailable")
		}
		receipt, err := r.port.SettleLegacyAuthorization(ctx, item.AuthorizationID, urm.LegacyAuthorizationSettlementRequest{
			ActualTenantMicro: item.TenantMicro,
			ActualUserMicro:   item.UserMicro,
		})
		if err != nil {
			item.Status = "quarantined"
			item.Detail = err.Error()
			report.Unresolved++
			continue
		}
		if !validLegacySettlementReceipt(item, receipt) {
			item.Status = "quarantined"
			item.Detail = "URM returned an invalid legacy settlement receipt"
			report.Unresolved++
			continue
		}
		if err := r.finalize(ctx, *item, receipt.AuthorizationID); err != nil {
			item.Status = "quarantined"
			item.Detail = err.Error()
			report.Unresolved++
			continue
		}
		item.Status = "resolved"
		report.Resolved++
	}
	return report, nil
}

func validLegacySettlementReceipt(item *LegacyReconcileItem, receipt *urm.LegacyAuthorizationSettlementResponse) bool {
	if item == nil || receipt == nil || receipt.AuthorizationID != item.AuthorizationID ||
		receipt.TenantDeductedMicro < 0 || receipt.TenantDebtAddedMicro < 0 ||
		receipt.UserDeductedMicro < 0 || receipt.UserDebtAddedMicro < 0 ||
		receipt.TenantDeductedMicro > item.TenantMicro ||
		receipt.UserDeductedMicro > item.UserMicro {
		return false
	}
	return receipt.TenantDebtAddedMicro == item.TenantMicro-receipt.TenantDeductedMicro &&
		receipt.UserDebtAddedMicro == item.UserMicro-receipt.UserDeductedMicro
}

func (r *LegacyReconciler) inspect(ctx context.Context) ([]LegacyReconcileItem, error) {
	exists, err := legacyLedgerExists(ctx, r.pool)
	if err != nil || !exists {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT owner_type, tenant_id, user_id, COALESCE(settle_window_id, '')
		FROM ai_user_credit_ledger
		WHERE settle_window_id IS NOT NULL
		   OR pending_tenant_micro > 0
		   OR pending_user_micro > 0
		ORDER BY owner_type, tenant_id, user_id
	`)
	if err != nil {
		return nil, err
	}
	var items []LegacyReconcileItem
	for rows.Next() {
		var item LegacyReconcileItem
		if err := rows.Scan(&item.OwnerType, &item.TenantID, &item.UserID, &item.WindowReference); err != nil {
			rows.Close()
			return nil, err
		}
		_, item.AuthorizationID, _ = strings.Cut(item.WindowReference, "::")
		item.Status = "inspected"
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].AuthorizationID == "" {
			continue
		}
		if err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(tenant_payable),0), COALESCE(SUM(user_charged),0)
			FROM ai_usage_logs
			WHERE urm_transaction_id=$1
			  AND billing_status='pending_settle'
			  AND settled_event_id IS NULL
		`, items[i].AuthorizationID).Scan(&items[i].TenantMicro, &items[i].UserMicro); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func legacyLedgerExists(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT to_regclass('ai_user_credit_ledger') IS NOT NULL`,
	).Scan(&exists)
	return exists, err
}

func (r *LegacyReconciler) finalize(ctx context.Context, item LegacyReconcileItem, eventID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current *string
	if err := tx.QueryRow(ctx, `
		SELECT settle_window_id FROM ai_user_credit_ledger
		WHERE owner_type=$1 AND tenant_id=$2 AND user_id=$3
		FOR UPDATE
	`, item.OwnerType, item.TenantID, item.UserID).Scan(&current); err != nil {
		return err
	}
	if current == nil || *current != item.WindowReference {
		return errors.New("legacy window changed during reconciliation")
	}
	now := time.Now()
	if _, err := tx.Exec(ctx, `
		UPDATE ai_user_credit_ledger
		SET pending_tenant_micro=GREATEST(pending_tenant_micro-$1,0),
		    pending_user_micro=GREATEST(pending_user_micro-$2,0),
		    settled_tenant_micro=settled_tenant_micro+$1,
		    settled_user_micro=settled_user_micro+$2,
		    settle_window_id=NULL,
		    settle_window_tenant_micro=0,
		    settle_window_user_micro=0,
		    settle_window_opened_at=NULL,
		    last_settled_at=$3,
		    updated_at=$3
		WHERE owner_type=$4 AND tenant_id=$5 AND user_id=$6
	`, item.TenantMicro, item.UserMicro, now, item.OwnerType, item.TenantID, item.UserID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs
		SET settled_event_id=$1, settled_at=$2, billing_status='settled'
		WHERE urm_transaction_id=$3
		  AND billing_status='pending_settle'
		  AND settled_event_id IS NULL
	`, eventID, now, item.AuthorizationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
