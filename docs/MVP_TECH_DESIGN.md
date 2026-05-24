# Uni AI API Gateway Technical Design

This document describes the current operating baseline. Local databases must be rebuilt from `ai-service/migrations/*.sql`; historical migration compatibility is not maintained in this iteration.

## 1. Goal

`uni-ai-api` is a dedicated AI API gateway. It exposes an OpenAI-compatible API to business users while hiding provider differences, endpoint routing, billing, quotas, and model authorization.

It is not a generic HTTP proxy. The domain model is AI-specific: provider, endpoint, upstream deployment, model, model route, price, API key, usage, and settlement.

## 2. Current Scope

Supported runtime APIs:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/embeddings`
- `POST /v1/images/generations`
- `GET /v1/models`
- OpenAI-compatible Chat Completions, Responses, Embeddings, and Images upstreams
- Canonical model mapping from public model code to provider upstream model name via model routes
- Upstream deployment cost prices for audit and margin reports
- Streaming and non-streaming chat/responses
- PostgreSQL for persistent configuration, quota ledgers, and usage logs
- Redis for conversation stickiness, deployment health state, runtime limit policies, concurrency, and quota reservations
- URM JWT for management APIs
- Local AI API keys for runtime OpenAI-compatible calls
- URM `Freeze -> Confirm -> Cancel` settlement

Deferred:

- Video, audio, rerank APIs
- Full native Claude/Gemini protocol adapters
- Prompt templates and response caching
- Cost-optimized routing
- Public self-service signup

## 3. Roles

| Role | Responsibilities |
| --- | --- |
| Platform admin | Configure providers, endpoints, upstream deployments, public models, model routes, tenant model grants, and global usage logs. |
| Tenant | View granted models, set tenant sale prices, create tenant-owned API keys, and view tenant usage. |
| End user | View authorized models and balance, create user-owned API keys, configure key quotas and allowed models, and view usage. |

**Note:** User model grants have been removed. Authorization is now only at the tenant level.

## 4. Authentication

Management APIs use URM-issued JWTs.

Runtime APIs use AI Gateway API keys:

```http
Authorization: Bearer sk-ai-...
```

API keys are local to `uni-ai-api`. Only hashes are stored. Plaintext keys are shown once at creation.

## 5. API Key Types

| Type | Created by | Bound identity | URM settlement | Local API key quota |
| --- | --- | --- | --- | --- |
| User key | End user | `tenant_id + user_id` | `tenantAmount = tenant sale price`, `userAmount = tenant sale price` | Charged at tenant sale price |
| Tenant key | Tenant | `tenant_id` | `tenantAmount = tenant sale price`, `userAmount = 0` | Charged at tenant sale price |

All billing values are integer credits. URM recharge flows convert money into credits before this service is called, so Uni AI API never converts cents to yuan or yuan to credits in runtime billing. Frontend and backend admin time fields are exchanged as Unix millisecond timestamps; the frontend owns display formatting.

Tenant-owned keys support anonymous resale scenarios. Optional request fields such as `user` or `X-End-User` may be logged for tenant analytics but do not participate in URM user billing.

## 6. Model Authorization

Models are separated into a public canonical model and upstream deployments connected via model routes:

```text
public model code: gpt-5.4
model route A → upstream deployment A → upstream model: GPT 5.4
model route B → upstream deployment B → upstream model: gpt-5.4
model route C → upstream deployment C → upstream model: aaa5.4
```

Consumers only see and request `model_code`. Provider adapters send `upstream_model` to the selected endpoint.

For all API keys (user-owned and tenant-owned):

```text
tenant model grants
∩ API key allowed models
= callable models
```

User model grants have been removed. Authorization is only at the tenant level.

## 7. Data Model

The new data model separates upstream deployments from public models:

```text
Provider → Endpoint → Upstream Deployment
                        ↓
Public Model → Model Route → Upstream Deployment
```

Key tables:

- **ai_providers**: Provider grouping (DeepSeek, OpenAI Compatible, etc.)
- **ai_provider_endpoints**: Connection configuration (base URL, API key, weight)
- **ai_upstream_deployments**: Upstream model configuration (upstream_model, protocol, parameters)
- **ai_models**: Public model catalog
- **ai_model_routes**: Routes connecting public models to upstream deployments
- **ai_model_prices**: Tenant sale prices (binds to model)
- **ai_upstream_deployment_cost_prices**: Provider cost prices (binds to upstream deployment)

This separation allows:
- Reusing an upstream deployment across multiple public models
- Independent management of upstream configuration vs. public model catalog
- Route-level priority and weight tuning per model

## 8. PostgreSQL Schema

Tables are unqualified. The target schema is controlled by PostgreSQL `search_path`, for example `search_path=public` or a tenant/platform-managed schema created before migration.

### ai_api_keys

```sql
CREATE TABLE ai_api_keys (
  id UUID PRIMARY KEY,
  owner_type TEXT NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id TEXT NOT NULL,
  user_id TEXT,
  key_hash TEXT NOT NULL UNIQUE,
  key_prefix TEXT NOT NULL,
  name TEXT NOT NULL,
  quota_limit BIGINT,
  quota_used BIGINT NOT NULL DEFAULT 0,
  quota_reserved BIGINT NOT NULL DEFAULT 0,
  allowed_models JSONB NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### ai_providers

```sql
CREATE TABLE ai_providers (
  id UUID PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Provider is a grouping entity for endpoints and deployments. The actual request behavior is determined by endpoint configuration (base_url, api_key) and deployment settings (upstream_protocol, upstream_model).

### ai_provider_endpoints

```sql
CREATE TABLE ai_provider_endpoints (
  id UUID PRIMARY KEY,
  provider_id UUID NOT NULL REFERENCES ai_providers(id),
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key_ciphertext TEXT NOT NULL,
  extra_headers JSONB NOT NULL DEFAULT '{}',
  weight INTEGER NOT NULL DEFAULT 100,
  timeout_ms INTEGER NOT NULL DEFAULT 30000,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (provider_id, name)
);
```

`base_url` should be the protocol base. For example, OpenAI can use `https://api.openai.com/v1`, while DeepSeek OpenAI-compatible chat can use `https://api.deepseek.com`.

### ai_upstream_deployments

```sql
CREATE TABLE ai_upstream_deployments (
  id UUID PRIMARY KEY,
  endpoint_id UUID NOT NULL REFERENCES ai_provider_endpoints(id),
  name TEXT NOT NULL,
  upstream_model TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  upstream_protocol TEXT NOT NULL DEFAULT 'openai_chat_completions',
  request_path TEXT,
  upstream_parameters JSONB NOT NULL DEFAULT '{}',
  tags JSONB NOT NULL DEFAULT '{}',
  health_status TEXT NOT NULL DEFAULT 'unknown',
  last_health_check_at TIMESTAMPTZ,
  last_health_error TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (endpoint_id, upstream_model, upstream_protocol)
);
```

`upstream_model` is the provider-specific model identifier sent to the provider.

`upstream_protocol` values:

- `openai_chat_completions`: OpenAI Chat Completions protocol.
- `openai_responses`: OpenAI Responses protocol.
- `openai_embeddings`: OpenAI-compatible Embeddings protocol.
- `openai_images_generations`: OpenAI-compatible Images Generations protocol.
- `anthropic_messages`: Anthropic Messages protocol (reserved).

`upstream_parameters` is for provider-specific defaults, for example DeepSeek reasoning parameters.

Default paths by protocol:

| Protocol | Default path |
| --- | --- |
| `openai_chat_completions` | `/chat/completions` |
| `openai_responses` | `/responses` |
| `openai_embeddings` | `/embeddings` |
| `openai_images_generations` | `/images/generations` |
| `anthropic_messages` | `/messages` |

### ai_models

```sql
CREATE TABLE ai_models (
  id UUID PRIMARY KEY,
  model_code TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  context_window INTEGER,
  default_max_output_tokens INTEGER NOT NULL DEFAULT 2048,
  max_output_tokens INTEGER,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### ai_model_routes

```sql
CREATE TABLE ai_model_routes (
  id UUID PRIMARY KEY,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  upstream_deployment_id UUID NOT NULL REFERENCES ai_upstream_deployments(id),
  priority INTEGER NOT NULL DEFAULT 100,
  weight INTEGER NOT NULL DEFAULT 100,
  supports_stream BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (model_id, upstream_deployment_id)
);
```

Routes connect public models to upstream deployments. Each route has its own priority and weight, allowing per-model tuning.

### ai_model_prices

```sql
CREATE TABLE ai_model_prices (
  id UUID PRIMARY KEY,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  input_price_per_1m BIGINT NOT NULL DEFAULT 0,
  output_price_per_1m BIGINT NOT NULL DEFAULT 0,
  image_prices JSONB NOT NULL DEFAULT '[]',
  video_prices JSONB NOT NULL DEFAULT '[]',
  audio_tts_price_per_1m_chars BIGINT NOT NULL DEFAULT 0,
  audio_stt_price_per_minute BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Sale prices bind to models. Database price columns and JSON `price` values are micro-credits; HTTP DTO fields expose decimal credits with explicit `*_credits` names.

### ai_upstream_deployment_cost_prices

```sql
CREATE TABLE ai_upstream_deployment_cost_prices (
  id UUID PRIMARY KEY,
  upstream_deployment_id UUID NOT NULL REFERENCES ai_upstream_deployments(id),
  capability_type TEXT NOT NULL DEFAULT 'chat',
  currency TEXT NOT NULL DEFAULT 'CNY_CREDITS',
  input_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  output_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  request_cost BIGINT NOT NULL DEFAULT 0,
  image_cost BIGINT NOT NULL DEFAULT 0,
  image_size_prices JSONB NOT NULL DEFAULT '{}',
  video_cost_per_second BIGINT NOT NULL DEFAULT 0,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Cost prices bind to upstream deployments. `image_size_prices` is a JSONB map of size to cost, for example `{"256x256": 1, "512x512": 2, "1024x1024": 4}`.

### ai_tenant_model_grants

```sql
CREATE TABLE ai_tenant_model_grants (
  id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id)
);
```

### ai_usage_logs

```sql
CREATE TABLE ai_usage_logs (
  id UUID PRIMARY KEY,
  request_id TEXT NOT NULL UNIQUE,
  trace_id TEXT,
  api_key_id UUID REFERENCES ai_api_keys(id),
  key_owner_type TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT,
  external_user_id TEXT,
  model_id UUID,
  model_code TEXT NOT NULL,
  model_route_id UUID,
  upstream_deployment_id UUID,
  endpoint_id UUID,
  provider_code TEXT,
  upstream_model TEXT,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  conversation_id TEXT,
  stream BOOLEAN NOT NULL DEFAULT false,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  image_count INTEGER NOT NULL DEFAULT 0,
  image_size TEXT,
  billable_unit_type TEXT NOT NULL DEFAULT 'token',
  billable_units BIGINT NOT NULL DEFAULT 0,
  provider_cost BIGINT NOT NULL DEFAULT 0,
  tenant_cost BIGINT NOT NULL DEFAULT 0,
  user_cost BIGINT NOT NULL DEFAULT 0,
  api_key_quota_cost BIGINT NOT NULL DEFAULT 0,
  urm_transaction_id TEXT,
  billing_status TEXT NOT NULL,
  request_status TEXT NOT NULL,
  http_status INTEGER,
  upstream_status INTEGER,
  latency_ms INTEGER,
  first_token_latency_ms INTEGER,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Usage logs now reference `model_id`, `model_route_id`, and `upstream_deployment_id` for complete traceability.

## 9. Redis Keys

```text
uni_ai_api:conv:{tenant_id}:{identity}:{model_code}:{conversation_id}
uni_ai_api:rate:{scope_type}:{scope_id}:{capability_type}:...
uni_ai_api:quota:key:{api_key_id}:reserved
uni_ai_api:deployment:{deployment_id}:cooldown
```

Conversation stickiness binds to `upstream_deployment_id`.

## 10. Routing

Routing order:

1. Resolve public model.
2. Resolve callable model grants (tenant grants ∩ API key allowed models).
3. Load active model routes for the model.
4. If conversation ID exists, load sticky deployment from Redis.
5. If sticky deployment is healthy, use it.
6. Otherwise choose among active routes with weighted random.
7. Weight calculation: `route.weight * endpoint.weight`.
8. Priority filtering: use lowest priority value among remaining routes.
9. On pre-first-token failure, retry another healthy route.
10. After streaming starts, do not switch providers.
11. On success, write or refresh sticky binding.

## 11. Billing

Pre-estimate:

```text
estimated_input_tokens = tokenizer(messages)
estimated_output_tokens = request.max_tokens OR model.default_max_output_tokens
tenant_estimated_micro = input_tokens * input_price_per_1m_micro + output_tokens * output_price_per_1m_micro
provider_estimated = input_tokens * input_cost_per_1m + output_tokens * output_cost_per_1m
```

Token prices are integer micro-credits per 1M tokens internally. The API boundary converts them to decimal credits.

Images use a different billable unit:

```text
billable_unit_type = image
billable_units = image_count
provider_cost = request_cost + image_size_prices[image_size] OR image_cost
tenant_cost_micro = image_count * image_prices[resolution].price_micro
user_cost_micro = image_count * image_prices[resolution].price_micro for user-owned keys
api_key_quota_cost_micro = image_count * image_prices[resolution].price_micro
```

For all API keys:

```text
URM Freeze:
tenantAmount = tenant_estimated
userAmount = tenant_estimated (user keys) OR 0 (tenant keys)
customerId = user_id (user keys) OR empty (tenant keys)
```

On success:

```text
actual token costs are calculated from prompt_tokens and completion_tokens.
actual image costs are calculated from image_count and image_size.
URM Confirm uses actual costs.
Local API key quota confirms api_key_quota_cost.
Provider cost is recorded from the selected upstream deployment cost price row.
```

## 12. OpenAI-Compatible Error Mapping

Insufficient balance or quota:

```json
{
  "error": {
    "message": "Insufficient quota.",
    "type": "insufficient_quota",
    "code": "insufficient_quota"
  }
}
```

Use HTTP `402`.

## 13. Frontend Plan

The initial frontend is copied from the URM foundation:

- `ai-admin`: platform operations
- `ai-tenant`: tenant operations
- `ai-customer`: end-user operations

AI Gateway modules should live under:

```text
src/views/AIGateway/
src/api/aiGateway.js
```

Platform pages:

- Providers (includes upstream deployment management)
- Models (includes model route management)
- Tenant model grants
- Global usage logs

Tenant pages:

- Available models
- Tenant sale prices
- Tenant-owned API keys
- Tenant usage logs

Customer pages:

- My models
- My API keys
- API key quota and allowed models
- My usage logs
- Balance shortcut
