package billingledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type outboxItem struct {
	OutboxID          string
	BatchID           string
	WindowID          string
	LeaseID           string
	SettlementID      string
	ActualTenantMicro int64
	ActualUserMicro   int64
	AttemptCount      int
}

var errOutboxClaimLost = errors.New("billing ledger: outbox claim lost")

func (c *Coordinator) Run(ctx context.Context) {
	if c == nil || c.pool == nil || c.port == nil {
		return
	}
	c.logger.Info("billing ledger worker started")
	ticker := time.NewTicker(c.config.WorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("billing ledger worker stopped")
			return
		case <-ticker.C:
			c.runOnce(ctx)
		case <-c.trigger:
			c.runOnce(ctx)
		}
	}
}

func (c *Coordinator) runOnce(ctx context.Context) {
	if err := c.recoverOpeningWindows(ctx); err != nil {
		c.logger.Warn("recover billing openings failed", zapError(err))
	}
	if count, err := c.quarantineExpiredAdmissions(ctx); err != nil {
		c.logger.Warn("quarantine expired billing admissions failed", zapError(err))
	} else if count > 0 {
		c.logger.Error("billing admissions require reconciliation", zap.Int64("count", count))
	}
	if err := c.renewExpiringWindows(ctx); err != nil {
		c.logger.Warn("renew billing windows failed", zapError(err))
	}
	if err := c.rotateWindows(ctx); err != nil {
		c.logger.Warn("rotate billing windows failed", zapError(err))
	}
	if err := c.createSettlementBatches(ctx); err != nil {
		c.logger.Warn("create billing settlements failed", zapError(err))
	}
	for i := 0; i < c.config.PickLimit; i++ {
		item, err := c.claimOutbox(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			c.logger.Warn("claim billing outbox failed", zapError(err))
			break
		}
		c.dispatchOutbox(ctx, item)
	}
	if err := c.observeSnapshot(ctx); err != nil {
		c.logger.Warn("observe billing ledger state failed", zapError(err))
	}
}

func (c *Coordinator) quarantineExpiredAdmissions(ctx context.Context) (int64, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		WITH picked AS (
		  SELECT request_id
		  FROM ai_billing_request_admissions
		  WHERE status='active' AND request_expires_at <= $1
		  ORDER BY request_expires_at
		  FOR UPDATE SKIP LOCKED
		  LIMIT $2
		)
		UPDATE ai_billing_request_admissions a
		SET status='reconciling', updated_at=$1
		FROM picked p
		WHERE a.request_id=p.request_id
		RETURNING a.window_id
	`, c.now(), c.config.PickLimit)
	if err != nil {
		return 0, err
	}
	windowIDs := make(map[string]struct{})
	var count int64
	for rows.Next() {
		var windowID string
		if err := rows.Scan(&windowID); err != nil {
			rows.Close()
			return 0, err
		}
		windowIDs[windowID] = struct{}{}
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	now := c.now()
	for windowID := range windowIDs {
		if _, err := tx.Exec(ctx, `
			UPDATE ai_billing_windows
			SET state='reconciling',
			    last_error_code='admission_expired',
			    last_error_detail='request completion was not durably recorded before its deadline',
			    updated_at=$1
			WHERE window_id=$2 AND state IN ('active','draining')
		`, now, windowID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *Coordinator) recoverOpeningWindows(ctx context.Context) error {
	rows, err := c.pool.Query(ctx, `
		SELECT window_id FROM ai_billing_windows
		WHERE state='opening'
		ORDER BY updated_at
		LIMIT $1
	`, c.config.PickLimit)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		win, err := c.readWindow(ctx, id)
		if err != nil {
			continue
		}
		if err := c.activateWindow(ctx, win); err != nil {
			c.logger.Warn("activate billing window failed", zapString("window_id", id), zapError(err))
		}
	}
	return nil
}

func (c *Coordinator) renewExpiringWindows(ctx context.Context) error {
	rows, err := c.pool.Query(ctx, `
		SELECT window_id FROM ai_billing_windows
		WHERE expires_at <= $1
		  AND (
		    state='active'
		    OR (
		      state='draining'
		      AND EXISTS (
		        SELECT 1 FROM ai_billing_request_admissions a
		        WHERE a.window_id=ai_billing_windows.window_id AND a.status='active'
		      )
		    )
		  )
		ORDER BY expires_at
		LIMIT $2
	`, c.now().Add(c.config.RenewLead), c.config.PickLimit)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		win, err := c.readWindow(ctx, id)
		if err != nil {
			continue
		}
		if err := c.renewWindow(ctx, win); err != nil {
			if shouldDrainAfterRenewFailure(err, win, c.now(), c.config.AdmissionHeadroom) {
				_ = c.beginDrain(context.WithoutCancel(ctx), id, "lease_renew_failed", err.Error())
				continue
			}
			c.logger.Warn("credit lease renewal will retry while admission headroom remains",
				zapString("window_id", id), zapError(err))
		}
	}
	return nil
}

func shouldDrainAfterRenewFailure(err error, win *window, now time.Time, headroom time.Duration) bool {
	if win == nil || win.ExpiresAt == nil || !win.ExpiresAt.After(now.Add(headroom)) {
		return true
	}
	return errors.Is(err, ErrProtocolViolation) || errors.Is(err, ErrAdmissionConflict)
}

func (c *Coordinator) rotateWindows(ctx context.Context) error {
	now := c.now()
	_, err := c.pool.Exec(ctx, `
		UPDATE ai_billing_windows
		SET state='draining', updated_at=$1
		WHERE state='active' AND (
		  max_age_at <= $1
		  OR (granted_tenant_micro > 0 AND accrued_tenant_micro >= (granted_tenant_micro*3)/4)
		  OR (granted_user_micro > 0 AND accrued_user_micro >= (granted_user_micro*3)/4)
		)
	`, now)
	return err
}

func (c *Coordinator) createSettlementBatches(ctx context.Context) error {
	rows, err := c.pool.Query(ctx, `
		SELECT w.window_id
		FROM ai_billing_windows w
		WHERE w.state='draining'
		  AND NOT EXISTS (
		    SELECT 1 FROM ai_billing_request_admissions a
		    WHERE a.window_id=w.window_id AND a.status IN ('active','reconciling')
		  )
		ORDER BY w.updated_at
		LIMIT $1
	`, c.config.PickLimit)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := c.createSettlementBatch(ctx, id); err != nil && !errors.Is(err, ErrAdmissionConflict) {
			return err
		}
	}
	return nil
}

func (c *Coordinator) createSettlementBatch(ctx context.Context, windowID string) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	win, err := findWindowByID(ctx, tx, windowID, true)
	if err != nil {
		return err
	}
	if win.State != "draining" || win.LeaseID == "" {
		return ErrAdmissionConflict
	}
	var active int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_billing_request_admissions
		WHERE window_id=$1 AND status IN ('active','reconciling')
	`, windowID).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return ErrAdmissionConflict
	}
	var admissionTenantMicro, admissionUserMicro int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(actual_tenant_micro),0),
		       COALESCE(SUM(actual_user_micro),0)
		FROM ai_billing_request_admissions
		WHERE window_id=$1 AND status='completed'
	`, windowID).Scan(&admissionTenantMicro, &admissionUserMicro); err != nil {
		return err
	}
	if admissionTenantMicro != win.AccruedTenantMicro || admissionUserMicro != win.AccruedUserMicro {
		now := c.now()
		if _, err := tx.Exec(ctx, `
			UPDATE ai_billing_windows
			SET state='reconciling', last_error_code='aggregate_mismatch',
			    last_error_detail=$1, updated_at=$2
			WHERE window_id=$3 AND state='draining'
		`, fmt.Sprintf("window=(%d,%d) admissions=(%d,%d)",
			win.AccruedTenantMicro, win.AccruedUserMicro,
			admissionTenantMicro, admissionUserMicro), now, windowID); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return fmt.Errorf("%w: request totals do not match billing window", ErrProtocolViolation)
	}
	batchID := uuid.New()
	settlementID := "bs_" + batchID.String()
	payload, err := json.Marshal(SettleLease{
		SettlementID:      settlementID,
		ActualTenantMicro: win.AccruedTenantMicro,
		ActualUserMicro:   win.AccruedUserMicro,
	})
	if err != nil {
		return err
	}
	now := c.now()
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_billing_settlement_batches (
			batch_id, window_id, lease_id, settlement_id,
			actual_tenant_micro, actual_user_micro, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'pending',$7,$7)
	`, batchID, windowID, win.LeaseID, settlementID,
		win.AccruedTenantMicro, win.AccruedUserMicro, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_billing_settlement_outbox (
			batch_id, lease_id, settlement_id, payload, status, available_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4::jsonb,'pending',$5,$5,$5)
	`, batchID, win.LeaseID, settlementID, payload, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs SET settlement_batch_id=$1
		WHERE billing_window_id=$2 AND settlement_batch_id IS NULL
	`, batchID, windowID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE ai_billing_windows SET state='settlement_pending', updated_at=$1
		WHERE window_id=$2 AND state='draining'
	`, now, windowID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrAdmissionConflict
	}
	return tx.Commit(ctx)
}

func (c *Coordinator) claimOutbox(ctx context.Context) (*outboxItem, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var item outboxItem
	err = tx.QueryRow(ctx, `
		SELECT o.outbox_id::text, o.batch_id::text, b.window_id, o.lease_id,
		       o.settlement_id, b.actual_tenant_micro, b.actual_user_micro,
		       o.attempt_count
		FROM ai_billing_settlement_outbox o
		JOIN ai_billing_settlement_batches b ON b.batch_id=o.batch_id
		WHERE (
		  o.status='pending'
		  OR (o.status='processing' AND o.locked_until <= $1)
		) AND o.available_at <= $1
		ORDER BY o.created_at
		FOR UPDATE OF o SKIP LOCKED
		LIMIT 1
	`, c.now()).Scan(&item.OutboxID, &item.BatchID, &item.WindowID, &item.LeaseID,
		&item.SettlementID, &item.ActualTenantMicro, &item.ActualUserMicro, &item.AttemptCount)
	if err != nil {
		return nil, err
	}
	now := c.now()
	if _, err := tx.Exec(ctx, `
		UPDATE ai_billing_settlement_outbox
		SET status='processing', locked_until=$1, attempt_count=attempt_count+1, updated_at=$2
		WHERE outbox_id=$3::uuid
	`, now.Add(c.config.DispatchLease), now, item.OutboxID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_billing_settlement_batches
		SET attempt_count=attempt_count+1, updated_at=$1 WHERE batch_id=$2::uuid
	`, now, item.BatchID); err != nil {
		return nil, err
	}
	item.AttemptCount++
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Coordinator) dispatchOutbox(ctx context.Context, item *outboxItem) {
	reconciled := false
	res, err := c.port.SettleCreditLease(ctx, item.LeaseID, SettleLease{
		SettlementID:      item.SettlementID,
		ActualTenantMicro: item.ActualTenantMicro,
		ActualUserMicro:   item.ActualUserMicro,
	})
	if err != nil {
		if errors.Is(err, ErrAdmissionConflict) {
			if current, getErr := c.port.GetCreditLease(ctx, item.LeaseID); getErr == nil &&
				current.SettlementState == "settled" && current.SettlementID == item.SettlementID &&
				current.ActualTenantMicro != nil && *current.ActualTenantMicro == item.ActualTenantMicro &&
				current.ActualUserMicro != nil && *current.ActualUserMicro == item.ActualUserMicro {
				res = current
				err = nil
				reconciled = true
			}
		}
	}
	if err != nil {
		c.observeSettlement("billing_error")
		if retryErr := c.retryOutbox(context.WithoutCancel(ctx), item, err); retryErr != nil &&
			!errors.Is(retryErr, errOutboxClaimLost) {
			c.logger.Error("schedule billing settlement retry failed",
				zapString("settlement_id", item.SettlementID), zapError(retryErr))
		}
		return
	}
	if err := c.deliverOutbox(context.WithoutCancel(ctx), item, res); err != nil {
		if errors.Is(err, errOutboxClaimLost) {
			c.observeSettlement("stale_claim")
			return
		}
		c.observeSettlement("persistence_error")
		c.logger.Error("persist billing settlement receipt failed",
			zapString("settlement_id", item.SettlementID), zapError(err))
		if retryErr := c.retryOutbox(context.WithoutCancel(ctx), item, err); retryErr != nil &&
			!errors.Is(retryErr, errOutboxClaimLost) {
			c.logger.Error("schedule billing settlement persistence retry failed",
				zapString("settlement_id", item.SettlementID), zapError(retryErr))
		}
		return
	}
	if reconciled {
		c.observeSettlement("reconciled_after_conflict")
	} else {
		c.observeSettlement("success")
	}
}

func (c *Coordinator) observeSnapshot(ctx context.Context) error {
	if c.observer == nil {
		return nil
	}
	var snapshot BillingSnapshot
	if err := c.pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE state='opening'),
		  COUNT(*) FILTER (WHERE state='active'),
		  COUNT(*) FILTER (WHERE state='draining'),
		  COUNT(*) FILTER (WHERE state='reconciling'),
		  COUNT(*) FILTER (WHERE state='settlement_pending')
		FROM ai_billing_windows
	`).Scan(&snapshot.OpeningWindows, &snapshot.ActiveWindows,
		&snapshot.DrainingWindows, &snapshot.ReconcilingWindows,
		&snapshot.SettlementPending); err != nil {
		return err
	}
	if err := c.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COALESCE(EXTRACT(EPOCH FROM ($1-MIN(created_at))),0)
		FROM ai_billing_settlement_outbox
		WHERE status IN ('pending','processing')
	`, c.now()).Scan(&snapshot.PendingOutbox, &snapshot.OldestOutboxAgeSeconds); err != nil {
		return err
	}
	if err := c.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_billing_request_admissions
		WHERE status='reconciling'
	`).Scan(&snapshot.ReconcilingAdmissions); err != nil {
		return err
	}
	c.observer.SetBillingSnapshot(snapshot)
	return nil
}

func (c *Coordinator) deliverOutbox(ctx context.Context, item *outboxItem, res *CreditLease) error {
	if err := validateSettlementReceipt(item, res); err != nil {
		return err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := c.now()
	tag, err := tx.Exec(ctx, `
		UPDATE ai_billing_settlement_outbox
		SET status='delivered', locked_until=NULL, delivered_at=$1, updated_at=$1,
		    last_error_code=NULL, last_error_detail=NULL
		WHERE outbox_id=$2::uuid AND status='processing' AND attempt_count=$3
	`, now, item.OutboxID, item.AttemptCount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errOutboxClaimLost
	}
	tag, err = tx.Exec(ctx, `
		UPDATE ai_billing_settlement_batches
		SET status='delivered', billing_event_id=NULLIF($1,''),
		    tenant_deducted_micro=$2, user_deducted_micro=$3,
		    tenant_debt_added_micro=$4, user_debt_added_micro=$5,
		    delivered_at=$6, updated_at=$6,
		    last_error_code=NULL, last_error_detail=NULL
		WHERE batch_id=$7::uuid AND status='pending'
	`, res.SettledEventID, res.TenantDeductedMicro, res.UserDeductedMicro,
		res.TenantDebtAddedMicro, res.UserDebtAddedMicro, now, item.BatchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: settlement batch is no longer pending", ErrProtocolViolation)
	}
	tag, err = tx.Exec(ctx, `
		UPDATE ai_billing_windows
		SET state='settled', lease_version=$1, settled_at=$2, updated_at=$2
		WHERE window_id=$3 AND state='settlement_pending'
	`, res.Version, now, item.WindowID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: settlement window is no longer pending", ErrProtocolViolation)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs
		SET billing_status=CASE
		      WHEN tenant_payable=0 AND user_charged=0 THEN 'free'
		      ELSE 'settled'
		    END,
		    settled_event_id=NULLIF($1,''),
		    settled_at=$2
		WHERE settlement_batch_id=$3::uuid
		  AND billing_status='pending_settle'
	`, res.SettledEventID, now, item.BatchID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateSettlementReceipt(item *outboxItem, res *CreditLease) error {
	if item == nil || res == nil ||
		res.LeaseID != item.LeaseID ||
		res.EscrowState != "released" ||
		res.SettlementState != "settled" ||
		res.SettlementID != item.SettlementID ||
		res.ActualTenantMicro == nil || *res.ActualTenantMicro != item.ActualTenantMicro ||
		res.ActualUserMicro == nil || *res.ActualUserMicro != item.ActualUserMicro ||
		res.TenantDeductedMicro < 0 || res.TenantDebtAddedMicro < 0 ||
		res.UserDeductedMicro < 0 || res.UserDebtAddedMicro < 0 ||
		res.TenantDeductedMicro+res.TenantDebtAddedMicro != item.ActualTenantMicro ||
		res.UserDeductedMicro+res.UserDebtAddedMicro != item.ActualUserMicro ||
		res.Version <= 0 || res.SettledAt == nil {
		return fmt.Errorf("%w: invalid settlement receipt", ErrProtocolViolation)
	}
	return nil
}

func (c *Coordinator) retryOutbox(ctx context.Context, item *outboxItem, dispatchErr error) error {
	delay := time.Second * time.Duration(1<<min(item.AttemptCount, 8))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	code := portErrorCode(dispatchErr)
	now := c.now()
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE ai_billing_settlement_outbox
		SET status='pending', locked_until=NULL, available_at=$1,
		    last_error_code=$2, last_error_detail=$3, updated_at=$4
		WHERE outbox_id=$5::uuid AND status='processing' AND attempt_count=$6
	`, now.Add(delay), code, dispatchErr.Error(), now, item.OutboxID, item.AttemptCount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errOutboxClaimLost
	}
	tag, err = tx.Exec(ctx, `
		UPDATE ai_billing_settlement_batches
		SET last_error_code=$1, last_error_detail=$2, updated_at=$3
		WHERE batch_id=$4::uuid AND status='pending'
	`, code, dispatchErr.Error(), now, item.BatchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: settlement batch is no longer pending", ErrProtocolViolation)
	}
	return tx.Commit(ctx)
}

// Small logger helpers keep worker.go independent of zap field construction
// at each call site while preserving structured logs.
func zapError(err error) zap.Field          { return zap.Error(err) }
func zapString(key, value string) zap.Field { return zap.String(key, value) }
