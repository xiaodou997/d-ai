# Local Smoke

This smoke path uses PostgreSQL, Redis, URM on `http://127.0.0.1:6900`, the fake upstream on `:18080`, and the backend on `:13010`.

## Setup

```bash
cp ai-service/config.local.example.yaml ai-service/config.local.yaml
```

Initialize schema and local data:

```bash
cd backend
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/migrate up
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/seed
```

Start the fake upstream:

```bash
cd backend
go run ./cmd/fake-upstream
```

Start the backend in another shell:

```bash
cd backend
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/server
```

Runtime key:

```text
sk-ai-local-dev
```

## Requests

Ready check:

```bash
curl http://127.0.0.1:13010/ready
```

Models:

```bash
curl http://127.0.0.1:13010/v1/models \
  -H 'Authorization: Bearer sk-ai-local-dev'
```

Chat non-stream:

```bash
curl http://127.0.0.1:13010/v1/chat/completions \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{"model":"local-chat-test","messages":[{"role":"user","content":"hello"}]}'
```

Chat stream:

```bash
curl -N http://127.0.0.1:13010/v1/chat/completions \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{"model":"local-chat-test","messages":[{"role":"user","content":"hello"}],"stream":true}'
```

Responses non-stream:

```bash
curl http://127.0.0.1:13010/v1/responses \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{"model":"local-responses-test","input":"hello"}'
```

Responses stream:

```bash
curl -N http://127.0.0.1:13010/v1/responses \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{"model":"local-responses-test","input":"hello","stream":true}'
```

Embeddings:

```bash
curl http://127.0.0.1:13010/v1/embeddings \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{"model":"local-embedding-test","input":["hello"]}'
```

Images:

```bash
curl http://127.0.0.1:13010/v1/images/generations \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{"model":"local-image-test","prompt":"local smoke image","n":1}'
```

## Usage Log Checks

In Navicat:

```sql
SELECT model_code,
       capability_type,
       stream,
       billable_unit_type,
       billable_units,
       billing_status,
       request_status,
       prompt_tokens,
       completion_tokens,
       total_tokens
FROM ai_usage_logs
WHERE tenant_id = 'tenant-local'
ORDER BY created_at DESC
LIMIT 20;
```

Expected billable units:

- `local-chat-test` and `local-responses-test`: `billable_unit_type = token`, `billable_units = total_tokens`
- `local-embedding-test`: `billable_unit_type = input_token`, `billable_units = prompt_tokens`
- `local-image-test`: `billable_unit_type = image`, `billable_units = image_count`

All cost columns are micro-credits in storage and decimal credits in API responses. For image calls, the local seed charges one credit per generated image, so `api_key_quota_cost`, `platform_cost`, and `user_cost` should match the requested image count for user-owned keys.

With URM running on `:6900`, successful billed requests should move through `confirmed`. If URM is unavailable, use the zero-price bypass in `ai-service/seeds/README.md`.

To verify URM `Cancel`, stop the fake upstream and send one chat request. The backend should return an upstream error and record a usage row with `billing_status = canceled`.
