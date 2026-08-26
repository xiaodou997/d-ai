#!/usr/bin/env bash
set -euo pipefail

# Create or rotate the two least-privilege LOGIN roles used by the application.
# Passwords are read from the environment so a secret manager can inject them
# without putting them in shell history or the psql command line.

usage() {
  cat <<'USAGE'
Usage:
  provision_db_roles.sh preflight   # inspect role readiness without changing it
  provision_db_roles.sh apply       # create/rotate roles (explicitly gated)
  provision_db_roles.sh --help

Required for both commands:
  DB_ROLE_PROVISION_DATABASE_URL  PostgreSQL admin/owner URL

Required for apply (or the compose equivalents):
  DB_ROLE_PROVISION_RUNTIME_PASSWORD  password for the runtime role
  DB_ROLE_PROVISION_BILLING_PASSWORD  password for the billing role
  DB_ROLE_PROVISION_CONFIRM=APPLY     explicit provisioning confirmation

Optional:
  DB_ROLE_PROVISION_RUNTIME_ROLE default: dai
  DB_ROLE_PROVISION_BILLING_ROLE default: dai_billing

The password variables fall back to DAI_DATABASE_PASSWORD and
DAI_BILLING_DATABASE_PASSWORD so a deployment secret bundle can be reused.
USAGE
}

fail() {
  echo "db-role-provision: $*" >&2
  exit 1
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

command_name=${1:-}
case "$command_name" in
  preflight|apply) ;;
  *)
    usage >&2
    exit 2
    ;;
esac

database_url=${DB_ROLE_PROVISION_DATABASE_URL:-${SCHEMA_OWNERSHIP_DATABASE_URL:-}}
runtime_role=${DB_ROLE_PROVISION_RUNTIME_ROLE:-dai}
billing_role=${DB_ROLE_PROVISION_BILLING_ROLE:-dai_billing}
runtime_password=${DB_ROLE_PROVISION_RUNTIME_PASSWORD:-${DAI_DATABASE_PASSWORD:-}}
billing_password=${DB_ROLE_PROVISION_BILLING_PASSWORD:-${DAI_BILLING_DATABASE_PASSWORD:-}}

[[ -n "$database_url" ]] || fail "set DB_ROLE_PROVISION_DATABASE_URL (or SCHEMA_OWNERSHIP_DATABASE_URL)"
for identifier in "$runtime_role" "$billing_role"; do
  [[ "$identifier" =~ ^[a-z_][a-z0-9_]*$ ]] || fail "invalid PostgreSQL identifier: $identifier"
done
[[ "$runtime_role" != "$billing_role" ]] || fail "runtime and billing roles must be different"

psql_query() {
  psql -X -v ON_ERROR_STOP=1 --dbname="$database_url" -Atqc "$1"
}

if [[ "$command_name" == "preflight" ]]; then
  command -v psql >/dev/null 2>&1 || fail "required command not found: psql"
  role_count=$(psql_query "
    SELECT count(*)
    FROM pg_roles
    WHERE rolname IN ('$runtime_role', '$billing_role')
  ")
  psql_query "
    SELECT rolname || E'\\t' ||
           CASE WHEN rolcanlogin THEN 'login' ELSE 'no-login' END || E'\\t' ||
           CASE WHEN rolinherit THEN 'inherit' ELSE 'no-inherit' END || E'\\t' ||
           CASE WHEN rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls
                THEN 'privileged' ELSE 'least-privilege' END
    FROM pg_roles
    WHERE rolname IN ('$runtime_role', '$billing_role')
    ORDER BY rolname
  "
  if [[ "$role_count" != "2" ]]; then
    echo "db-role-provision: one or more target roles are missing; run apply during the provisioning window" >&2
    exit 0
  fi
  unsafe_count=$(psql_query "
    SELECT count(*)
    FROM pg_roles
    WHERE rolname IN ('$runtime_role', '$billing_role')
      AND (NOT rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls OR rolinherit)
  ")
  [[ "$unsafe_count" == "0" ]] || fail "target roles are not ready for application use; inspect role attributes above"
  membership_count=$(psql_query "
    SELECT count(*)
    FROM pg_auth_members m
    JOIN pg_roles member ON member.oid = m.member
    JOIN pg_roles parent ON parent.oid = m.roleid
    WHERE member.rolname IN ('$runtime_role', '$billing_role')
      AND parent.rolname IN ('$runtime_role', '$billing_role')
  ")
  [[ "$membership_count" == "0" ]] || fail "runtime and billing roles must not be members of one another"
  echo "db-role-provision: roles are provisioned with least-privilege attributes"
  exit 0
fi

[[ "${DB_ROLE_PROVISION_CONFIRM:-}" == "APPLY" ]] || fail "apply requires DB_ROLE_PROVISION_CONFIRM=APPLY"
[[ -n "$runtime_password" ]] || fail "set runtime role password through the deployment secret manager"
[[ -n "$billing_password" ]] || fail "set billing role password through the deployment secret manager"
[[ "$runtime_password" != "$billing_password" ]] || fail "runtime and billing passwords must be different"
case "$runtime_password" in
  replace-with-*|change-me*|password) fail "runtime role password is a placeholder" ;;
esac
case "$billing_password" in
  replace-with-*|change-me*|password) fail "billing role password is a placeholder" ;;
esac
for password_label in runtime billing; do
  password_value=$runtime_password
  if [[ "$password_label" == "billing" ]]; then
    password_value=$billing_password
  fi
  [[ "${#password_value}" -ge 32 ]] || fail "$password_label role password must be at least 32 characters"
  [[ "$password_value" != *[[:space:]]* ]] || fail "$password_label role password must not contain whitespace"
done
command -v psql >/dev/null 2>&1 || fail "required command not found: psql"

membership_count=$(psql_query "
  SELECT count(*)
  FROM pg_auth_members m
  JOIN pg_roles member ON member.oid = m.member
  WHERE member.rolname IN ('$runtime_role', '$billing_role')
")
[[ "$membership_count" == "0" ]] || fail "target roles have role memberships; revoke them before provisioning"

# Keep secrets out of argv. psql's \getenv reads the values from the child
# environment and the SQL only receives them as local variables in this DO.
DB_ROLE_PROVISION_RUNTIME_PASSWORD="$runtime_password" \
DB_ROLE_PROVISION_BILLING_PASSWORD="$billing_password" \
psql -X -v ON_ERROR_STOP=1 \
  --dbname="$database_url" \
  -v runtime_role="$runtime_role" \
  -v billing_role="$billing_role" \
  --file=- <<'SQL'
\set ON_ERROR_STOP on
\getenv runtime_password DB_ROLE_PROVISION_RUNTIME_PASSWORD
\getenv billing_password DB_ROLE_PROVISION_BILLING_PASSWORD

DO $provision$
DECLARE
  runtime_name TEXT := :'runtime_role';
  billing_name TEXT := :'billing_role';
  runtime_secret TEXT := :'runtime_password';
  billing_secret TEXT := :'billing_password';
BEGIN
  IF runtime_name = current_user OR billing_name = current_user THEN
    RAISE EXCEPTION 'refusing to alter the current database role';
  END IF;
  IF runtime_secret = billing_secret THEN
    RAISE EXCEPTION 'runtime and billing passwords must be different';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = runtime_name) THEN
    EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', runtime_name, runtime_secret);
  ELSE
    EXECUTE format('ALTER ROLE %I LOGIN PASSWORD %L', runtime_name, runtime_secret);
  END IF;
  EXECUTE format('ALTER ROLE %I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT', runtime_name);

  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = billing_name) THEN
    EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', billing_name, billing_secret);
  ELSE
    EXECUTE format('ALTER ROLE %I LOGIN PASSWORD %L', billing_name, billing_secret);
  END IF;
  EXECUTE format('ALTER ROLE %I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT', billing_name);

  EXECUTE format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), runtime_name);
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), billing_name);
END
$provision$;
SQL

role_count=$(psql_query "
  SELECT count(*)
  FROM pg_roles
  WHERE rolname IN ('$runtime_role', '$billing_role')
    AND rolcanlogin
    AND NOT rolsuper
    AND NOT rolcreaterole
    AND NOT rolcreatedb
    AND NOT rolreplication
    AND NOT rolbypassrls
    AND NOT rolinherit
")
[[ "$role_count" == "2" ]] || fail "role provisioning completed but target attributes did not verify"

echo "db-role-provision: provisioned runtime=$runtime_role billing=$billing_role"
