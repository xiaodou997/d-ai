# Backend

Go service for Uni AI API Gateway.

## Runtime APIs

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

Implemented backend responsibilities:

- Runtime API key authentication with `sk-ai-` bearer tokens
- Tenant/user model grant and key allowed-model checks
- **Strict 1:1 protocol passthrough routing** — client protocol must match deployment `upstream_protocol`; no cross-protocol translation
- Chat, responses, Anthropic messages, Gemini generate, embeddings, and image generation — non-streaming and SSE streaming relay
- PostgreSQL configuration and usage ledgers
- Redis stickiness, rate limiting, endpoint cooldowns, and quota reservations
- URM HMAC `Freeze -> Confirm -> Cancel` settlement integration
- Admin APIs for providers, endpoints, models, deployments, prices, grants, API keys, limits, audit logs, and usage logs

## Local Commands

From `backend/`:

```bash
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/migrate up
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/seed
go run ./cmd/fake-upstream
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/server
```

The seed command reads `../db/local_seed.sql` by default.
`migrate down` rolls back only the latest applied migration.

Local setup details live in `backend/seeds/README.md`.
Smoke verification lives in `docs/LOCAL_SMOKE.md`.
Admin API examples live in `docs/ADMIN_API.md`.
