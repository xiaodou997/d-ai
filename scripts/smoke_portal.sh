#!/usr/bin/env bash
set -euo pipefail

dist=${1:-release/portal}
if [[ ! -f "$dist/index.html" ]]; then
  echo "portal smoke: missing $dist/index.html" >&2
  exit 1
fi

grep -q '<div id="app"></div>' "$dist/index.html" || {
  echo "portal smoke: index.html does not contain the application root" >&2
  exit 1
}
asset=$(grep -oE 'assets/[^" ]+\.(js|css)' "$dist/index.html" | sed -n '1p' || true)
[[ -n "$asset" && -f "$dist/$asset" ]] || {
  echo "portal smoke: index.html references no existing hashed asset" >&2
  exit 1
}
[[ -f "$dist/SHA256SUMS" ]] || {
  echo "portal smoke: checksum manifest is missing" >&2
  exit 1
}
echo "portal smoke: static artifact passed ($dist)"
