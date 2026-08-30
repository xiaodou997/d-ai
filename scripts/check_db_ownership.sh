#!/usr/bin/env bash
set -euo pipefail

# Probe the ownership contract without touching the application's production
# role. A superuser creates an isolated schema and two ephemeral NOLOGIN roles,
# applies the real contract, then exercises both allowed and denied writes.

database_url=${SCHEMA_OWNERSHIP_DATABASE_URL:-${DAI_TEST_DATABASE_URL:-}}
project_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
[[ -n "$database_url" ]] || {
  echo "db-ownership: set SCHEMA_OWNERSHIP_DATABASE_URL (or DAI_TEST_DATABASE_URL)" >&2
  exit 1
}
command -v psql >/dev/null 2>&1 || {
  echo "db-ownership: required command not found: psql" >&2
  exit 1
}

suffix="$$_$(date +%s)"
schema_name="dai_ownership_probe_${suffix}"
runtime_role="dai_probe_runtime_${suffix}"
billing_role="dai_probe_billing_${suffix}"
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/dai-db-ownership.XXXXXX")

cleanup() {
  psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -c \
    "DROP SCHEMA IF EXISTS \"$schema_name\" CASCADE;
     REASSIGN OWNED BY \"$billing_role\" TO CURRENT_USER;
     REASSIGN OWNED BY \"$runtime_role\" TO CURRENT_USER;
     DROP OWNED BY \"$billing_role\";
     DROP OWNED BY \"$runtime_role\";
     DROP ROLE IF EXISTS \"$billing_role\";
     DROP ROLE IF EXISTS \"$runtime_role\";" >/dev/null 2>&1 || true
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -c \
  "CREATE ROLE \"$runtime_role\" NOLOGIN;
   CREATE ROLE \"$billing_role\" NOLOGIN;
   CREATE SCHEMA \"$schema_name\";"

{
  printf 'SET search_path TO "%s";\n' "$schema_name"
  cat "$project_dir/internal/db/init.sql"
} >"$tmp_dir/init.sql"
psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" --file="$tmp_dir/init.sql"

SCHEMA_OWNERSHIP_CONFIRM=APPLY \
SCHEMA_OWNERSHIP_DATABASE_URL="$database_url" \
SCHEMA_OWNERSHIP_SCHEMA="$schema_name" \
SCHEMA_OWNERSHIP_RUNTIME_ROLE="$runtime_role" \
SCHEMA_OWNERSHIP_BILLING_ROLE="$billing_role" \
SCHEMA_OWNERSHIP_SQL="$project_dir/internal/db/ownership.sql" \
  "$project_dir/deploy/production/apply_db_ownership.sh"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -c \
  "SET ROLE \"$runtime_role\";
   SELECT count(*) FROM \"$schema_name\".bill_accounts;"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      INSERT INTO \"$schema_name\".iam_tenants (tenant_id, tenant_name, status)
        VALUES ('ownership-runtime-tenant', 'Ownership Runtime Tenant', 'active');
      INSERT INTO \"$schema_name\".iam_accounts
        (user_id, tenant_id, username, password_hash, user_type, status)
        VALUES ('ownership-runtime-user', 'ownership-runtime-tenant',
                'ownership-runtime-user', 'hash', 4, 'active');
      SELECT count(*) FROM \"$schema_name\".bill_accounts
       WHERE account_id IN ('ownership-runtime-tenant', 'ownership-runtime-user');"

if psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      SELECT count(*) FROM \"$schema_name\".pay_cash_ledger;" \
  >"$tmp_dir/runtime-raw-billing-read-denied.log" 2>&1; then
  echo "db-ownership: runtime role unexpectedly read raw pay_cash_ledger" >&2
  cat "$tmp_dir/runtime-raw-billing-read-denied.log" >&2
  exit 1
fi

if psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -c \
  "SET ROLE \"$runtime_role\";
   UPDATE \"$schema_name\".bill_accounts SET balance_micro = balance_micro + 1;" \
  >"$tmp_dir/runtime-denied.log" 2>&1; then
  echo "db-ownership: runtime role unexpectedly updated bill_accounts" >&2
  cat "$tmp_dir/runtime-denied.log" >&2
  exit 1
fi

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      INSERT INTO \"$schema_name\".bill_charge_outbox
        (request_id, tenant_id, tenant_micro, user_micro, description)
      VALUES ('ownership-probe', 'ownership-probe', 1, 0, 'runtime enqueue');"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      INSERT INTO \"$schema_name\".bill_accounts (account_id, account_kind, tenant_id)
        VALUES ('ownership-billing', 1, 'ownership-billing');
      UPDATE \"$schema_name\".bill_accounts
        SET balance_micro = 7 WHERE account_id = 'ownership-billing';"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      SELECT count(*) FROM \"$schema_name\".billing_recharge_order_projection;"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      INSERT INTO \"$schema_name\".bill_repair_audits
        (repair_id, action, idempotency_key, target_type, target_id,
         operator_id, reason, before_state, after_state)
      VALUES ('ownership-repair', 'usage_refund', 'ownership-repair-key',
              'probe', 'ownership-probe', 'ownership-test', 'probe', '{}', '{}');
      SELECT count(*) FROM \"$schema_name\".bill_repair_audits;"

if psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      UPDATE \"$schema_name\".bill_repair_audits SET reason = 'tampered';" \
  >"$tmp_dir/audit-update-denied.log" 2>&1; then
  echo "db-ownership: billing role unexpectedly updated immutable repair audit" >&2
  cat "$tmp_dir/audit-update-denied.log" >&2
  exit 1
fi

if psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      DELETE FROM \"$schema_name\".bill_repair_audits;" \
  >"$tmp_dir/audit-delete-denied.log" 2>&1; then
  echo "db-ownership: billing role unexpectedly deleted immutable repair audit" >&2
  cat "$tmp_dir/audit-delete-denied.log" >&2
  exit 1
fi

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" \
  -c "SET search_path TO \"$schema_name\";
      INSERT INTO \"$schema_name\".iam_tenants (tenant_id, tenant_name, status)
        VALUES ('ownership-requeue-tenant', 'Ownership Requeue Tenant', 'active');
      INSERT INTO \"$schema_name\".iam_accounts
        (user_id, tenant_id, username, password_hash, user_type, status)
        VALUES ('ownership-requeue-user', 'ownership-requeue-tenant', 'ownership-requeue-user', 'hash', 4, 'active');
      INSERT INTO \"$schema_name\".bill_accounts (account_id, account_kind, tenant_id)
        VALUES ('ownership-requeue-tenant', 1, 'ownership-requeue-tenant'),
               ('ownership-requeue-user', 2, 'ownership-requeue-tenant')
        ON CONFLICT (account_id) DO NOTHING;
      INSERT INTO \"$schema_name\".ai_usage_logs
        (request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
         model_code, billable_unit_type, tenant_payable, user_payable, user_charged,
         billing_status, request_status, client_protocol, billing_source)
        VALUES ('ownership-requeue-request', 'user', 'jwt', 'ownership-probe',
                'ownership-requeue-tenant', 'ownership-requeue-user', 'probe-model', 'token',
                10, 5, 5, 'failed', 'success', 'openai_chat', 'payg');
      INSERT INTO \"$schema_name\".bill_charge_outbox
        (request_id, tenant_id, user_id, tenant_micro, user_micro, status, attempts, last_error)
        VALUES ('ownership-requeue-request', 'ownership-requeue-tenant', 'ownership-requeue-user',
                10, 5, 'failed', 10, 'probe');"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      SET search_path TO \"$schema_name\";
      SELECT \"$schema_name\".bill_requeue_parked_outbox(
        'ownership-requeue-request', 'ownership-requeue-repair',
        'outbox-requeue:ownership-requeue-request:probe', 'ownership-test', 'probe repair');"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      SELECT count(*) FROM \"$schema_name\".ai_sub_subscriptions;"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      SELECT count(*) FROM \"$schema_name\".tenant_management_projection;"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      SELECT count(*) FROM \"$schema_name\".user_admin_end_user_projection;"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      SELECT count(*) FROM \"$schema_name\".system_usage_projection;"

psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v runtime_role="$runtime_role" \
  -c "SET ROLE \"$runtime_role\";
      SELECT count(*) FROM \"$schema_name\".system_account_stats_projection;
      SELECT count(*) FROM \"$schema_name\".tenant_income_projection;"

if psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" \
  -v schema_name="$schema_name" -v billing_role="$billing_role" \
  -c "SET ROLE \"$billing_role\";
      SELECT count(*) FROM \"$schema_name\".ai_groups;" \
  >"$tmp_dir/billing-denied.log" 2>&1; then
  echo "db-ownership: billing role unexpectedly read an unrelated catalog table" >&2
  cat "$tmp_dir/billing-denied.log" >&2
  exit 1
fi

echo "db-ownership: runtime projection read/outbox insert, billing ledger/view/audit insert passed; raw billing read, immutable audit update/delete, runtime ledger update and billing catalog read denied"
