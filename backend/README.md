# Backend

Go service for Uni AI API Gateway.

## Runtime APIs

- `GET /health`
- `GET /ready`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/embeddings`
- `POST /v1/images/generations`

Implemented backend responsibilities:

- Runtime API key authentication with `sk-ai-` bearer tokens
- Tenant/user model grant and key allowed-model checks
- OpenAI-compatible provider routing through model deployments
- Chat and responses non-streaming and SSE streaming relay
- Embeddings and image generation relay
- PostgreSQL configuration and usage ledgers
- Redis stickiness, rate limiting, endpoint cooldowns, and quota reservations
- URM HMAC `Freeze -> Confirm -> Cancel` settlement integration
- Admin APIs for providers, endpoints, models, deployments, prices, grants, API keys, limits, audit logs, and usage logs

## Local Commands

From `backend/`:

```bash
go run ./cmd/fake-upstream
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/server
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/migrate up
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/seed
```

The seed command reads `../db/local_seed.sql` by default.

Local setup details live in `backend/seeds/README.md`.
Smoke verification lives in `docs/LOCAL_SMOKE.md`.
Admin API examples live in `docs/ADMIN_API.md`.
