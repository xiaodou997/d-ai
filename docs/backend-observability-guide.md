# Backend Observability Guide

## Configuration

Backend observability is configured with the `server`, `logging`, and `observability` blocks.

```yaml
app:
  env: development
  serviceName: uni-ai-api
  version: dev

server:
  httpAddr: ":13010"

logging:
  level: debug
  format: console
  accessLog: true
  slowRequestMs: 1000

metrics:
  enabled: true
  path: /metrics
```

Legacy `app.httpAddr` and `app.logLevel` are intentionally removed. Use `server.httpAddr` and `logging.level`.

## Local Run

```bash
cd uni-ai-api/backend
make run
```

Expected startup logs include:

- loaded config path
- effective environment, log level, log format and HTTP address
- Postgres connection status
- Redis connection status or disabled status
- URM client configuration
- metrics path
- server listen address

## Log Formats

Use `logging.format: console` for local development.

Use `logging.format: json` for production log collectors.

The logger always attaches:

- `service`
- `env`
- `version`
- `pid`

## Safety Defaults

Sensitive field redaction is always enabled in code. Do not add deployment config for redaction unless there is a concrete compliance requirement.

The backend never logs raw provider keys, API keys, bearer tokens, URM secrets, request prompts, or complete upstream payloads by default.

Upstream error bodies are truncated to 2KB and redacted before logging.

## Request Logs

Every HTTP request gets an `X-Request-ID` response header and a structured access log containing:

- `request_id`
- `method`
- `path`
- `raw_path`
- `status`
- `duration_ms`
- `bytes`
- `remote_ip`
- `user_agent`
- auth context when available

Requests slower than `logging.slowRequestMs` are also logged as `slow http request`.

## Gateway Debug Logs

When `logging.level: debug`, gateway calls emit key routing events:

- `gateway request received`
- `gateway route selected`
- `gateway upstream request started`

These events include public model, capability, provider, endpoint, deployment, upstream protocol, upstream model, request path and request id.

## Metrics

When enabled, metrics are exposed at `observability.metrics.path`, default `/metrics`.

Current metrics:

- `http_requests_total`
- `http_request_duration_seconds_sum`
- `http_request_duration_seconds_count`
- `gateway_requests_total`
- `gateway_request_duration_seconds_sum`
- `gateway_request_duration_seconds_count`
- `gateway_upstream_requests_total`
- `gateway_upstream_duration_seconds_sum`
- `gateway_upstream_duration_seconds_count`
- `gateway_settlements_total`

Keep high-cardinality values such as tenant IDs and user IDs out of metric labels.

## Troubleshooting

If `make run` only prints a config error, check:

- `UNI_AI_API_CONFIG`
- `logging.level`
- `logging.format`
- `server.httpAddr`
- `postgres.dsn`
- `security.providerKeyMaster`

If `logging.level: debug` does not show gateway debug logs, verify that a `/v1/*` runtime request actually reaches the backend and passes API key authentication.
