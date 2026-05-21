# Web Runtime Chat Design

This document records the agreed design for adding the first web runtime
feature: `AI 对话`, a first-party text chat page available in both the tenant
and end-user consoles.

The project is not in production yet. Historical database compatibility is not
required. Prefer long-term clarity over incremental compatibility patches.

## Goals

- Add a formal web text chat entrypoint, not a reduced "trial" feature.
- Reuse the same runtime route selection, upstream execution, billing,
  rate-limit, usage logging, and audit pipeline used by API key calls.
- Keep API key integration and first-party web usage distinct in analytics.
- Avoid requiring users to create API keys before trying or using chat in the
  web UI.
- Make the model extensible for future web features such as image generation.

## Product Decisions

| Topic | Decision |
| --- | --- |
| Page name | `AI 对话` |
| Consoles | Tenant console and end-user console |
| Capability | Text chat only for this feature |
| Attachments | Not supported |
| Streaming | Required |
| Frontend conversation history | Current browser page/session only in v1 |
| Audit payload | Persist request messages and final assistant message, same as API key calls |
| Tenant web chat billing | Charge the current tenant only |
| User web chat billing | Charge both the current user and the user's tenant |
| Insufficient user balance | Reject even if tenant balance is sufficient |
| API key requirement | Not required for web chat |
| API key `allowed_models` | Not applied to web chat |
| Model authorization | Tenant grants for tenant console; user available models for user console, with current fallback semantics |
| Rate limits | Count web chat into tenant/user RPM and TPM policies; do not count API key scope policies |
| Usage filtering | Add a first-class request source filter |

## Public Entrypoints

### API Key Runtime

Existing programmatic runtime entrypoints remain OpenAI-compatible:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/embeddings`
- `POST /v1/images/generations`

These use API key authentication and are recorded as:

```text
auth_method = api_key
request_source = api_key
```

### Web Chat Runtime

Add a non-management runtime entrypoint:

```http
POST /console/v1/chat/completions
Authorization: Bearer <URM JWT>
Content-Type: application/json
Accept: text/event-stream
```

This endpoint is JWT-authenticated and streams OpenAI-compatible chat
completion chunks. It is recorded as:

```text
auth_method = jwt
request_source = web_chat
capability_type = chat
```

The route must not be mounted under the current management `/api/v1` group,
because that group has a bounded timeout which is inappropriate for SSE.

## Unified Runtime Identity

The current runtime pipeline is centered on API keys. Long term, runtime calls
should be centered on a single identity model:

```go
type RuntimeIdentity struct {
    AuthMethod    RuntimeAuthMethod // api_key | jwt
    RequestSource RequestSource     // api_key | web_chat
    OwnerType     OwnerType         // tenant | user
    TenantID      string
    UserID        string            // empty for tenant-owned calls
    APIKeyID      string            // empty for JWT web calls
    AllowedModels []string          // API key allowlist only
    QuotaLimit    *int64            // API key quota only
    QuotaUsed     int64
    QuotaReserved int64
}
```

Rules:

- API key calls populate `APIKeyID`, `AllowedModels`, and quota fields.
- Web chat calls leave `APIKeyID` empty and skip API key quota/allowlist.
- Authorization, billing, rate limiting, usage logging, and audit should read
  the identity instead of reaching directly into API key fields.
- API key quota reservation/confirmation only runs when `AuthMethod=api_key`
  and `APIKeyID` is present.

## Authorization

API key runtime:

```text
tenant model grants
intersect optional user availability rules
intersect API key allowed_models
= callable models
```

Web chat runtime:

```text
tenant console: tenant model grants
user console: user available models using existing fallback semantics
= callable models
```

Web chat must not inspect or require any API key.

## Billing

Billing calculation uses the same pricing resolver and `URM Freeze -> Confirm
-> Cancel` lifecycle for both sources.

Tenant-owned web chat:

```text
tenant_amount = tenant sale price
user_amount = 0
```

User-owned web chat:

```text
tenant_amount = tenant sale price
user_amount = tenant sale price
```

If user billing fails, the request is rejected. It must not proceed just because
the tenant has balance.

## Rate Limiting

Rate limiting must use the unified identity:

- API key calls may match tenant, user, API key, provider, endpoint, and model
  policies.
- Web chat calls may match tenant, user, provider, endpoint, and model policies.
- Web chat must not match API key scope policies.

RPM uses request count. TPM uses the existing conservative preflight token
estimate.

## Usage And Rollups

Break historical compatibility and update usage tables directly.

Recommended naming:

- `request_source`: `api_key` | `web_chat`
- `auth_method`: `api_key` | `jwt`
- `owner_type`: `tenant` | `user`
- `token_usage_source`: `upstream` | `estimated`

Avoid using `usage_source` for request source because it currently describes
token usage origin.

`ai_usage_logs` should allow `api_key_id` to be null for web calls and include
`request_source`, `auth_method`, `owner_type`, and `capability_type`.

`ai_usage_rollups_hourly` should include `request_source` in the aggregation
key so usage pages can accurately filter API key calls versus web chat.

## Audit

Audit behavior should match API key runtime behavior:

- Persist user-sent messages.
- Persist the final assistant message after a stream completes.
- Persist failed requests when enough context exists.
- Include `request_source` and `auth_method` either directly in audit payloads
  or through the linked usage record.

The frontend's v1 page history is not a business chat history store. Audit
storage is operational and compliance logging.

## Frontend

Tenant console:

- Add `/ai/chat`.
- Add `AI 对话` under the `AI Gateway` sidebar group.
- Show chat-capable tenant-granted models only.
- Billing context: current tenant.

End-user console:

- Add `/ai/chat`.
- Add `AI 对话` under the `AI Gateway` sidebar group.
- Show chat-capable user-available models only.
- Billing context: current user and current tenant.

Chat page:

- Generate a `conversation_id` for each new chat.
- Send `X-Conversation-Id` with every request in that chat.
- Default visible control: model selection.
- Advanced controls: `temperature`, `max_tokens`.
- Maintain messages in page state.
- Use streaming fetch, not Axios, for SSE.
- Show clear errors for insufficient balance, missing authorization, and rate
  limits.

Usage pages:

- Add request source filter: all, API key calls, web chat.
- Keep capability filters separate from request source so future web image
  generation can reuse the same source model.

## Implementation Order

1. Add `RuntimeIdentity`, `RequestSource`, and `AuthMethod` domain concepts.
2. Refactor the serving request from API-key-centered fields to identity-centered
   fields.
3. Adapt API key auth to populate `RuntimeIdentity`.
4. Add JWT console runtime auth and `/console/v1/chat/completions`.
5. Refactor grant checking, price resolving, rate limiting, quota handling,
   URM billing, usage logging, and audit payload construction to read identity.
6. Update `init.sql`, sqlc queries, generated DB code, and usage DTOs.
7. Add request source filters to admin, tenant, and user usage APIs.
8. Add tenant and user `AI 对话` pages with streaming chat.
9. Add usage UI source filters.
10. Run Go tests, frontend builds, and a local streaming smoke test.

## Non-Goals For V1

- Persist user-facing chat history as a retrievable conversation product.
- Upload files or images.
- Expose image/audio/video web runtime features.
- Require API key creation before web chat.
- Emulate or create hidden API keys for web chat.
