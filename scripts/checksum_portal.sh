#!/usr/bin/env bash
set -euo pipefail

dist=${1:-release/portal}
if [[ ! -d "$dist" || ! -f "$dist/index.html" ]]; then
  echo "portal checksum: missing static Portal directory or index.html: $dist" >&2
  exit 1
fi

manifest="$dist/SHA256SUMS"
tmp="${manifest}.tmp"
trap 'rm -f "$tmp"' EXIT

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$dist" && find . -type f ! -name SHA256SUMS | sort | while IFS= read -r file; do sha256sum "$file"; done) >"$tmp"
else
  (cd "$dist" && find . -type f ! -name SHA256SUMS | sort | while IFS= read -r file; do shasum -a 256 "$file"; done) >"$tmp"
fi
mv "$tmp" "$manifest"
echo "portal checksum: wrote $manifest"
