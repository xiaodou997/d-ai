#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  smoke_release.sh <public-base-url> [management-base-url]

The public URL is checked for /health, Portal /, and /api/v1/info. The
management URL is checked for /health, /ready, and /metrics. Set
DAI_RELEASE_SMOKE_STREAM_URL and DAI_RELEASE_SMOKE_BEARER_TOKEN to run an
authenticated streaming smoke as well; the URL must point at a test-safe
chat completion route and DAI_RELEASE_SMOKE_STREAM_PAYLOAD may override the
default request body.
USAGE
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

public_url=${1:-${DAI_RELEASE_SMOKE_PUBLIC_URL:-}}
management_url=${2:-${DAI_RELEASE_SMOKE_MANAGEMENT_URL:-}}

[[ -n "$public_url" ]] || { usage >&2; exit 2; }
public_url=${public_url%/}

curl_json() {
  curl --fail --silent --show-error --max-time "${DAI_RELEASE_SMOKE_TIMEOUT_SECONDS:-10}" \
    -H 'Accept: application/json' "$1"
}

echo "release smoke: public health"
health=$(curl_json "$public_url/health")
grep -q '"status"[[:space:]]*:[[:space:]]*"ok"' <<<"$health" || {
  echo "release smoke: public health did not report ok" >&2
  exit 1
}

echo "release smoke: embedded Portal"
portal=$(curl --fail --silent --show-error --max-time "${DAI_RELEASE_SMOKE_TIMEOUT_SECONDS:-10}" "$public_url/")
grep -q 'id="app"' <<<"$portal" || {
  echo "release smoke: public root did not return Portal application HTML" >&2
  exit 1
}

echo "release smoke: public API info"
curl_json "$public_url/api/v1/info" >/dev/null

if [[ -n "$management_url" ]]; then
  management_url=${management_url%/}
  echo "release smoke: management health/readiness/metrics"
  curl_json "$management_url/health" >/dev/null
  curl_json "$management_url/ready" >/dev/null
  curl --fail --silent --show-error --max-time "${DAI_RELEASE_SMOKE_TIMEOUT_SECONDS:-10}" \
    -H 'Accept: text/plain' "$management_url/metrics" | grep -q '^# HELP ' || {
      echo "release smoke: management metrics did not return Prometheus text" >&2
      exit 1
    }
else
  echo "release smoke: management URL not supplied; management checks deferred to deployment runbook" >&2
fi

if [[ -n "${DAI_RELEASE_SMOKE_STREAM_URL:-}" ]]; then
  [[ -n "${DAI_RELEASE_SMOKE_BEARER_TOKEN:-}" ]] || {
    echo "release smoke: DAI_RELEASE_SMOKE_BEARER_TOKEN is required for stream smoke" >&2
    exit 2
  }
  payload=${DAI_RELEASE_SMOKE_STREAM_PAYLOAD:-'{"model":"smoke","messages":[{"role":"user","content":"ping"}],"stream":true}'}
  echo "release smoke: authenticated stream"
  stream_output=$(curl --fail --silent --show-error --max-time "${DAI_RELEASE_SMOKE_STREAM_TIMEOUT_SECONDS:-30}" \
    -N -H "Authorization: Bearer ${DAI_RELEASE_SMOKE_BEARER_TOKEN}" \
    -H 'Content-Type: application/json' -H 'Accept: text/event-stream' \
    --data "$payload" "$DAI_RELEASE_SMOKE_STREAM_URL") || {
    echo "release smoke: stream request failed" >&2
    exit 1
  }
  grep -q '^data:' <<<"$stream_output" || {
      echo "release smoke: stream response did not contain an SSE data frame" >&2
      exit 1
    }
fi

echo "release smoke: passed"
