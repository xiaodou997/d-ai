package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecoverRequests never charges observations left by a dead runtime. Existing
// sealed work remains independently retryable in bill_settlements.
func (s *RequestStore) RecoverRequests(ctx context.Context) error {
	tx, err := s.financialPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT request_id,registered_at,tenant_id,user_id FROM ai_request_keys WHERE sealed_at IS NULL AND lease_until<now() FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return err
	}
	type lost struct {
		id, tenant, user string
		at               time.Time
	}
	var all []lost
	for rows.Next() {
		var x lost
		if err = rows.Scan(&x.id, &x.at, &x.tenant, &x.user); err != nil {
			rows.Close()
			return err
		}
		all = append(all, x)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, x := range all {
		if _, err = tx.Exec(ctx, `UPDATE ai_request_keys SET sealed_at=now() WHERE request_id=$1`, x.id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE ai_requests SET end_reason='gateway_interrupted',is_error=true,completed_at=now() WHERE created_at=$1 AND request_id=$2`, x.at, x.id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO ai_request_errors(created_at,request_id,origin,stage,code,message) VALUES($1,$2,'gateway','execute','runtime_lost','执行进程中断，未计费') ON CONFLICT DO NOTHING`, x.at, x.id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO bill_settlements(created_at,request_id,tenant_id,user_id,state,reason) VALUES($1,$2,$3,$4,'waived','unconfirmed_execution') ON CONFLICT DO NOTHING`, x.at, x.id, x.tenant, x.user); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO bill_daily_totals(day,tenant_id,user_id,requests,errors) VALUES(($1::timestamptz AT TIME ZONE 'UTC')::date,$2,$3,1,1) ON CONFLICT(day,tenant_id,user_id) DO UPDATE SET requests=bill_daily_totals.requests+1,errors=bill_daily_totals.errors+1`, x.at, x.tenant, x.user); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *RequestStore) MaintainRecords(ctx context.Context) error {
	if err := s.ensureRecordPartitions(ctx); err != nil {
		return err
	}
	if err := s.RecoverRequests(ctx); err != nil {
		return err
	}
	tx, err := s.financialPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(82624042)`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	now := time.Now().UTC()
	financial := monthStart(now.AddDate(0, 0, -365))
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bill_settlements WHERE created_at<$1 AND state IN ('pending','review')) OR EXISTS(SELECT 1 FROM ai_request_keys WHERE registered_at<$1 AND sealed_at IS NULL)`, financial).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return fmt.Errorf("financial retention blocked by unresolved work")
	}
	if err = checkpointJournal(ctx, tx, financial); err != nil {
		return err
	}
	policies := map[string]time.Time{"ai_requests": monthStart(now.AddDate(0, 0, -90)), "bill_settlements": financial, "bill_journal": financial,
		"ai_request_attempts": dayStart(now.AddDate(0, 0, -30)), "ai_request_errors": dayStart(now.AddDate(0, 0, -30)), "ai_request_debug_payloads": dayStart(now.AddDate(0, 0, -7))}
	rows, err := tx.Query(ctx, `SELECT parent.relname,child.relname FROM pg_inherits JOIN pg_class parent ON parent.oid=inhparent JOIN pg_class child ON child.oid=inhrelid JOIN pg_namespace n ON n.oid=parent.relnamespace WHERE n.nspname=current_schema()`)
	if err != nil {
		return err
	}
	var drop []string
	for rows.Next() {
		var base, name string
		if err = rows.Scan(&base, &name); err != nil {
			rows.Close()
			return err
		}
		cutoff, ok := policies[base]
		if !ok {
			continue
		}
		suffix := strings.TrimPrefix(name, base+"_")
		if !regexp.MustCompile(`^[0-9]{6}([0-9]{2})?$`).MatchString(suffix) {
			continue
		}
		layout := "200601"
		if len(suffix) == 8 {
			layout = "20060102"
		}
		lo, e := time.Parse(layout, suffix)
		if e != nil {
			continue
		}
		hi := lo.AddDate(0, 1, 0)
		if len(suffix) == 8 {
			hi = lo.AddDate(0, 0, 1)
		}
		if !hi.After(cutoff) {
			drop = append(drop, name)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, name := range drop {
		if _, err = tx.Exec(ctx, `DROP TABLE `+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE bill_record_control SET closed_before=GREATEST(closed_before,$1),updated_at=now() WHERE singleton`, financial); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM ai_request_keys WHERE registered_at<$1 AND sealed_at IS NOT NULL`, financial); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM ai_debug_sessions WHERE expires_at<now()-interval '7 days'`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *RequestStore) ensureRecordPartitions(ctx context.Context) error {
	_, err := s.financialPool.Exec(ctx, `SELECT ensure_request_record_partitions(now())`)
	return err
}
func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
func checkpointJournal(ctx context.Context, tx pgx.Tx, cutoff time.Time) error {
	// A checkpoint is derived from the previous opening plus append-only deltas.
	// The equality is validated against the live account before any history drops.
	var bad int
	err := tx.QueryRow(ctx, `WITH last AS(SELECT DISTINCT ON(account_id,meter) * FROM bill_checkpoints ORDER BY account_id,meter,through_at DESC),
 live AS (SELECT account_id,'balance' AS meter,balance_micro AS value FROM bill_accounts
 UNION ALL SELECT id::text,'api_key',quota_used FROM ai_api_keys
 UNION ALL SELECT id::text,meter,value FROM ai_sub_subscriptions CROSS JOIN LATERAL (VALUES ('subscription_total',total_used_micro),('subscription_5h',win5h_used_micro),('subscription_7d',win7d_used_micro)) m(meter,value))
 SELECT count(*) FROM live a LEFT JOIN last c USING(account_id,meter)
 WHERE c.account_id IS NULL OR a.value<>c.value+COALESCE((SELECT sum(delta) FROM bill_journal j WHERE j.account_id=c.account_id AND j.meter=c.meter AND j.created_at>=c.through_at),0)`).Scan(&bad)
	if err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("journal checkpoint refused: %d balance or quota discrepancies", bad)
	}
	_, err = tx.Exec(ctx, `WITH last AS(SELECT DISTINCT ON(account_id,meter) * FROM bill_checkpoints WHERE through_at<$1 ORDER BY account_id,meter,through_at DESC)
 INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source)
 SELECT c.account_id,c.meter,$1,c.value+COALESCE((SELECT sum(delta) FROM bill_journal j WHERE j.account_id=c.account_id AND j.meter=c.meter AND j.created_at>=c.through_at AND j.created_at<$1),0),'retention_rollforward' FROM last c
 ON CONFLICT DO NOTHING`, cutoff)
	return err
}
