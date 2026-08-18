#!/usr/bin/env bash
set -euo pipefail

mode=${1:-}
if [[ "$mode" != "preflight" && "$mode" != "final" ]]; then
  echo "usage: $0 preflight|final" >&2
  echo "preflight is a destructive database rehearsal and preserves file storage; final also clears AI file storage." >&2
  exit 2
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
project_dir=$(cd "$script_dir/../.." && pwd)
compose=(docker compose --env-file "$script_dir/.env" -f "$script_dir/compose.yaml")
source_container=unihub-postgres-1
target_container=dai-postgres-1
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
backup_dir="$project_dir/backups/${timestamp}-${mode}"
data_root="$project_dir/deploy/data"
ai_file_storage=unchanged

mkdir -p "$backup_dir"

source_psql() {
  local database=$1
  shift
  docker exec "$source_container" psql -X -v ON_ERROR_STOP=1 -U postgres -d "$database" "$@"
}

target_psql() {
  docker exec -i "$target_container" psql -X -v ON_ERROR_STOP=1 -U postgres -d dai "$@"
}

copy_query() {
  local source_database=$1
  local target_table=$2
  local target_columns=$3
  local source_query=$4

  target_psql -qAtc "TRUNCATE TABLE public.$target_table RESTART IDENTITY CASCADE"
  source_psql "$source_database" -q -c \
    "COPY ($source_query) TO STDOUT WITH (FORMAT binary)" \
    | docker exec -i "$target_container" psql -X -v ON_ERROR_STOP=1 -U postgres -d dai -q -c \
      "COPY public.$target_table ($target_columns) FROM STDIN WITH (FORMAT binary)"
}

copy_target_columns() {
  local source_database=$1
  local table=$2
  local where_clause=${3:-}
  local columns

  columns=$(target_psql -qAtc "
    SELECT string_agg(quote_ident(column_name), ',' ORDER BY ordinal_position)
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = '$table'
  ")
  if [[ -z "$columns" ]]; then
    echo "target table has no columns: $table" >&2
    exit 1
  fi
  copy_query "$source_database" "$table" "$columns" \
    "SELECT $columns FROM public.$table $where_clause"
}

echo "Creating source backups in $backup_dir"
docker exec "$source_container" pg_dump -U postgres -d urm -Fc > "$backup_dir/urm.dump"
# Keep the old AI dump only for offline rollback; it is deliberately not imported.
docker exec "$source_container" pg_dump -U postgres -d ai_gateway -Fc > "$backup_dir/ai_gateway.dump"
sha256sum "$backup_dir/urm.dump" "$backup_dir/ai_gateway.dump" > "$backup_dir/SHA256SUMS"

echo "Reinitializing isolated D-AI database"
"${compose[@]}" stop app >/dev/null 2>&1 || true
"${compose[@]}" up -d --wait postgres redis
docker exec "$target_container" psql -X -v ON_ERROR_STOP=1 -U postgres -d postgres -q -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'dai' AND pid <> pg_backend_pid()"
docker exec "$target_container" dropdb -U postgres --if-exists dai
docker exec "$target_container" createdb -U postgres dai
docker exec -i "$target_container" psql -X -v ON_ERROR_STOP=1 -U postgres -d dai -q \
  < "$project_dir/internal/db/init.sql"

echo "Migrating URM identity and tenant data"
copy_target_columns urm iam_tenants
copy_query urm iam_accounts \
  "user_id,tenant_id,username,password_hash,email,phone,user_type,internal_note,nickname,avatar,frozen_credits,overdraft_limit,current_overdraft,status,last_login_at,created_at,updated_at" \
  "
    SELECT user_id, NULL::text, btrim(username), password_hash, email, NULL::text,
           user_type, ''::text, NULL::text, NULL::text, 0::bigint, 0::bigint,
           0::bigint, status, last_login_at, created_at, updated_at
    FROM public.iam_admins
    UNION ALL
    SELECT t.user_id, t.tenant_id,
           CASE WHEN EXISTS (
             SELECT 1 FROM public.iam_admins a
             WHERE lower(btrim(a.username)) = lower(btrim(t.username))
           ) THEN 't_' || btrim(t.username) ELSE btrim(t.username) END,
           t.password_hash, t.email, t.phone, 3, ''::text, NULL::text, NULL::text,
           0::bigint, 0::bigint, 0::bigint, t.status, t.last_login_at,
           t.created_at, t.updated_at
    FROM public.iam_tenant_users t
    UNION ALL
    SELECT user_id, tenant_id,
           CASE WHEN btrim(username) LIKE 'u\\_%' ESCAPE '\\'
                THEN btrim(username) ELSE 'u_' || btrim(username) END,
           password_hash, email, phone, 4, ''::text, nickname, avatar,
           frozen_credits, overdraft_limit, current_overdraft, status,
           last_login_at, created_at, updated_at
    FROM public.iam_users
  "
copy_target_columns urm iam_invitation_codes
copy_target_columns urm auth_signing_keys
copy_target_columns urm auth_audit_logs

echo "Migrating URM balances and recharge history"
copy_query urm bill_recharge_orders \
  "id,order_id,order_type,tenant_id,user_id,credit_amount,paid_amount,payment_ref,expires_at,operator_id,note,status,reversed_at,reversed_by,reversal_reason,created_at" \
  "
    SELECT id, order_id,
           CASE WHEN order_type = 'cash_purchase' THEN 'user_topup_income' ELSE order_type END,
           tenant_id, user_id, credit_amount, paid_amount, payment_ref, expires_at,
           operator_id, note, status, reversed_at, reversed_by, reversal_reason, created_at
    FROM public.bill_recharge_orders
  "
copy_query urm bill_credit_packages \
  "id,package_id,package_type,tenant_id,user_id,total_credits,remaining_credits,expires_at,status,source,recharge_order_id,version,created_at,updated_at" \
  "
    SELECT id, package_id, package_type, tenant_id, user_id, total_credits,
           remaining_credits, expires_at, status,
           CASE WHEN source = 'CASH_PURCHASE' THEN 'USER_TOPUP_INCOME' ELSE source END,
           recharge_order_id, version, created_at, updated_at
    FROM public.bill_credit_packages
  "
copy_target_columns urm bill_overdraft_adjustments
copy_query urm pay_orders \
  "id,order_id,out_trade_no,scene,tenant_id,user_id,topup_mode,package_id,package_name,package_badge,payment_currency,payment_amount_minor,ledger_currency,gross_amount_micro_usd,fee_amount_micro_usd,gift_amount_micro_usd,credited_amount_micro_usd,fee_rate_bp,tenant_income_micro_usd,balance_expires_at,channel,code_url,transaction_id,status,paid_at,expires_at,balance_order_id,fail_note,notify_raw,created_at,updated_at" \
  "
    SELECT id, order_id, out_trade_no, scene, tenant_id, user_id, topup_mode,
           package_id, package_name, package_badge, 'USD'::text, amount,
           'USD'::text, gross_credit_amount * 10000, fee_credit_amount * 10000,
           0::bigint, credit_amount * 10000, fee_rate_bp, net_amount * 10000,
           NULL::timestamptz, channel, code_url, transaction_id, status, paid_at,
           expires_at, credit_order_id, fail_note, notify_raw, created_at, updated_at
    FROM public.pay_orders
  "
copy_query urm pay_cash_ledger \
  "id,txn_id,tenant_id,txn_type,amount_micro_usd,balance_after_micro_usd,ref_type,ref_id,operator_id,note,idempotency_key,created_at" \
  "
    SELECT id, txn_id, tenant_id,
           CASE WHEN txn_type = 'buy_credits' THEN 'consumption' ELSE txn_type END,
           amount * 10000, balance_after * 10000, ref_type, ref_id, operator_id,
           note, idempotency_key, created_at
    FROM public.pay_cash_ledger
  "
copy_query urm pay_withdrawals \
  "id,withdrawal_id,tenant_id,amount_micro_usd,fee_amount_micro_usd,payout_amount_micro_usd,account_name,bank_name,account_no,apply_note,status,applied_by,reviewed_by,reviewed_at,review_note,paid_by,paid_at,payment_ref,created_at,updated_at" \
  "
    SELECT id, withdrawal_id, tenant_id, amount * 10000, fee_amount * 10000,
           payout_amount * 10000, account_name, bank_name, account_no, apply_note,
           status, applied_by, reviewed_by, reviewed_at, review_note, paid_by,
           paid_at, payment_ref, created_at, updated_at
    FROM public.pay_withdrawals
  "
copy_query urm sys_settings "key,value,updated_by,updated_at" "
  SELECT key,
         CASE WHEN key <> 'payment' THEN value ELSE jsonb_build_object(
           'tenantCustomTopupFeeBp', COALESCE((value->>'tenantCustomTopupFeeBp')::int, 0),
           'tenantWithdrawFeeBp', COALESCE((value->>'tenantWithdrawFeeBp')::int, 0),
           'tenantTopupPackages', COALESCE((
             SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
               'id', p->>'id', 'name', p->>'name',
               'paymentAmountMicroUsd', (p->>'amount')::bigint * 10000,
               'giftAmountMicroUsd', 0, 'badge', p->>'badge',
               'enabled', COALESCE((p->>'enabled')::boolean, true),
               'sortOrder', COALESCE((p->>'sortOrder')::int, 0)
             )) ORDER BY COALESCE((p->>'sortOrder')::int, 0))
             FROM jsonb_array_elements(COALESCE(value->'tenantTopupPackages', '[]'::jsonb)) p
           ), '[]'::jsonb)
         ) END,
         updated_by, updated_at
  FROM public.sys_settings
"
copy_query urm pay_tenant_settings "tenant_id,value,updated_by,updated_at" "
  SELECT tenant_id, jsonb_build_object(
           'userCustomTopupFeeBp', COALESCE((value->>'userCustomTopupFeeBp')::int, 0),
           'userTopupPackages', COALESCE((
             SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
               'id', p->>'id', 'name', p->>'name',
               'paymentAmountMicroUsd', (p->>'amount')::bigint * 10000,
               'giftAmountMicroUsd', 0, 'badge', p->>'badge',
               'enabled', COALESCE((p->>'enabled')::boolean, true),
               'sortOrder', COALESCE((p->>'sortOrder')::int, 0)
             )) ORDER BY COALESCE((p->>'sortOrder')::int, 0))
             FROM jsonb_array_elements(COALESCE(value->'userTopupPackages', '[]'::jsonb)) p
           ), '[]'::jsonb)
         ), updated_by, updated_at
  FROM public.pay_tenant_settings
"
copy_target_columns urm pay_wechat_config
copy_target_columns urm iam_tenant_portal_branding
copy_target_columns urm iam_user_legal_acceptances
copy_target_columns urm ann_announcements
copy_target_columns urm ann_audiences
copy_target_columns urm ann_receipts
copy_target_columns urm ann_audit_events

echo "Skipping AI runtime data: D-AI starts with empty AI configuration and schema defaults"

echo "Resetting imported sequences"
docker exec -i "$target_container" psql -X -v ON_ERROR_STOP=1 -U postgres -d dai -q <<'SQL'
DO $$
DECLARE
  row record;
  has_rows boolean;
  max_value bigint;
BEGIN
  FOR row IN
    SELECT table_name, column_name,
           pg_get_serial_sequence(format('%I.%I', table_schema, table_name), column_name) AS sequence_name
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND column_default LIKE 'nextval(%'
  LOOP
    EXECUTE format('SELECT max(%I) IS NOT NULL, COALESCE(max(%I), 1) FROM %I.%I',
                   row.column_name, row.column_name, 'public', row.table_name)
      INTO has_rows, max_value;
    PERFORM setval(row.sequence_name, max_value, has_rows);
  END LOOP;
END
$$;
SQL

echo "Validating AI reset"
target_psql -X -v ON_ERROR_STOP=1 -q <<'SQL'
DO $$
DECLARE
  row record;
  row_count bigint;
BEGIN
  FOR row IN
    SELECT tablename
    FROM pg_catalog.pg_tables
    WHERE schemaname = 'public'
      AND tablename LIKE 'ai_%'
      AND tablename NOT IN ('ai_settings', 'ai_route_score_weights')
  LOOP
    EXECUTE format('SELECT count(*) FROM public.%I', row.tablename) INTO row_count;
    IF row_count <> 0 THEN
      RAISE EXCEPTION 'AI reset validation failed: % has % rows', row.tablename, row_count;
    END IF;
  END LOOP;

  IF (SELECT count(*) FROM public.file_assets) <> 0
     OR (SELECT count(*) FROM public.file_access_links) <> 0 THEN
    RAISE EXCEPTION 'AI file reset validation failed';
  END IF;
END
$$;
SQL

echo "Validating migrated data"
source_identity_count=$(source_psql urm -qAtc \
  "SELECT (SELECT count(*) FROM iam_admins) + (SELECT count(*) FROM iam_tenant_users) + (SELECT count(*) FROM iam_users)")
source_credit_summary=$(source_psql urm -qAtc \
  "SELECT count(*) || ':' || COALESCE(sum(total_credits),0) || ':' || COALESCE(sum(remaining_credits),0) FROM bill_credit_packages")
target_identity_count=$(target_psql -qAtc "SELECT count(*) FROM iam_accounts")
target_credit_summary=$(target_psql -qAtc \
  "SELECT count(*) || ':' || COALESCE(sum(total_credits),0) || ':' || COALESCE(sum(remaining_credits),0) FROM bill_credit_packages")

[[ "$source_identity_count" == "$target_identity_count" ]]
[[ "$source_credit_summary" == "$target_credit_summary" ]]

if [[ "$mode" == "final" ]]; then
  case "$data_root" in
    ""|"/")
      echo "refusing to clear unsafe data root: $data_root" >&2
      exit 1
      ;;
  esac
  echo "Clearing AI file storage and adopting the unified data root"
  rm -rf -- "$data_root/images" "$data_root/files" \
    "$data_root/ai-images" "$data_root/ai-files"
  mkdir -p "$data_root/images" "$data_root/files"
  ai_file_storage=reset
fi

target_psql -P pager=off -c "
  SELECT version AS schema_version FROM dai_schema_metadata;
  SELECT user_type, count(*) AS accounts FROM iam_accounts GROUP BY user_type ORDER BY user_type;
  SELECT count(*) AS tenants FROM iam_tenants;
  SELECT package_type, status, count(*) AS packages,
         sum(total_credits) AS total_micro_usd,
         sum(remaining_credits) AS remaining_micro_usd
  FROM bill_credit_packages GROUP BY package_type, status ORDER BY package_type, status;
  SELECT count(*) AS ai_usage_rows FROM ai_usage_logs;
  SELECT key FROM ai_settings ORDER BY key;
  SELECT scope FROM ai_route_score_weights ORDER BY scope;
"

cat > "$backup_dir/MIGRATION.txt" <<EOF
mode=$mode
source_identity_count=$source_identity_count
target_identity_count=$target_identity_count
source_credit_summary=$source_credit_summary
target_credit_summary=$target_credit_summary
ai_data=reset_to_schema_defaults
ai_usage_history=not_migrated
ai_file_storage=$ai_file_storage
EOF

echo "Migration $mode completed; backup: $backup_dir"
