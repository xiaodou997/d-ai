# uni-ai-api

Uni AI API Gateway provides an OpenAI-compatible API surface for internal AI services.

## MVP Scope

- `POST /v1/chat/completions`
- `GET /v1/models`
- OpenAI-compatible upstream providers
- PostgreSQL for configuration and usage ledgers
- Redis for conversation stickiness, rate limits, health state, and quota reservations
- URM JWT for management APIs
- Local AI API keys for OpenAI-compatible runtime calls
- URM pre-authorization billing for tenant and user credit settlement

## Project Layout

```text
backend/        Go service, API gateway, provider routing, billing integration
web/admin/      Platform admin console, copied from URM admin foundation
web/tenant/     Tenant console, copied from URM tenant foundation
web/customer/   End-user console, copied from URM customer foundation
docs/           Product and technical design documents
deployments/    Deployment manifests and environment examples
```

Useful docs:

- `docs/MVP_TECH_DESIGN.md`
- `docs/ADMIN_API.md`
- `backend/seeds/README.md`

## Confirmed Product Decisions

- Platform admins configure providers, endpoints, public models, platform pricing, and tenant model grants.
- Public model codes are canonical. Provider-specific model names are mapped per deployment.
- Provider model cost prices are recorded for audit and margin reports, while runtime billing uses platform and tenant prices.
- Tenants view granted models, set tenant-to-user prices, grant models to users, and create tenant-owned API keys.
- End users create their own API keys, set key quotas, select allowed models, and view their balance and usage.
- User-owned API keys charge both tenant and user through URM.
- Tenant-owned API keys charge only the tenant through URM, while local API key quota is still calculated at the tenant sale price.
- API key quotas are local to `uni-ai-api`; URM remains the source of account balances and credit settlement.
- Runtime API keys use the `sk-ai-` prefix.
- Backend Go module path is `uni-ai-api/backend`.
- Provider API keys are encrypted with an environment-provided master key in MVP.
- Providers support a custom provider mode with selectable protocol type. MVP implements `openai_chat_completions` first and reserves `openai_responses` and `anthropic_messages` in the adapter design.
