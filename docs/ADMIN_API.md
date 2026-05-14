# Admin API

Management APIs are under `/admin`.

Authentication uses URM admin JWT when configured, or an optional local header token for development:

```http
X-Admin-Token: local-admin-token
```

Set it in config as `security.adminToken` or env `UNI_AI_API_ADMIN_TOKEN`.
If both `security.adminToken` and URM are empty, admin routes are open for local development.

The current database source of truth is `backend/migrations/*.sql`. This iteration intentionally changes the initial schema; rebuild local databases from scratch before using the seed.

## Dashboard

The business dashboard is AI Gateway scoped. It does not read URM recharge, grant, transaction, platform-admin, JWT-key, or system-application data.

```bash
curl 'http://127.0.0.1:13010/admin/dashboard/summary?days=1' \
  -H 'X-Admin-Token: local-admin-token'

curl 'http://127.0.0.1:13010/admin/dashboard/top-models?days=7&limit=10' \
  -H 'X-Admin-Token: local-admin-token'

curl 'http://127.0.0.1:13010/admin/dashboard/top-tenants?days=7&limit=10' \
  -H 'X-Admin-Token: local-admin-token'

curl 'http://127.0.0.1:13010/admin/dashboard/recent-errors?days=7&limit=10' \
  -H 'X-Admin-Token: local-admin-token'
```

`days=0` means all time. Tenant and user roles are automatically scoped by backend authorization.

## Providers

Create provider:

```bash
curl -X POST http://127.0.0.1:13010/admin/providers \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "code": "custom_vendor",
    "name": "Custom Vendor",
    "status": "active"
  }'
```

List providers:

```bash
curl http://127.0.0.1:13010/admin/providers \
  -H 'X-Admin-Token: local-admin-token'
```

Update provider status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/providers/{provider_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

## Endpoints

Create endpoint:

```bash
curl -X POST http://127.0.0.1:13010/admin/providers/{provider_id}/endpoints \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Custom OpenAI Chat",
    "base_url": "https://example.com/v1",
    "api_key": "upstream-provider-key",
    "weight": 100,
    "timeout_ms": 60000
  }'
```

The `api_key` is encrypted before storage. Endpoint list responses do not return it.
When editing an endpoint, omit `api_key` to keep the stored provider key; send a non-empty `api_key` only when rotating it.

List provider endpoints:

```bash
curl http://127.0.0.1:13010/admin/providers/{provider_id}/endpoints \
  -H 'X-Admin-Token: local-admin-token'
```

Update endpoint status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/providers/{provider_id}/endpoints/{endpoint_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

## Upstream Deployments

Upstream deployments define how to call a specific upstream model through an endpoint. They are independent of public models and can be reused across multiple model routes.

Create upstream deployment:

```bash
curl -X POST http://127.0.0.1:13010/admin/upstream-deployments \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "endpoint_id": "{endpoint_id}",
    "name": "DeepSeek V4 Pro",
    "upstream_model": "deepseek-v4-pro",
    "capability_type": "chat",
    "upstream_protocol": "openai_chat_completions",
    "request_path": "",
    "upstream_parameters": {
      "reasoning_effort": "high"
    },
    "tags": {"tier": "premium"},
    "status": "active"
  }'
```

List upstream deployments (supports optional `provider_id` or `endpoint_id` filter):

```bash
curl http://127.0.0.1:13010/admin/upstream-deployments \
  -H 'X-Admin-Token: local-admin-token'

curl 'http://127.0.0.1:13010/admin/upstream-deployments?provider_id={provider_id}' \
  -H 'X-Admin-Token: local-admin-token'
```

Update upstream deployment:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/upstream-deployments/{deployment_id} \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"upstream_parameters": {"reasoning_effort": "medium"}}'
```

Update upstream deployment status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/upstream-deployments/{deployment_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

Run a deployment health check. Active probes are supported for `openai_chat_completions`, `openai_responses`, and `openai_embeddings`; image, video, audio, and rerank deployments return an unsupported probe error without marking the deployment unhealthy.

```bash
curl -X POST http://127.0.0.1:13010/admin/upstream-deployments/{deployment_id}/health-check \
  -H 'X-Admin-Token: local-admin-token'
```

## Models

Create public model:

```bash
curl -X POST http://127.0.0.1:13010/admin/models \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model_code": "my-chat-model",
    "display_name": "My Chat Model",
    "capability_type": "chat",
    "default_max_output_tokens": 4096
  }'
```

List models:

```bash
curl http://127.0.0.1:13010/admin/models \
  -H 'X-Admin-Token: local-admin-token'
```

Update model status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/models/{model_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "inactive"}'
```

## Model Routes

Model routes connect public models to upstream deployments. Each route defines priority, weight, and streaming support.

Create model route:

```bash
curl -X POST http://127.0.0.1:13010/admin/models/{model_id}/routes \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "upstream_deployment_id": "{deployment_id}",
    "priority": 100,
    "weight": 100,
    "supports_stream": true,
    "status": "active"
  }'
```

List model routes:

```bash
curl http://127.0.0.1:13010/admin/models/{model_id}/routes \
  -H 'X-Admin-Token: local-admin-token'
```

Update model route:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/models/{model_id}/routes/{route_id} \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"priority": 50, "weight": 200}'
```

Update model route status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/models/{model_id}/routes/{route_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

Delete model route:

```bash
curl -X DELETE http://127.0.0.1:13010/admin/models/{model_id}/routes/{route_id} \
  -H 'X-Admin-Token: local-admin-token'
```

## Model Prices

Model prices define tenant sale prices. All price fields are non-negative integer credits. Token prices are credits per 1M tokens; image prices are credits per generated image. `effective_from` and other admin timestamps use Unix milliseconds in API requests and responses.

Create a model sale price:

```bash
curl -X POST http://127.0.0.1:13010/admin/models/{model_id}/prices \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_input_price_per_1m": 1500,
    "tenant_output_price_per_1m": 3000,
    "tenant_image_price": 80,
    "effective_from": 1777248000000,
    "status": "active"
  }'
```

List model prices:

```bash
curl http://127.0.0.1:13010/admin/models/{model_id}/prices \
  -H 'X-Admin-Token: local-admin-token'
```

Update model price status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/models/{model_id}/prices/{price_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "inactive"}'
```

## Upstream Deployment Cost Prices

Upstream deployment cost prices define provider-side costs for audit and margin reporting. This is not used for user billing. All cost fields are non-negative integer credits.

Create a deployment cost price:

```bash
curl -X POST http://127.0.0.1:13010/admin/upstream-deployments/{deployment_id}/cost-prices \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "capability_type": "chat",
    "currency": "CNY_CREDITS",
    "input_cost_per_1m": 800,
    "output_cost_per_1m": 1600,
    "request_cost": 0,
    "image_cost": 0,
    "image_size_prices": {"256x256": 1, "512x512": 2, "1024x1024": 4},
    "effective_from": 1777248000000,
    "status": "active"
  }'
```

List deployment cost prices:

```bash
curl http://127.0.0.1:13010/admin/upstream-deployments/{deployment_id}/cost-prices \
  -H 'X-Admin-Token: local-admin-token'
```

Update deployment cost price status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/upstream-deployments/{deployment_id}/cost-prices/{price_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "inactive"}'
```

## Tenant Grants

Grant a model to a tenant:

```bash
curl -X POST http://127.0.0.1:13010/admin/tenants/tenant-local/model-grants \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model_id": "{model_id}",
    "status": "active",
    "created_by": "admin"
  }'
```

List tenant model grants:

```bash
curl http://127.0.0.1:13010/admin/tenants/tenant-local/model-grants \
  -H 'X-Admin-Token: local-admin-token'
```

Update tenant model grant status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/tenants/tenant-local/model-grants/{model_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

## Tenant API Keys

Create a tenant-owned runtime API key:

```bash
curl -X POST http://127.0.0.1:13010/admin/tenants/tenant-local/api-keys \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Tenant test key",
    "quota_limit": 1000000000,
    "allowed_models": ["deepseek-v4-pro"],
    "created_by": "admin"
  }'
```

The response includes the plaintext `api_key` once. It is not stored.

List tenant-owned API keys:

```bash
curl http://127.0.0.1:13010/admin/tenants/tenant-local/api-keys \
  -H 'X-Admin-Token: local-admin-token'
```

Update tenant-owned API key status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/tenants/tenant-local/api-keys/{api_key_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

Supported status values are `active`, `inactive`, and `disabled`.

## User API Keys

Create a user-owned runtime API key. User-owned keys require an active tenant grant for the requested model.

```bash
curl -X POST http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/api-keys \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "User test key",
    "quota_limit": 100000000,
    "allowed_models": ["deepseek-v4-pro"],
    "created_by": "admin"
  }'
```

The response includes the plaintext `api_key` once.

List user-owned API keys:

```bash
curl http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/api-keys \
  -H 'X-Admin-Token: local-admin-token'
```

Update user-owned API key status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/api-keys/{api_key_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

## Usage Logs

Runtime paths currently supported:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/embeddings`
- `POST /v1/images/generations`
- `GET /v1/models`

List latest usage logs:

```bash
curl 'http://127.0.0.1:13010/admin/usage-logs?limit=100' \
  -H 'X-Admin-Token: local-admin-token'
```

`limit` defaults to `100` and is capped at `500`.

List usage summary grouped by capability, billable unit, and request status:

```bash
curl 'http://127.0.0.1:13010/admin/usage-summary' \
  -H 'X-Admin-Token: local-admin-token'
```

List usage summary grouped only by billable unit type:

```bash
curl 'http://127.0.0.1:13010/admin/usage-unit-summary' \
  -H 'X-Admin-Token: local-admin-token'
```

All three endpoints support `tenant_id`, `user_id`, `model_code`, and `request_status` filters. Summary totals are calculated across all matching rows, not only the latest `limit` rows.

Usage rows include token fields plus unified billable units:

- Chat/Responses: `billable_unit_type=token`, `billable_units=total_tokens`.
- Embeddings: `billable_unit_type=input_token`, `billable_units=prompt_tokens`.
- Images: `billable_unit_type=image`, `billable_units=image_count`.
- `usage_source=upstream` means provider usage was available; `estimated_length` marks fallback estimation.

Cost fields in usage logs are integer credits:

- Token capabilities calculate token costs from prompt/completion tokens and per-1M-token prices, rounded up to whole credits.
- Image capability calculates `api_key_quota_cost` and `user_cost` as `image_count * image_price` (using tenant sale price).
- Provider image cost is recorded from `image_size_prices` based on the requested image size.

## Runtime Limits

Runtime limits are configured only through `/admin/limit-policies`; legacy API-key and endpoint limit columns were removed.

- Supported scopes: `tenant`, `user`, `api_key`, `provider`, `endpoint`.
- Supported metrics: `rpm_limit`, `tpm_limit`, and `concurrency_limit`.
- Policies are additive; any matching active policy can reject the request.
- `quota_limit` is checked before chat calls.
- Token calls reserve estimated quota in Redis before calling upstream.
- The reservation estimate uses request `max_tokens`, `max_completion_tokens`, `max_output_tokens`, or the model `default_max_output_tokens`.
- After a successful upstream response, actual `usage` tokens confirm `quota_used` in PostgreSQL.

Redis keys:

```text
uni_ai_api:rate:{scope_type}:{scope_id}:{capability_type}:...
uni_ai_api:quota:key:{api_key_id}:reserved
```

Deployment failures are tracked in Redis:

```text
uni_ai_api:deployment:{deployment_id}:cooldown
```

Runtime routing skips deployments in cooldown. The current cooldown TTL is 60 seconds. Upstream network errors, HTTP 429, and HTTP 5xx mark a deployment as cooling down. HTTP 4xx other than 429 does not, because it is usually a request or credential problem rather than provider capacity.

Route-based routing:

- Resolve public model → get active model routes.
- Filter out deployments in Redis cooldown.
- Use the lowest `priority` value among remaining routes.
- Within that priority group, choose by `route.weight * endpoint.weight` weighted random.
- Conversation stickiness binds to `upstream_deployment_id` instead of deployment.
