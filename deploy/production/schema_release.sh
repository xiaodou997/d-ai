#!/usr/bin/env bash
set -euo pipefail

# Explicit release-time schema migration helper. The application never calls
# this script and never runs DDL during startup. It creates a verified backup,
# applies only the contiguous pending SQL files, and records the exact window.

usage() {
  cat <<'USAGE'
Usage:
  schema_release.sh preflight   # read-only checks and migration plan
  schema_release.sh migrate     # backup, then apply pending migrations
  schema_release.sh verify      # verify target schema and optional readiness URL
  schema_release.sh --help

Required for database commands:
  SCHEMA_DATABASE_URL   PostgreSQL connection URL (or DAI_DATABASE_URL)

Optional:
  SCHEMA_SQL_DIR         SQL artifact directory (default: internal/db)
  SCHEMA_BACKUP_DIR      Backup root (default: backups/schema)
  SCHEMA_RELEASE_HEALTH_URL
                         readiness URL checked by `verify`
  SCHEMA_RELEASE_APP_VERSION
                         application build identifier recorded in MIGRATION.txt
  SCHEMA_RELEASE_ALLOW_ACTIVE_SESSIONS=1
                         rehearsal-only escape hatch; never use during a live cutover

`migrate` is deliberately gated by SCHEMA_RELEASE_CONFIRM=APPLY.
USAGE
}

fail() {
  echo "schema-release: $*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
project_dir=$(cd "$script_dir/../.." && pwd)
sql_dir=${SCHEMA_SQL_DIR:-$project_dir/internal/db}
init_sql=$sql_dir/init.sql
changes_dir=$sql_dir/changes
backup_root=${SCHEMA_BACKUP_DIR:-$project_dir/backups/schema}
database_url=${SCHEMA_DATABASE_URL:-${DAI_DATABASE_URL:-}}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

command_name=${1:-}
case "$command_name" in
  preflight|migrate|verify) ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [[ "$command_name" == "migrate" && "${SCHEMA_RELEASE_CONFIRM:-}" != "APPLY" ]]; then
  fail "migrate requires SCHEMA_RELEASE_CONFIRM=APPLY"
fi

[[ -n "$database_url" ]] || fail "set SCHEMA_DATABASE_URL (or DAI_DATABASE_URL)"
[[ -f "$init_sql" ]] || fail "schema baseline not found: $init_sql"
[[ -d "$changes_dir" ]] || fail "schema changes directory not found: $changes_dir"

require_command psql
if [[ "$command_name" == "migrate" || "$command_name" == "preflight" ]]; then
  require_command pg_dump
fi

psql_query() {
  psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -Atqc "$1"
}

target_version=$(grep -i 'INSERT INTO dai_schema_metadata' "$init_sql" \
  | sed -nE 's/.*VALUES[[:space:]]*\([[:space:]]*TRUE[[:space:]]*,[[:space:]]*([0-9]+)[[:space:]]*\).*/\1/p' \
  | tail -n 1)
[[ "$target_version" =~ ^[0-9]+$ ]] || fail "could not read target schema version from $init_sql"

metadata_exists=$(psql_query "SELECT to_regclass('dai_schema_metadata') IS NOT NULL")
[[ "$metadata_exists" == "t" ]] || fail "database has no dai_schema_metadata; initialize it explicitly before migrating"

current_version=$(psql_query "SELECT version FROM dai_schema_metadata WHERE singleton = TRUE")
[[ "$current_version" =~ ^[0-9]+$ ]] || fail "database has no singleton schema version"

assert_no_active_sessions() {
  if [[ "${SCHEMA_RELEASE_ALLOW_ACTIVE_SESSIONS:-0}" == "1" ]]; then
    echo "WARNING: active-session check bypassed for rehearsal" >&2
    return
  fi
  local active
  active=$(psql_query "
    SELECT count(*)
    FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
      AND backend_type = 'client backend'
  ")
  [[ "$active" == "0" ]] || fail "found $active other database session(s); stop all application instances before migration (or explicitly set SCHEMA_RELEASE_ALLOW_ACTIVE_SESSIONS=1 for a rehearsal)"
}

find_migration() {
  local version=$1
  local prefix
  prefix=$(printf '%04d_' "$version")
  local matches=()
  mapfile -t matches < <(find "$changes_dir" -maxdepth 1 -type f -name "${prefix}*.sql" -print | sort)
  [[ "${#matches[@]}" -eq 1 ]] || fail "expected exactly one migration for target v$version, found ${#matches[@]}"
  printf '%s\n' "${matches[0]}"
}

print_plan() {
  local version
  if (( current_version == target_version )); then
    echo "schema already at target v$target_version"
    return
  fi
  (( current_version < target_version )) || fail "database schema v$current_version is newer than artifact target v$target_version"
  echo "pending schema migrations: v$current_version -> v$target_version"
  for ((version=current_version + 1; version<=target_version; version++)); do
    echo "  $(basename "$(find_migration "$version")")"
  done
}

backup_database() {
  local stamp backup_dir backup_file
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  backup_dir="$backup_root/${stamp}-from-v${current_version}-to-v${target_version}"
  backup_file="$backup_dir/database.dump"
  umask 077
  mkdir -p "$backup_dir"
  echo "creating PostgreSQL custom-format backup: $backup_file" >&2
  pg_dump -Fc --no-owner --no-privileges --dbname="$database_url" >"$backup_file"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$backup_file" >"$backup_dir/SHA256SUMS"
  else
    shasum -a 256 "$backup_file" >"$backup_dir/SHA256SUMS"
  fi
  cat >"$backup_dir/MIGRATION.txt" <<EOF
created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
source_schema_version=$current_version
target_schema_version=$target_version
sql_dir=$sql_dir
database_backup=database.dump
restore_command=pg_restore --clean --if-exists --dbname=<restored-database> database.dump
application_version=${SCHEMA_RELEASE_APP_VERSION:-unknown}
EOF
  printf '%s\n' "$backup_dir"
}

write_sql_manifest() {
  local version migration manifest
  manifest="$backup_dir/SQL_SHA256SUMS"
  : >"$manifest"
  for ((version=current_version + 1; version<=target_version; version++)); do
    migration=$(find_migration "$version")
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$migration" >>"$manifest"
    else
      shasum -a 256 "$migration" >>"$manifest"
    fi
  done
}

apply_migrations() {
  local version migration observed
  for ((version=current_version + 1; version<=target_version; version++)); do
    migration=$(find_migration "$version")
    echo "applying $(basename "$migration")"
    psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" --file="$migration"
    observed=$(psql_query "SELECT version FROM dai_schema_metadata WHERE singleton = TRUE")
    [[ "$observed" == "$version" ]] || fail "$(basename "$migration") completed but database reports schema v$observed, expected v$version"
  done
}

verify_target() {
  local observed
  observed=$(psql_query "SELECT version FROM dai_schema_metadata WHERE singleton = TRUE")
  [[ "$observed" == "$target_version" ]] || fail "database schema v$observed does not match artifact target v$target_version"
  if [[ -n "${SCHEMA_RELEASE_HEALTH_URL:-}" ]]; then
    require_command curl
    curl --fail --silent --show-error "$SCHEMA_RELEASE_HEALTH_URL" >/dev/null
    echo "readiness check passed: $SCHEMA_RELEASE_HEALTH_URL"
  else
    echo "schema target verified: v$target_version (readiness URL not configured)"
  fi
}

case "$command_name" in
  preflight)
    assert_no_active_sessions
    print_plan
    ;;
  migrate)
    assert_no_active_sessions
    if (( current_version == target_version )); then
      echo "nothing to migrate; schema already at v$target_version"
      exit 0
    fi
    print_plan
    backup_dir=$(backup_database)
    write_sql_manifest
    apply_migrations
    verify_target
    printf 'completed_at=%s\nreadiness_url=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${SCHEMA_RELEASE_HEALTH_URL:-not-configured}" >>"$backup_dir/MIGRATION.txt"
    echo "migration completed; backup and restore metadata: $backup_dir"
    ;;
  verify)
    verify_target
    ;;
esac
