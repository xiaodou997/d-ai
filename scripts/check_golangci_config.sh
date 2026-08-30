#!/usr/bin/env bash
set -euo pipefail

config=${1:-.golangci.yml}
[[ -f "$config" ]] || { echo "golangci config: missing $config" >&2; exit 1; }

# Broad directory/file exclusions make a green lint result meaningless. Any
# future exception must be a specific, documented linter rule instead.
if grep -Eq '^[[:space:]]*(skip-dirs|skip-files|exclude-dirs|exclude-files):' "$config"; then
  echo "golangci config: broad directory/file exclusions are forbidden" >&2
  exit 1
fi
if grep -Eq '^[[:space:]]*exclude:[[:space:]]*$' "$config"; then
  echo "golangci config: global issue exclusions are forbidden; use specific rule exclusions" >&2
  exit 1
fi

grep -q '^version: "2"$' "$config" || {
  echo "golangci config: v2 configuration is required" >&2
  exit 1
}
echo "golangci config: focused v2 configuration passed"
