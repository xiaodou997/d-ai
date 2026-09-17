package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/billing/ledger"
)

func (s *RequestStore) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	run, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	if err := s.ensureRecordPartitions(run); err != nil {
		s.logger.Error("initial record partition maintenance", zap.Error(err))
	}
	go func() {
		defer close(s.done)
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		recovery := time.NewTicker(time.Minute)
		defer recovery.Stop()
		maintenance := time.NewTicker(time.Hour)
		defer maintenance.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-tick.C:
				if _, err := s.DrainSettlements(run, 100); err != nil && run.Err() == nil {
					s.logger.Error("settlement worker", zap.Error(err))
				}
			case <-recovery.C:
				if err := s.RecoverRequests(run); err != nil {
					s.logger.Error("recover request leases", zap.Error(err))
				}
			case <-maintenance.C:
				if err := s.MaintainRecords(run); err != nil {
					s.logger.Error("record maintenance", zap.Error(err))
				}
			}
		}
	}()
}
func (s *RequestStore) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type settlementWork struct {
	at                                        time.Time
	id, tenant, user, key, sub, source, state string
	tenantDue, userDue, keyDue, subDue        int64
	windows                                   []byte
}

func scanSettlement(row pgx.Row) (w settlementWork, err error) {
	err = row.Scan(&w.at, &w.id, &w.tenant, &w.user, &w.key, &w.sub, &w.source, &w.state, &w.tenantDue, &w.userDue, &w.keyDue, &w.subDue, &w.windows)
	return
}

const settlementWorkColumns = `created_at,request_id,tenant_id,user_id,api_key_id,subscription_id,billing_source,state,tenant_due,user_due,key_due,subscription_due,subscription_windows`

func (s *RequestStore) DrainSettlements(ctx context.Context, limit int) (int, error) {
	for i := 0; i < limit; i++ {
		tx, err := s.financialPool.Begin(ctx)
		if err != nil {
			return i, err
		}
		w, err := scanSettlement(tx.QueryRow(ctx, `SELECT `+settlementWorkColumns+` FROM bill_settlements WHERE state='pending' AND available_at<=now() ORDER BY created_at,request_id FOR UPDATE SKIP LOCKED LIMIT 1`))
		if errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return i, nil
		}
		if err != nil {
			tx.Rollback(ctx)
			return i, err
		}
		if err = s.postSettlement(ctx, tx, w); err != nil {
			tx.Rollback(ctx)
			_, markErr := s.financialPool.Exec(ctx, `UPDATE bill_settlements SET attempts=attempts+1,last_error=$3,available_at=now()+make_interval(secs=>LEAST(60,power(2,LEAST(attempts,6))::int)),state=CASE WHEN attempts>=9 THEN 'review' ELSE 'pending' END WHERE created_at=$1 AND request_id=$2 AND state='pending'`, w.at, w.id, err.Error())
			if markErr != nil {
				return i, markErr
			}
			continue
		}
		if err = tx.Commit(ctx); err != nil {
			return i, err
		}
		if s.invalidator != nil && w.key != "" {
			if err = s.invalidator.DelByID(ctx, w.key); err != nil {
				s.logger.Warn("invalidate settled key", zap.Error(err))
			}
		}
	}
	return limit, nil
}
func billingOperation(ctx context.Context, tx pgx.Tx, id, reason string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('dai.billing_operation',$1,true),set_config('dai.billing_reason',$2,true)`, id, reason)
	return err
}
func lockSettlementAccounts(ctx context.Context, tx pgx.Tx, w settlementWork) error {
	ids := []string{w.tenant}
	if w.user != "" {
		ids = append(ids, w.user)
	}
	rows, err := tx.Query(ctx, `SELECT account_id FROM bill_accounts WHERE account_id=ANY($1) ORDER BY account_id FOR UPDATE`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if n != len(ids) {
		return errors.New("settlement account missing")
	}
	return nil
}

func (s *RequestStore) postSettlement(ctx context.Context, tx pgx.Tx, w settlementWork) error {
	if err := lockSettlementAccounts(ctx, tx, w); err != nil {
		return err
	}
	if err := billingOperation(ctx, tx, w.id, "usage_charge"); err != nil {
		return err
	}
	for _, payer := range []struct {
		id     string
		kind   ledger.Kind
		amount int64
	}{{w.tenant, ledger.KindTenant, w.tenantDue}, {w.user, ledger.KindUser, w.userDue}} {
		if payer.amount > 0 {
			if err := ledger.Charge(ctx, tx, ledger.Ref{ID: payer.id, Kind: payer.kind, TenantID: w.tenant}, payer.amount); err != nil {
				return err
			}
		}
	}
	if w.keyDue > 0 {
		var before, after int64
		err := tx.QueryRow(ctx, `UPDATE ai_api_keys SET quota_used=quota_used+$2,updated_at=now() WHERE id::text=$1 RETURNING quota_used-$2,quota_used`, w.key, w.keyDue).Scan(&before, &after)
		if err != nil {
			return err
		}
	}
	if w.subDue > 0 {
		var before [3]int64
		if err := tx.QueryRow(ctx, `SELECT total_used_micro,win5h_used_micro,win7d_used_micro FROM ai_sub_subscriptions WHERE id::text=$1 FOR UPDATE`, w.sub).Scan(&before[0], &before[1], &before[2]); err != nil {
			return err
		}
		if _, err := dbgen.New(tx).DebitSubscription(ctx, dbgen.DebitSubscriptionParams{ID: mustParseUUID(w.sub), Win5hUsedMicro: w.subDue}); err != nil {
			return err
		}
		var after [3]int64
		var windows []byte
		if err := tx.QueryRow(ctx, `SELECT total_used_micro,win5h_used_micro,win7d_used_micro,jsonb_build_object('win5h_start',win5h_start,'win7d_start',win7d_start,'win5h_debit',CASE WHEN window_5h_limit_micro IS NULL THEN 0 ELSE $2::bigint END,'win7d_debit',CASE WHEN window_7d_limit_micro IS NULL THEN 0 ELSE $2::bigint END) FROM ai_sub_subscriptions WHERE id::text=$1`, w.sub, w.subDue).Scan(&after[0], &after[1], &after[2], &windows); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bill_settlements SET subscription_windows=$3 WHERE created_at=$1 AND request_id=$2`, w.at, w.id, windows); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE bill_settlements SET state='posted',posted_at=now(),tenant_charged=tenant_due,user_charged=user_due,last_error='' WHERE created_at=$1 AND request_id=$2`, w.at, w.id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE bill_daily_totals SET tenant_charged=tenant_charged+$4,user_charged=user_charged+$5 WHERE day=($1::timestamptz AT TIME ZONE 'UTC')::date AND tenant_id=$2 AND user_id=$3`, w.at, w.tenant, w.user, w.tenantDue, w.userDue); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE ai_async_tasks SET caller_charge=GREATEST(caller_charge,$2) WHERE request_id=$1`, w.id, func() int64 {
		if w.user != "" {
			return w.userDue
		}
		return w.tenantDue
	}())
	return err
}

// Refund reverses actual debits, independently of request-log retention.
func (s *RequestStore) Refund(ctx context.Context, id, reason, operator string) error {
	if reason == "" || operator == "" {
		return errors.New("refund requires reason and operator")
	}
	tx, err := s.financialPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	w, err := scanSettlement(tx.QueryRow(ctx, `SELECT `+settlementWorkColumns+` FROM bill_settlements WHERE request_id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if w.state == "refunded" {
		return nil
	}
	if w.at.Before(time.Now().AddDate(0, 0, -365)) {
		return errors.New("settlement exceeds individual refund retention")
	}
	if w.state != "posted" {
		return fmt.Errorf("settlement %s is not refundable", w.state)
	}
	if err = lockSettlementAccounts(ctx, tx, w); err != nil {
		return err
	}
	if err = billingOperation(ctx, tx, "refund:"+id, "usage_refund"); err != nil {
		return err
	}
	for _, p := range []struct {
		id     string
		kind   ledger.Kind
		amount int64
	}{{w.tenant, ledger.KindTenant, w.tenantDue}, {w.user, ledger.KindUser, w.userDue}} {
		if p.amount > 0 {
			if _, err = ledger.Grant(ctx, tx, ledger.Ref{ID: p.id, Kind: p.kind, TenantID: w.tenant}, p.amount, nil, "REFUND", ""); err != nil {
				return err
			}
		}
	}
	if w.keyDue > 0 {
		var before, after int64
		err = tx.QueryRow(ctx, `UPDATE ai_api_keys SET quota_used=quota_used-$2,updated_at=now() WHERE id::text=$1 AND quota_used>=$2 RETURNING quota_used,quota_used`, w.key, w.keyDue).Scan(&before, &after)
		if err != nil {
			return err
		}
	}
	if w.subDue > 0 {
		var before, after [3]int64
		if err = tx.QueryRow(ctx, `SELECT total_used_micro,win5h_used_micro,win7d_used_micro FROM ai_sub_subscriptions WHERE id::text=$1 FOR UPDATE`, w.sub).Scan(&before[0], &before[1], &before[2]); err != nil {
			return err
		}
		var windows map[string]json.RawMessage
		if err = json.Unmarshal(w.windows, &windows); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `UPDATE ai_sub_subscriptions SET total_used_micro=total_used_micro-$2,
   win5h_used_micro=CASE WHEN win5h_start=($3::jsonb->>'win5h_start')::timestamptz AND now()<win5h_start+interval '5 hours' THEN GREATEST(0,win5h_used_micro-COALESCE(($3::jsonb->>'win5h_debit')::bigint,0)) ELSE win5h_used_micro END,
   win7d_used_micro=CASE WHEN win7d_start=($3::jsonb->>'win7d_start')::timestamptz AND now()<win7d_start+interval '7 days' THEN GREATEST(0,win7d_used_micro-COALESCE(($3::jsonb->>'win7d_debit')::bigint,0)) ELSE win7d_used_micro END,updated_at=now()
   WHERE id::text=$1 AND total_used_micro>=$2 RETURNING total_used_micro,win5h_used_micro,win7d_used_micro`, w.sub, w.subDue, w.windows).Scan(&after[0], &after[1], &after[2])
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE bill_settlements SET state='refunded',refunded_at=now(),refund_reason=$3,refund_operator=$4 WHERE created_at=$1 AND request_id=$2`, w.at, id, reason, operator); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE bill_daily_totals SET tenant_refunded=tenant_refunded+$4,user_refunded=user_refunded+$5 WHERE day=($1::timestamptz AT TIME ZONE 'UTC')::date AND tenant_id=$2 AND user_id=$3`, w.at, w.tenant, w.user, w.tenantDue, w.userDue); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if s.invalidator != nil && w.key != "" {
		if err = s.invalidator.DelByID(context.WithoutCancel(ctx), w.key); err != nil {
			s.logger.Warn("invalidate refunded key", zap.Error(err))
		}
	}
	return nil
}
