#!/usr/bin/env bash
set -euo pipefail

# CI/rehearsal entry point. The actual replay lives in a Go integration test so
# it uses the same pgx driver and isolated-schema fixture on macOS, CI and
# release rehearsal hosts; this wrapper only supplies strict database settings.
# The test pins the last compatible v1 baseline, 54135ad.

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
project_dir=$(cd "$script_dir/.." && pwd)
database_url=${SCHEMA_REPLAY_DATABASE_URL:-${DAI_TEST_DATABASE_URL:-}}

[[ -n "$database_url" ]] || {
  echo "schema-replay: set SCHEMA_REPLAY_DATABASE_URL (or DAI_TEST_DATABASE_URL)" >&2
  exit 1
}

command -v go >/dev/null 2>&1 || {
  echo "schema-replay: required command not found: go" >&2
  exit 1
}

cd "$project_dir"
exec env \
  DAI_TEST_DATABASE_URL="$database_url" \
  DAI_TEST_DATABASE_STRICT=1 \
  GOCACHE="${GOCACHE:-$project_dir/.cache/go-build}" \
  go test ./internal/db -run '^TestMigrationChainReplaysHistoricalV1IntoCanonicalSchema$' -count=1 -v
