#!/usr/bin/env bash
set -euo pipefail

binary=${1:-release/dai}
[[ -x "$binary" || -f "$binary" ]] || {
  echo "embed Portal smoke: missing binary $binary" >&2
  exit 1
}

strings "$binary" | grep -F '<div id="app"></div>' >/dev/null || {
  echo "embed Portal smoke: binary does not contain embedded Portal application root" >&2
  exit 1
}
echo "embed Portal smoke: embedded artifact marker found ($binary)"
