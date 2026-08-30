#!/usr/bin/env bash
set -euo pipefail

# Generate release evidence after the binary and Portal artifacts have been
# built. The script deliberately excludes its own checksum manifest to avoid a
# self-referential hash; SBOM and provenance are included in the final manifest.

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
project_dir=$(cd "$script_dir/.." && pwd)
release_dir=${1:-release}
version=${2:-}

if [[ "$release_dir" != /* ]]; then
  release_dir="$project_dir/$release_dir"
fi

cd "$project_dir"
if [[ -z "$version" ]]; then
  version=$(git describe --tags --always --dirty 2>/dev/null || printf '%s' dev)
fi

[[ -d "$release_dir" ]] || { echo "release metadata: missing release directory $release_dir" >&2; exit 1; }
[[ -f "$release_dir/dai" || -f "$release_dir/dai-linux-amd64" ]] || {
  echo "release metadata: no D-AI binary found in $release_dir" >&2
  exit 1
}

go run ./cmd/release_metadata -out "$release_dir" -version "$version"

manifest="$release_dir/SHA256SUMS"
tmp_manifest=$(mktemp "${TMPDIR:-/tmp}/dai-release-checksums.XXXXXX")
trap 'rm -f "$tmp_manifest"' EXIT

find "$release_dir" -type f \
  ! -name "SHA256SUMS" \
  ! -path "*/.DS_Store" \
  -print | sort | while read -r file; do
  relative=${file#"$release_dir"/}
  shasum -a 256 "$file" | awk -v name="$relative" '{ print $1 "  " name }'
done > "$tmp_manifest"
mv "$tmp_manifest" "$manifest"
trap - EXIT

echo "release metadata: wrote $release_dir/SBOM.spdx.json"
echo "release metadata: wrote $release_dir/PROVENANCE.json"
echo "release metadata: wrote $manifest"
