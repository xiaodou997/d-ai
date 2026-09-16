#!/usr/bin/env bash
# Run once with the old server stopped, before starting the fixed build.
# Accepts ordinary redis-cli connection arguments (e.g. -h redis -p 6379 -n 1).
# Supply passwords through REDISCLI_AUTH, not command-line arguments.
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
key_file="$(mktemp)"
trap 'rm -f "$key_file"' EXIT
redis-cli -e "$@" --scan --pattern 'dai:availability:v2:scope:*' > "$key_file"
changed=0
while IFS= read -r state_key; do
  [[ -n "$state_key" ]] || continue
  result="$(redis-cli -e "$@" --raw --eval "$script_dir/cleanup_upstream_empty_history.lua" "$state_key")"
  case "$result" in
    0) ;;
    1) changed=$((changed + 1)) ;;
    *) printf 'Unexpected cleanup result: %s\n' "$result" >&2; exit 1 ;;
  esac
done < "$key_file"
printf 'Cleaned %d empty upstream histories; cooldowns, leases and TTLs preserved.\n' "$changed"
