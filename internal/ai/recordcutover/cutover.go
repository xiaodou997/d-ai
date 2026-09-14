// Package recordcutover owns the one-time, offline removal of superseded
// execution history. It deliberately never deletes balances or entitlements.
package recordcutover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
)

type RelationSize struct {
	Name       string `json:"name"`
	Bytes      int64  `json:"bytes"`
	HeapBytes  int64  `json:"heap_bytes"`
	IndexBytes int64  `json:"index_bytes"`
	ToastBytes int64  `json:"toast_bytes"`
}
type Inventory struct {
	Database      string           `json:"database"`
	Schema        string           `json:"schema"`
	Version       int              `json:"version"`
	DatabaseBytes int64            `json:"database_bytes"`
	SchemaBytes   int64            `json:"schema_bytes"`
	Relations     []RelationSize   `json:"relations"`
	PendingLegacy map[string]int64 `json:"pending_legacy"`
}

func Inspect(ctx context.Context, pool *pgxpool.Pool) (Inventory, error) {
	var out Inventory
	if err := pool.QueryRow(ctx, `SELECT current_database(),current_schema(),version,pg_database_size(current_database()) FROM dai_schema_metadata WHERE singleton`).Scan(&out.Database, &out.Schema, &out.Version, &out.DatabaseBytes); err != nil {
		return out, err
	}
	rows, err := pool.Query(ctx, `SELECT c.relname,pg_total_relation_size(c.oid),pg_relation_size(c.oid),pg_indexes_size(c.oid),CASE WHEN c.reltoastrelid=0 THEN 0 ELSE pg_total_relation_size(c.reltoastrelid) END FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind IN('r','m') ORDER BY pg_total_relation_size(c.oid) DESC`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r RelationSize
		if err = rows.Scan(&r.Name, &r.Bytes, &r.HeapBytes, &r.IndexBytes, &r.ToastBytes); err != nil {
			return out, err
		}
		out.Relations = append(out.Relations, r)
		out.SchemaBytes += r.Bytes
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	rows.Close()
	out.PendingLegacy, err = legacyWork(ctx, pool)
	return out, err
}

const assetSQL = `SELECT jsonb_build_object(
 'accounts',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',account_id,'kind',account_kind,'tenant',tenant_id,'balance',balance_micro) ORDER BY account_id) FROM bill_accounts),'[]'::jsonb),
 'credit_lots',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY lot_id) FROM bill_credit_lots l),'[]'::jsonb),
 'api_keys',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'used',quota_used,'limit',quota_limit,'expires_at',expires_at,'status',status) ORDER BY id) FROM ai_api_keys),'[]'::jsonb),
 'subscriptions',COALESCE((SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM ai_sub_subscriptions s),'[]'::jsonb),
 'payment_orders',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM pay_orders p),'[]'::jsonb),
 'recharge_orders',COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM bill_recharge_orders r),'[]'::jsonb),
 'payment_refunds',COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM pay_refunds r),'[]'::jsonb))`

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Assets(ctx context.Context, q Querier) (json.RawMessage, string, error) {
	var raw []byte
	err := q.QueryRow(ctx, assetSQL).Scan(&raw)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

var obsolete = []string{"ai_billing_settlement_outbox", "ai_billing_settlement_batches", "ai_billing_request_admissions", "ai_billing_windows", "bill_charge_outbox", "ledger_credit_leases", "ai_audit_inbox", "ai_request_payloads", "ai_audit_blobs", "ai_usage_rollups_hourly", "ai_usage_logs"}
var archiveName = regexp.MustCompile(`^ai_request_payloads_archive_[0-9]{4}_[0-9]{2}$`)

// Release requires an offline application and a verified external backup. It
// uses RESTRICT, never CASCADE: unexpected dependencies must be reviewed.
func Release(ctx context.Context, pool *pgxpool.Pool, wantAssetHash string) (Inventory, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Inventory{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(82624001)`); err != nil {
		return Inventory{}, err
	}
	var version int
	if err = tx.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton FOR UPDATE`).Scan(&version); err != nil {
		return Inventory{}, err
	}
	if version != 41 && version != 42 {
		return Inventory{}, fmt.Errorf("release requires prepared schema 41 or already released schema 42, got %d", version)
	}
	legacy, err := legacyWork(ctx, tx)
	if err != nil {
		return Inventory{}, err
	}
	for name, count := range legacy {
		if count > 0 {
			return Inventory{}, fmt.Errorf("legacy financial work remains in %s: %d", name, count)
		}
	}
	var accepting bool
	if err = tx.QueryRow(ctx, `SELECT accepting FROM bill_record_control WHERE singleton FOR UPDATE`).Scan(&accepting); err != nil {
		return Inventory{}, err
	}
	if accepting {
		return Inventory{}, errors.New("pause new requests before history release")
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_request_keys WHERE sealed_at IS NULL) OR EXISTS(SELECT 1 FROM bill_settlements WHERE state IN('pending','review')) OR EXISTS(SELECT 1 FROM ai_async_tasks WHERE status IN('pending','running')) OR EXISTS(SELECT 1 FROM pay_orders WHERE status IN('created','paying') OR status='paid' AND fulfillment_status='pending') OR EXISTS(SELECT 1 FROM ai_sub_orders WHERE status IN('created','deducting')) OR EXISTS(SELECT 1 FROM pay_refunds WHERE status IN('created','processing'))`).Scan(&active); err != nil {
		return Inventory{}, err
	}
	if active {
		return Inventory{}, errors.New("active requests, settlements or payment operations remain")
	}
	_, hash, err := Assets(ctx, tx)
	if err != nil {
		return Inventory{}, err
	}
	if hash != wantAssetHash {
		return Inventory{}, errors.New("assets differ from the offline backup snapshot")
	}
	// New traffic after cutover makes direct rollback unsafe. Release is only
	// permitted before the first new balance movement or settled request.
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bill_settlements) OR EXISTS(SELECT 1 FROM bill_journal)`).Scan(&active); err != nil {
		return Inventory{}, err
	}
	if active {
		return Inventory{}, errors.New("new financial activity already exists; use forward maintenance instead")
	}
	rows, err := tx.Query(ctx, `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='r' AND c.relname LIKE 'ai_request_payloads_archive_%'`)
	if err != nil {
		return Inventory{}, err
	}
	var archives []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return Inventory{}, err
		}
		if archiveName.MatchString(name) {
			archives = append(archives, name)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return Inventory{}, err
	}
	if _, err = tx.Exec(ctx, `DROP FUNCTION IF EXISTS archive_request_payloads(integer)`); err != nil {
		return Inventory{}, err
	}
	for _, name := range append(archives, obsolete...) {
		// Some optional monthly archives or legacy tables may already have been
		// retired by an earlier maintenance attempt; this makes release resumable.
		if _, err = tx.Exec(ctx, `DROP TABLE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` RESTRICT`); err != nil {
			return Inventory{}, fmt.Errorf("release %s: %w", name, err)
		}
	}
	_, after, err := Assets(ctx, tx)
	if err != nil {
		return Inventory{}, err
	}
	if after != hash {
		return Inventory{}, errors.New("asset equality check failed")
	}
	if _, err = tx.Exec(ctx, `UPDATE dai_schema_metadata SET version=42,updated_at=now() WHERE singleton AND version=41`); err != nil {
		return Inventory{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Inventory{}, err
	}
	return Inspect(ctx, pool)
}

// Only closed, fully reconciled legacy work may be discarded. Names and
// predicates are an internal allowlist, never caller-controlled SQL.
func legacyWork(ctx context.Context, q Querier) (map[string]int64, error) {
	out := map[string]int64{}
	predicates := map[string]string{
		"bill_charge_outbox":            "status<>'done'",
		"ai_billing_settlement_outbox":  "status<>'delivered'",
		"ai_billing_settlement_batches": "status<>'delivered'",
		"ai_billing_request_admissions": "status<>'completed'",
		"ai_billing_windows":            "state<>'settled'",
		"ledger_credit_leases":          "escrow_state<>'released' OR settlement_state<>'settled'",
	}
	for table, predicate := range predicates {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		var count int64
		if err := q.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()+" WHERE "+predicate).Scan(&count); err != nil {
			return nil, err
		}
		out[table] = count
	}
	return out, nil
}
