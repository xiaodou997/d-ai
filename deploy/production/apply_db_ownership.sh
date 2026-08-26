#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  apply_db_ownership.sh

Required:
  SCHEMA_OWNERSHIP_DATABASE_URL  PostgreSQL URL (or DAI_DATABASE_URL)
  SCHEMA_OWNERSHIP_CONFIRM=APPLY  explicit production confirmation

Optional role/schema names (validated as simple PostgreSQL identifiers):
  SCHEMA_OWNERSHIP_SCHEMA       default: public
  SCHEMA_OWNERSHIP_RUNTIME_ROLE default: dai
  SCHEMA_OWNERSHIP_BILLING_ROLE default: dai_billing
USAGE
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

database_url=${SCHEMA_OWNERSHIP_DATABASE_URL:-${DAI_DATABASE_URL:-}}
schema_name=${SCHEMA_OWNERSHIP_SCHEMA:-public}
runtime_role=${SCHEMA_OWNERSHIP_RUNTIME_ROLE:-dai}
billing_role=${SCHEMA_OWNERSHIP_BILLING_ROLE:-dai_billing}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
project_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ownership_sql=${SCHEMA_OWNERSHIP_SQL:-$project_dir/internal/db/ownership.sql}
if [[ -z "${SCHEMA_OWNERSHIP_SQL:-}" && ! -f "$ownership_sql" && -f "$script_dir/ownership.sql" ]]; then
  ownership_sql=$script_dir/ownership.sql
fi

[[ "${SCHEMA_OWNERSHIP_CONFIRM:-}" == "APPLY" ]] || {
  echo "db-ownership: set SCHEMA_OWNERSHIP_CONFIRM=APPLY" >&2
  exit 1
}
[[ -n "$database_url" ]] || {
  echo "db-ownership: set SCHEMA_OWNERSHIP_DATABASE_URL (or DAI_DATABASE_URL)" >&2
  exit 1
}
[[ -f "$ownership_sql" ]] || {
  echo "db-ownership: ownership contract not found: $ownership_sql" >&2
  exit 1
}

for identifier in "$schema_name" "$runtime_role" "$billing_role"; do
  [[ "$identifier" =~ ^[a-z_][a-z0-9_]*$ ]] || {
    echo "db-ownership: invalid PostgreSQL identifier: $identifier" >&2
    exit 1
  }
done
command -v psql >/dev/null 2>&1 || {
  echo "db-ownership: required command not found: psql" >&2
  exit 1
}

psql -X -v ON_ERROR_STOP=1 \
  --dbname="$database_url" \
  -v schema_name="$schema_name" \
  -v runtime_role="$runtime_role" \
  -v billing_role="$billing_role" \
  --file="$ownership_sql"

echo "db-ownership: applied schema=$schema_name runtime=$runtime_role billing=$billing_role"
