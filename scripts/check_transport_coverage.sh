#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"
cd "${repo_root}"

# Keep the gate explicit and ratchetable. The current suite is 10.4%; a later
# test-only change can raise this without changing the script or CI wiring.
minimum_percent="${TRANSPORT_COVERAGE_MIN:-10.0}"
transport_cache="${GOCACHE:-${repo_root}/.cache/go-build}"
profile_file="$(mktemp "${TMPDIR:-/tmp}/dai-transport-cover.XXXXXX")"
trap 'rm -f "${profile_file}"' EXIT

test_output="$(GOCACHE="${transport_cache}" go test ./internal/transport \
  -covermode=atomic -coverprofile="${profile_file}" -count=1 2>&1)" || {
	printf '%s\n' "${test_output}"
	exit 1
}
printf '%s\n' "${test_output}"

actual_percent="$(printf '%s\n' "${test_output}" | awk '
  /coverage:/ {
    for (i = 1; i <= NF; i++) {
      if ($i == "coverage:") {
        value = $(i + 1)
        gsub(/%/, "", value)
        print value
        exit
      }
    }
  }
')"
if [[ -z "${actual_percent}" ]]; then
	echo "transport coverage: could not parse go test coverage output" >&2
	exit 1
fi

if ! awk -v actual="${actual_percent}" -v minimum="${minimum_percent}" \
  'BEGIN { exit ((actual + 0) >= (minimum + 0)) ? 0 : 1 }'; then
	echo "transport coverage: ${actual_percent}% is below ${minimum_percent}%" >&2
	exit 1
fi

printf 'transport coverage: %s%% (minimum %s%%)\n' "${actual_percent}" "${minimum_percent}"
