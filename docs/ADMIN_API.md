# Admin API

Management APIs are under `/admin`.

MVP authentication uses an optional header token:

```http
X-Admin-Token: local-admin-token
```

Set it in config as `security.adminToken` or env `UNI_AI_API_ADMIN_TOKEN`.
If it is empty, admin routes are open for local development. This will be replaced by URM JWT middleware.

## Providers

Create provider:

```bash
curl -X POST http://127.0.0.1:13010/admin/providers \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "code": "custom_vendor",
    "name": "Custom Vendor",
    "provider_type": "custom",
    "protocol_type": "openai_chat_completions",
    "is_custom": true
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
    "protocol_type": "openai_chat_completions",
    "api_key": "upstream-provider-key",
    "weight": 100,
    "timeout_ms": 60000
  }'
```

The `api_key` is encrypted before storage. Endpoint list responses do not return it.

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

## Deployments

Create deployment:

```bash
curl -X POST http://127.0.0.1:13010/admin/models/{model_id}/deployments \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "endpoint_id": "{endpoint_id}",
    "upstream_model": "provider-model-name",
    "capability_type": "chat",
    "upstream_protocol": "openai_chat_completions",
    "upstream_parameters": {
      "reasoning_effort": "high"
    },
    "priority": 100,
    "weight": 100,
    "supports_stream": true
  }'
```

List deployments:

```bash
curl http://127.0.0.1:13010/admin/models/{model_id}/deployments \
  -H 'X-Admin-Token: local-admin-token'
```

Update deployment status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/models/{model_id}/deployments/{deployment_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

## Model Prices

Create a model sale price. Unit is credits per 1M tokens.

```bash
curl -X POST http://127.0.0.1:13010/admin/models/{model_id}/prices \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "platform_input_price_per_1m": 1000,
    "platform_output_price_per_1m": 2000,
    "tenant_input_price_per_1m": 1500,
    "tenant_output_price_per_1m": 3000,
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

## Provider Cost Prices

Create a provider model cost price. This is for audit and margin reporting, not user billing.

```bash
curl -X POST http://127.0.0.1:13010/admin/providers/{provider_id}/model-prices \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "endpoint_id": "{optional_endpoint_id}",
    "upstream_model": "provider-model-name",
    "capability_type": "chat",
    "currency": "CNY_CREDITS",
    "input_cost_per_1m": 800,
    "output_cost_per_1m": 1600,
    "request_cost": 0,
    "status": "active"
  }'
```

List provider model cost prices:

```bash
curl http://127.0.0.1:13010/admin/providers/{provider_id}/model-prices \
  -H 'X-Admin-Token: local-admin-token'
```

Update provider cost price status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/providers/{provider_id}/model-prices/{price_id}/status \
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
    "rpm_limit": 60,
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

## User Grants

Grant a model to a user under a tenant:

```bash
curl -X POST http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/model-grants \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model_id": "{model_id}",
    "status": "active",
    "created_by": "admin"
  }'
```

List user model grants:

```bash
curl http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/model-grants \
  -H 'X-Admin-Token: local-admin-token'
```

Update user model grant status:

```bash
curl -X PATCH http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/model-grants/{model_id}/status \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"status": "disabled"}'
```

## User API Keys

Create a user-owned runtime API key:

```bash
curl -X POST http://127.0.0.1:13010/admin/tenants/tenant-local/users/user-local/api-keys \
  -H 'X-Admin-Token: local-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "User test key",
    "quota_limit": 100000000,
    "allowed_models": ["deepseek-v4-pro"],
    "rpm_limit": 30,
    "created_by": "admin"
  }'
```

The response includes the plaintext `api_key` once. User-owned keys require both an active tenant grant and an active user grant for the requested model.

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

List latest usage logs:

```bash
curl 'http://127.0.0.1:13010/admin/usage-logs?limit=100' \
  -H 'X-Admin-Token: local-admin-token'
```

`limit` defaults to `100` and is capped at `500`.

## Runtime Limits

Runtime API keys support RPM limiting and quota fields:

- `rpm_limit` is enforced with Redis per API key per minute.
- `quota_limit` is checked before chat calls.
- Chat calls reserve an estimated output-token quota in Redis before calling upstream.
- The reservation estimate uses request `max_tokens`, `max_completion_tokens`, or the model `default_max_output_tokens`.
- After a successful upstream response, actual `usage` tokens confirm `quota_used` in PostgreSQL.

Redis keys:

```text
uni_ai_api:rate:key:{api_key_id}:rpm:{minute}
uni_ai_api:quota:key:{api_key_id}:reserved
```

Endpoint failures are also tracked in Redis:

```text
uni_ai_api:endpoint:{endpoint_id}:cooldown
```

Runtime routing skips endpoints in cooldown. The current cooldown TTL is 60 seconds. Upstream network errors, HTTP 429, and HTTP 5xx mark an endpoint as cooling down. HTTP 4xx other than 429 does not, because it is usually a request or credential problem rather than provider capacity.

Deployment routing:

- Filter out endpoints in Redis cooldown.
- Use the lowest `priority` value among remaining deployments.
- Within that priority group, choose by `deployment.weight * endpoint.weight`.
