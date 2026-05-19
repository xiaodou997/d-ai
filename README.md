# uni-ai-api

Uni AI API Gateway provides OpenAI-compatible runtime APIs for internal AI services, with provider routing, local API keys, usage logging, and URM settlement.

## Current Runtime Surface

- `GET /health`
- `GET /ready`
- `GET /v1/models`
- `POST /v1/chat/completions` (openai_chat)
- `POST /v1/responses` (openai_responses)
- `POST /v1/messages` (anthropic_messages)
- `POST /v1/embeddings` (openai_embeddings)
- `POST /v1/images/generations` (openai_images)
- `POST /v1beta/models/{model}:{action}` (gemini_generate / gemini_embeddings)
- `POST /v1/messages/count_tokens` (Anthropic token estimation)

The gateway uses **strict 1:1 protocol passthrough**: the client protocol (derived from the request path) must exactly match the deployment's `upstream_protocol`. No cross-protocol format translation is performed — request and response bytes are forwarded verbatim. To serve the same model over multiple protocols, create separate deployments (one per `upstream_protocol`) under the same endpoint.

## Local E2E

Use the fake upstream first; real provider keys are not needed for the default local smoke path.

```bash
cp backend/config.local.example.yaml backend/config.local.yaml
```

Initialize schema and local data first:

```bash
cd backend
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/migrate up
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/seed
```

Then start:

```bash
cd backend
go run ./cmd/fake-upstream
```

In another shell:

```bash
cd backend
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/server
```

Runtime key:

```text
sk-ai-local-dev
```

Full smoke commands are in `docs/LOCAL_SMOKE.md`.

## Project Layout

```text
backend/        Go service, API gateway, provider routing, billing integration
db/             Local seed SQL for development data
web/admin/      Platform admin console
web/tenant/     Tenant console
web/customer/   End-user console
docs/           Product, API, and smoke verification docs
deployments/    Deployment manifests and environment examples
```

Useful docs:

- `docs/LOCAL_SMOKE.md`
- `docs/ADMIN_API.md`
- `docs/BUSINESS_BOUNDARY.md`
- `docs/frontend-auth.md`
- `docs/backend-usage-architecture.md`
- `backend/seeds/README.md`

## Product Decisions

- Public model codes are canonical; provider model names are mapped per deployment.
- Provider model cost prices are recorded for audit and margin reports.
- Runtime billing uses platform and tenant model prices, always in integer credits.
- Token interfaces bill by token units; image interfaces bill by generated image count.
- API key quotas are local to `uni-ai-api`; URM remains the source of account balances and credit settlement.
- URM remains the source of truth for account, recharge, grant, transaction, platform-admin, JWT-key, and system-application data.
- The admin console keeps AI Gateway business convenience entries for tenant/user management, tenant recharge from tenant operations, and tenant recharge records.
- Tenant-owned API keys charge the tenant through URM.
- User-owned API keys charge both tenant and user through URM.
- Runtime API keys use the `sk-ai-` prefix.
