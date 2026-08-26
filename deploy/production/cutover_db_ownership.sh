#!/usr/bin/env bash
set -euo pipefail

# Verify the two application DSNs and apply the ownership contract as one
# explicit maintenance-window operation. This wrapper does not stop services;
# the operator must stop them first, and the active-session check is repeated
# immediately before ownership changes.

usage() {
  cat <<'USAGE'
Usage:
  cutover_db_ownership.sh preflight   # verify DSNs and an empty maintenance window
  cutover_db_ownership.sh apply       # apply ownership/revoke after explicit confirmation
  cutover_db_ownership.sh verify      # verify grants after the application is running
  cutover_db_ownership.sh --help

Required for all commands:
  DB_OWNERSHIP_CUTOVER_ADMIN_DATABASE_URL    database owner/superuser URL
  DB_OWNERSHIP_CUTOVER_RUNTIME_DATABASE_URL  runtime application DSN
  DB_OWNERSHIP_CUTOVER_BILLING_DATABASE_URL  billing application DSN

Required for apply:
  DB_OWNERSHIP_CUTOVER_CONFIRM=APPLY         explicit destructive-operation confirmation
  DB_OWNERSHIP_CUTOVER_WINDOW=OPEN           operator-confirmed maintenance window

Optional:
  DB_OWNERSHIP_CUTOVER_SCHEMA       default: public
  DB_OWNERSHIP_CUTOVER_RUNTIME_ROLE default: dai
  DB_OWNERSHIP_CUTOVER_BILLING_ROLE default: dai_billing
  DB_OWNERSHIP_CUTOVER_HEALTH_URL   readiness URL for verify
  DB_OWNERSHIP_CUTOVER_APPLY_SCRIPT path to apply_db_ownership.sh (optional)

The three DSNs fall back to SCHEMA_OWNERSHIP_DATABASE_URL, DAI_DATABASE_URL,
and DAI_BILLING_DATABASE_URL respectively. Passwords stay in the deployment
secret manager; this script never prints them.
USAGE
}

fail() {
  echo "db-ownership-cutover: $*" >&2
  exit 1
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

command_name=${1:-}
case "$command_name" in
  preflight|apply|verify) ;;
  *)
    usage >&2
    exit 2
    ;;
esac

admin_database_url=${DB_OWNERSHIP_CUTOVER_ADMIN_DATABASE_URL:-${SCHEMA_OWNERSHIP_DATABASE_URL:-}}
runtime_database_url=${DB_OWNERSHIP_CUTOVER_RUNTIME_DATABASE_URL:-${DAI_DATABASE_URL:-}}
billing_database_url=${DB_OWNERSHIP_CUTOVER_BILLING_DATABASE_URL:-${DAI_BILLING_DATABASE_URL:-}}
schema_name=${DB_OWNERSHIP_CUTOVER_SCHEMA:-public}
runtime_role=${DB_OWNERSHIP_CUTOVER_RUNTIME_ROLE:-dai}
billing_role=${DB_OWNERSHIP_CUTOVER_BILLING_ROLE:-dai_billing}
health_url=${DB_OWNERSHIP_CUTOVER_HEALTH_URL:-}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
project_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ownership_sql=${DB_OWNERSHIP_CUTOVER_SQL:-$project_dir/internal/db/ownership.sql}
if [[ -z "${DB_OWNERSHIP_CUTOVER_SQL:-}" && ! -f "$ownership_sql" && -f "$script_dir/ownership.sql" ]]; then
  ownership_sql=$script_dir/ownership.sql
fi
apply_script=${DB_OWNERSHIP_CUTOVER_APPLY_SCRIPT:-$project_dir/deploy/production/apply_db_ownership.sh}
if [[ -z "${DB_OWNERSHIP_CUTOVER_APPLY_SCRIPT:-}" && ! -f "$apply_script" && -f "$script_dir/apply_db_ownership.sh" ]]; then
  apply_script=$script_dir/apply_db_ownership.sh
fi

[[ -n "$admin_database_url" ]] || fail "set DB_OWNERSHIP_CUTOVER_ADMIN_DATABASE_URL (or SCHEMA_OWNERSHIP_DATABASE_URL)"
[[ -n "$runtime_database_url" ]] || fail "set DB_OWNERSHIP_CUTOVER_RUNTIME_DATABASE_URL (or DAI_DATABASE_URL)"
[[ -n "$billing_database_url" ]] || fail "set DB_OWNERSHIP_CUTOVER_BILLING_DATABASE_URL (or DAI_BILLING_DATABASE_URL)"
[[ -f "$ownership_sql" ]] || fail "ownership contract not found: $ownership_sql"
[[ -f "$apply_script" ]] || fail "ownership apply script not found: $apply_script"
for identifier in "$schema_name" "$runtime_role" "$billing_role"; do
  [[ "$identifier" =~ ^[a-z_][a-z0-9_]*$ ]] || fail "invalid PostgreSQL identifier: $identifier"
done
[[ "$runtime_role" != "$billing_role" ]] || fail "runtime and billing roles must be different"

if [[ "$command_name" == "apply" ]]; then
  [[ "${DB_OWNERSHIP_CUTOVER_CONFIRM:-}" == "APPLY" ]] || fail "apply requires DB_OWNERSHIP_CUTOVER_CONFIRM=APPLY"
  [[ "${DB_OWNERSHIP_CUTOVER_WINDOW:-}" == "OPEN" ]] || fail "apply requires DB_OWNERSHIP_CUTOVER_WINDOW=OPEN"
fi
command -v psql >/dev/null 2>&1 || fail "required command not found: psql"

psql_query() {
  psql -X -v ON_ERROR_STOP=1 --dbname="$admin_database_url" -Atqc "$1"
}

role_identity() {
  local database_url=$1
  psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -Atqc \
    "SELECT current_user || E'\\t' || current_database()"
}

assert_application_dsns() {
  local admin_identity runtime_identity billing_identity admin_user admin_db runtime_user runtime_db billing_user billing_db
  admin_identity=$(role_identity "$admin_database_url") || fail "admin database connection failed"
  runtime_identity=$(role_identity "$runtime_database_url") || fail "runtime database connection failed"
  billing_identity=$(role_identity "$billing_database_url") || fail "billing database connection failed"

  IFS=$'\t' read -r admin_user admin_db <<<"$admin_identity"
  IFS=$'\t' read -r runtime_user runtime_db <<<"$runtime_identity"
  IFS=$'\t' read -r billing_user billing_db <<<"$billing_identity"
  [[ "$admin_user" != "$runtime_role" && "$admin_user" != "$billing_role" ]] || fail "admin DSN must not use an application role"
  [[ "$runtime_user" == "$runtime_role" ]] || fail "runtime DSN connected as $runtime_user, expected $runtime_role"
  [[ "$billing_user" == "$billing_role" ]] || fail "billing DSN connected as $billing_user, expected $billing_role"
  [[ "$runtime_db" == "$admin_db" && "$billing_db" == "$admin_db" ]] || fail "admin, runtime and billing DSNs must target the same database"
}

assert_role_attributes() {
  local role_count unsafe_count membership_count
  role_count=$(psql_query "
    SELECT count(*)
    FROM pg_roles
    WHERE rolname IN ('$runtime_role', '$billing_role')
  ")
  [[ "$role_count" == "2" ]] || fail "runtime/billing roles are not both provisioned"

  unsafe_count=$(psql_query "
    SELECT count(*)
    FROM pg_roles
    WHERE rolname IN ('$runtime_role', '$billing_role')
      AND (NOT rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls OR rolinherit)
  ")
  [[ "$unsafe_count" == "0" ]] || fail "runtime/billing role attributes are not least privilege"

  membership_count=$(psql_query "
    SELECT count(*)
    FROM pg_auth_members m
    JOIN pg_roles member ON member.oid = m.member
    WHERE member.rolname IN ('$runtime_role', '$billing_role')
  ")
  [[ "$membership_count" == "0" ]] || fail "runtime/billing roles must not inherit role memberships"
}

assert_no_client_sessions() {
  local active_sessions
  active_sessions=$(psql_query "
    SELECT count(*)
    FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
      AND backend_type = 'client backend'
  ")
  [[ "$active_sessions" == "0" ]] || fail "found $active_sessions active database session(s); stop all application instances before cutover"
}

assert_post_cutover_contract() {
  local runtime_projection billing_accounts runtime_update runtime_outbox
  runtime_projection=$(psql_query "SELECT has_table_privilege('$runtime_role', '$schema_name.tenant_management_projection', 'SELECT')")
  billing_accounts=$(psql_query "SELECT has_table_privilege('$billing_role', '$schema_name.bill_accounts', 'UPDATE')")
  runtime_update=$(psql_query "SELECT has_table_privilege('$runtime_role', '$schema_name.bill_accounts', 'UPDATE')")
  runtime_outbox=$(psql_query "SELECT has_table_privilege('$runtime_role', '$schema_name.bill_charge_outbox', 'INSERT')")
  [[ "$runtime_projection" == "t" ]] || fail "runtime role cannot read tenant management projection"
  [[ "$billing_accounts" == "t" ]] || fail "billing role cannot update bill_accounts"
  [[ "$runtime_update" == "f" ]] || fail "runtime role still has bill_accounts UPDATE privilege"
  [[ "$runtime_outbox" == "t" ]] || fail "runtime role cannot enqueue bill_charge_outbox"
}

run_preflight() {
  assert_role_attributes
  assert_application_dsns
  assert_no_client_sessions
  echo "db-ownership-cutover: preflight passed (roles=$runtime_role,$billing_role schema=$schema_name)"
}

run_apply() {
  run_preflight
  SCHEMA_OWNERSHIP_DATABASE_URL="$admin_database_url" \
  SCHEMA_OWNERSHIP_SCHEMA="$schema_name" \
  SCHEMA_OWNERSHIP_RUNTIME_ROLE="$runtime_role" \
  SCHEMA_OWNERSHIP_BILLING_ROLE="$billing_role" \
  SCHEMA_OWNERSHIP_SQL="$ownership_sql" \
  SCHEMA_OWNERSHIP_CONFIRM=APPLY \
    bash "$apply_script"
  assert_post_cutover_contract
  echo "db-ownership-cutover: ownership contract applied and verified; keep application stopped until readiness checks pass"
}

run_verify() {
  assert_role_attributes
  assert_application_dsns
  assert_post_cutover_contract
  if [[ -n "$health_url" ]]; then
    command -v curl >/dev/null 2>&1 || fail "required command not found: curl"
    curl --fail --silent --show-error "$health_url" >/dev/null
    echo "db-ownership-cutover: readiness passed: $health_url"
  fi
  echo "db-ownership-cutover: post-cutover role contract verified"
}

case "$command_name" in
  preflight) run_preflight ;;
  apply) run_apply ;;
  verify) run_verify ;;
esac
