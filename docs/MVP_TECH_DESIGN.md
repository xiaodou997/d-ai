# Uni AI API Gateway MVP Technical Design

## 1. Goal

`uni-ai-api` is a dedicated AI API gateway. It exposes an OpenAI-compatible API to business users while hiding provider differences, endpoint routing, billing, quotas, and model authorization.

It is not a generic HTTP proxy. The domain model is AI-specific: provider, endpoint, model, deployment, price, API key, usage, and settlement.

## 2. MVP Scope

Supported in the first version:

- `POST /v1/chat/completions`
- `GET /v1/models`
- OpenAI Chat Completions-compatible upstream providers first
- Protocol design reserves OpenAI Responses API and Anthropic Messages API
- Canonical model mapping from public model code to provider upstream model name
- Provider model cost prices for audit and margin reports
- Streaming and non-streaming chat completions
- PostgreSQL for persistent configuration, quota ledgers, and usage logs
- Redis for conversation stickiness, endpoint health state, rate limits, and quota reservations
- URM JWT for management APIs
- Local AI API keys for runtime OpenAI-compatible calls
- URM `Freeze -> Confirm -> Cancel` settlement

Deferred:

- Image, video, audio, embedding, and rerank APIs
- Full native Claude/Gemini protocol adapters
- Prompt templates and response caching
- Cost-optimized routing
- Public self-service signup

## 3. Roles

| Role | Responsibilities |
| --- | --- |
| Platform admin | Configure providers, endpoints, public models, deployments, platform pricing, tenant model grants, and global usage logs. |
| Tenant | View granted models, set tenant sale prices, grant models to users, create tenant-owned API keys, and view tenant usage. |
| End user | View authorized models and balance, create user-owned API keys, configure key quotas and allowed models, and view usage. |

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
| User key | End user | `tenant_id + user_id` | `tenantAmount = platform price`, `userAmount = tenant sale price` | Charged at tenant sale price |
| Tenant key | Tenant | `tenant_id` | `tenantAmount = platform price`, `userAmount = 0` | Charged at tenant sale price |

Tenant-owned keys support anonymous resale scenarios. Optional request fields such as `user` or `X-End-User` may be logged for tenant analytics but do not participate in URM user billing.

## 6. Model Authorization

Models are separated into a public canonical model and provider deployments:

```text
public model code: gpt-5.4
deployment A upstream model: GPT 5.4
deployment B upstream model: gpt-5.4
deployment C upstream model: aaa5.4
```

Consumers only see and request `model_code`. Provider adapters send `upstream_model` to the selected endpoint.

For user-owned keys:

```text
platform tenant grants
∩ tenant user grants
∩ API key allowed models
= callable models
```

For tenant-owned keys:

```text
platform tenant grants
∩ API key allowed models
= callable models
```

## 7. Runtime APIs

### POST /v1/chat/completions

The request body follows the OpenAI Chat Completions shape.

Conversation stickiness is resolved in this order:

1. `X-Conversation-Id`
2. `metadata.conversation_id`
3. No sticky binding

The OpenAI `user` field remains an end-user identifier and should not be overloaded as the conversation ID.

Future runtime APIs should be capability-specific rather than overloading chat:

| Capability | External API path | Canonical model type |
| --- | --- | --- |
| Chat | `/v1/chat/completions` | `chat` |
| Image | `/v1/images/generations` | `image` |
| Video | `/v1/videos/generations` or vendor-compatible path decided later | `video` |
| Embedding | `/v1/embeddings` | `embedding` |

### GET /v1/models

Returns only models callable by the current API key.

## 8. PostgreSQL Schema Draft

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
  daily_quota BIGINT,
  daily_used BIGINT NOT NULL DEFAULT 0,
  monthly_quota BIGINT,
  monthly_used BIGINT NOT NULL DEFAULT 0,
  allowed_models JSONB NOT NULL DEFAULT '[]',
  rpm_limit INTEGER,
  tpm_limit INTEGER,
  concurrency_limit INTEGER,
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
  provider_type TEXT NOT NULL,
  protocol_type TEXT NOT NULL DEFAULT 'openai_compatible',
  is_custom BOOLEAN NOT NULL DEFAULT false,
  config JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`protocol_type` values:

- `openai_chat_completions`: OpenAI Chat Completions protocol.
- `openai_responses`: OpenAI Responses protocol. Reserved until we expose `/v1/responses` or need an upstream-only adapter for OpenAI-native models.
- `anthropic_messages`: Anthropic Messages protocol. Reserved in MVP schema; implementation can follow after the OpenAI Chat Completions path is stable.

`is_custom = true` allows tenants or platform admins to configure a custom provider with its own request base URL, model names, and protocol type.

### ai_provider_endpoints

```sql
CREATE TABLE ai_provider_endpoints (
  id UUID PRIMARY KEY,
  provider_id UUID NOT NULL REFERENCES ai_providers(id),
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  protocol_type TEXT NOT NULL DEFAULT 'openai_chat_completions',
  api_key_ciphertext TEXT NOT NULL,
  extra_headers JSONB NOT NULL DEFAULT '{}',
  custom_path TEXT,
  protocol_overrides JSONB NOT NULL DEFAULT '{}',
  weight INTEGER NOT NULL DEFAULT 100,
  rpm_limit INTEGER,
  tpm_limit INTEGER,
  concurrency_limit INTEGER,
  timeout_ms INTEGER NOT NULL DEFAULT 30000,
  status TEXT NOT NULL DEFAULT 'active',
  health_status TEXT NOT NULL DEFAULT 'unknown',
  last_health_check_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`custom_path` lets custom providers override the default protocol path.

Default paths by protocol:

| Protocol | Default path |
| --- | --- |
| `openai_chat_completions` | `/chat/completions` |
| `openai_responses` | `/responses` |
| `anthropic_messages` | `/messages` |

`base_url` should be the protocol base. For example, OpenAI can use `https://api.openai.com/v1`, while DeepSeek OpenAI-compatible chat can use `https://api.deepseek.com`.

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

### ai_model_deployments

```sql
CREATE TABLE ai_model_deployments (
  id UUID PRIMARY KEY,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  endpoint_id UUID NOT NULL REFERENCES ai_provider_endpoints(id),
  upstream_model TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  upstream_protocol TEXT NOT NULL DEFAULT 'openai_chat_completions',
  upstream_parameters JSONB NOT NULL DEFAULT '{}',
  priority INTEGER NOT NULL DEFAULT 100,
  weight INTEGER NOT NULL DEFAULT 100,
  supports_stream BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`upstream_model` is the provider-specific model identifier. This is the model name sent to the provider. It may differ from our public `model_code`.

`upstream_protocol` is the adapter actually used for this deployment. It normally matches the endpoint protocol, but keeping it on the deployment lets a provider expose multiple compatible protocols under the same provider account without creating ambiguous routing logic.

`upstream_parameters` is reserved for provider-specific defaults, for example DeepSeek `thinking`, Anthropic `max_tokens`, OpenAI-compatible extra body fields, or vendor-specific switches.

### ai_provider_model_prices

```sql
CREATE TABLE ai_provider_model_prices (
  id UUID PRIMARY KEY,
  provider_id UUID NOT NULL REFERENCES ai_providers(id),
  endpoint_id UUID REFERENCES ai_provider_endpoints(id),
  upstream_model TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  currency TEXT NOT NULL DEFAULT 'CNY_CREDITS',
  input_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  output_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  request_cost BIGINT NOT NULL DEFAULT 0,
  image_cost BIGINT NOT NULL DEFAULT 0,
  video_cost_per_second BIGINT NOT NULL DEFAULT 0,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

This table records provider-side cost prices for audit and margin analysis. It is not used to charge end users. Runtime billing still uses `ai_model_prices`.

`endpoint_id` is nullable. A provider-level price applies to all endpoints under that provider; an endpoint-level price can override it when a channel has a custom contract or discount.

### ai_model_prices

```sql
CREATE TABLE ai_model_prices (
  id UUID PRIMARY KEY,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  platform_input_price_per_1m BIGINT NOT NULL DEFAULT 0,
  platform_output_price_per_1m BIGINT NOT NULL DEFAULT 0,
  tenant_input_price_per_1m BIGINT NOT NULL DEFAULT 0,
  tenant_output_price_per_1m BIGINT NOT NULL DEFAULT 0,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

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

### ai_user_model_grants

```sql
CREATE TABLE ai_user_model_grants (
  id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, model_id)
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
  model_code TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  deployment_id UUID,
  endpoint_id UUID,
  provider_code TEXT,
  upstream_model TEXT,
  conversation_id TEXT,
  stream BOOLEAN NOT NULL DEFAULT false,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  provider_cost BIGINT NOT NULL DEFAULT 0,
  platform_cost BIGINT NOT NULL DEFAULT 0,
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

## 9. Redis Keys

```text
uni_ai_api:conv:{tenant_id}:{identity}:{model_code}:{conversation_id}
uni_ai_api:rate:key:{api_key_id}:rpm
uni_ai_api:rate:key:{api_key_id}:tpm
uni_ai_api:quota:key:{api_key_id}:reserved:{request_id}
uni_ai_api:endpoint:{endpoint_id}:health
uni_ai_api:endpoint:{endpoint_id}:cooldown
```

## 10. Routing

Routing order:

1. Resolve public model.
2. Resolve callable model grants.
3. If conversation ID exists, load sticky deployment from Redis.
4. If sticky deployment is healthy, use it.
5. Otherwise choose among active deployments with weighted random.
6. On pre-first-token failure, retry another healthy deployment.
7. After streaming starts, do not switch providers.
8. On success, write or refresh sticky binding.

After routing selects a deployment, the provider adapter is selected by `upstream_protocol`. MVP implementation should ship `openai_chat_completions` first and keep the adapter interface ready for `openai_responses` and `anthropic_messages`.

OpenAI currently has two relevant HTTP API surfaces for our gateway:

- Chat Completions: message-list style API, broadly supported by third-party OpenAI-compatible providers.
- Responses API: newer unified API surface for multimodal, tools, state, and agentic workflows.

Recommendation:

1. MVP external API exposes `POST /v1/chat/completions` only.
2. Internal protocol enum already reserves `openai_responses`.
3. Add external `POST /v1/responses` after the chat path is stable, instead of forcing Responses semantics into Chat Completions.
4. Allow an upstream deployment to use `openai_responses` later, but only after we implement request/response translation and usage extraction explicitly.

Provider templates should be presets, not hard-coded business rules. DeepSeek should be seeded as:

| Field | Value |
| --- | --- |
| `provider.code` | `deepseek` |
| `provider.protocol_type` | `openai_chat_completions` for `/chat/completions`; `anthropic_messages` can use a separate endpoint when implemented |
| OpenAI-compatible `base_url` | `https://api.deepseek.com` |
| Anthropic-compatible `base_url` | `https://api.deepseek.com/anthropic` |
| Chat path | `/chat/completions` |
| Current upstream models | `deepseek-v4-flash`, `deepseek-v4-pro` |
| Deprecated upstream models | `deepseek-chat`, `deepseek-reasoner`, deprecated on 2026-07-24 per DeepSeek docs |

Model mapping examples:

```text
model_code = deepseek-v4-pro
deployment.upstream_model = deepseek-v4-pro

model_code = gpt-5.4
deployment.upstream_model = GPT 5.4
deployment.upstream_parameters = {"thinking":{"type":"enabled"},"reasoning_effort":"high"}
```

## 11. Billing

Pre-estimate:

```text
estimated_input_tokens = tokenizer(messages)
estimated_output_tokens = request.max_tokens OR model.default_max_output_tokens
provider_estimated = input * provider_input_cost + output * provider_output_cost
platform_estimated = input * platform_input_price + output * platform_output_price
user_estimated = input * tenant_input_price + output * tenant_output_price
```

`provider_estimated` is recorded for cost audit and margin reporting. It does not participate in URM settlement.

For user-owned keys:

```text
URM Freeze:
tenantAmount = platform_estimated
userAmount = user_estimated
customerId = user_id
```

For tenant-owned keys:

```text
URM Freeze:
tenantAmount = platform_estimated
userAmount = 0
customerId = empty
```

On success:

```text
actual costs are calculated from prompt_tokens and completion_tokens.
URM Confirm uses platform_actual and user_actual.
Local API key quota confirms api_key_quota_actual = user_actual.
Provider cost is recorded separately from the selected provider price row.
```

On failure before any upstream response:

```text
URM Cancel.
Local API key reservation is released.
```

On stream interruption after chunks were sent:

```text
Set request_status = PARTIAL.
Estimate or count emitted tokens.
Confirm the known actual cost.
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

- `web/admin`: platform operations
- `web/tenant`: tenant operations
- `web/customer`: end-user operations

AI Gateway modules should live under:

```text
src/views/AIGateway/
src/api/aiGateway.js
```

Platform pages:

- Providers
- Endpoints
- Models
- Deployments
- Platform pricing
- Tenant model grants
- Global usage logs
- Endpoint health

Tenant pages:

- Available models
- Tenant sale prices
- User model grants
- Tenant-owned API keys
- Tenant usage logs

Customer pages:

- My models
- My API keys
- API key quota and allowed models
- My usage logs
- Balance shortcut

## 14. Iteration Plan

### MVP-0

- Repository layout
- PostgreSQL migration draft
- URM HMAC client design
- API key hash and verification design
- Management API route map

### MVP-1

- `/v1/models`
- Non-streaming `/v1/chat/completions`
- OpenAI-compatible upstream client
- Custom OpenAI-compatible provider configuration
- URM Freeze/Confirm/Cancel
- Usage logging

### MVP-2

- Streaming chat completions
- Redis conversation stickiness
- Weighted routing
- Endpoint cooldown and health state
- API key quota reservation and confirmation

### MVP-3

- Platform admin pages
- Tenant pages
- Customer API key pages
- Usage and billing reports

### MVP-4

- Image generation
- Video generation
- Anthropic protocol adapter
- Native non-OpenAI provider adapters
- Gross margin and provider cost reports
- Cost-aware routing
