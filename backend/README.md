# Backend

Go service for Uni AI API Gateway.

Planned responsibilities:

- OpenAI-compatible runtime APIs
- Provider endpoint routing
- Stream and non-stream upstream calls
- API key authentication and local quota ledgers
- PostgreSQL persistence
- Redis stickiness, health state, rate limits, and quota reservations
- URM HMAC client integration for `Freeze -> Confirm -> Cancel`

The backend implementation should stay AI-domain-specific and avoid becoming a generic HTTP proxy.

Current MVP-0 status:

- `GET /health` and `GET /ready`
- Runtime API Key authentication with `sk-ai-` bearer tokens
- `GET /v1/models`, filtered by tenant/user grants and key allowed models
- `POST /v1/chat/completions` request entrypoint with model access checks, quota exhaustion precheck, and deployment lookup
- Non-streaming OpenAI Chat Completions provider forwarding
- Public model to upstream model mapping through deployments
- Local PostgreSQL seed script for DeepSeek and DashScope Coding/Token Plan endpoints
- Admin provider, endpoint, model, and deployment configuration APIs
- Admin tenant model grant and tenant-owned API key APIs
- Basic chat usage logging and admin usage log query API
- Model sale price and provider cost price configuration APIs
- Chat token cost calculation and local API key quota usage confirmation
- Redis RPM limiting and estimated quota reservation for chat calls
- Redis endpoint cooldown after upstream failures
- Priority and weighted deployment routing

Not implemented yet:

- Streaming response relay
- Redis stickiness and rate limiting
- URM freeze/confirm/cancel integration in the request lifecycle
- TPM and concurrency limiting
- Periodic active health checks

Local initialization notes live in `backend/seeds/README.md`.
Admin API examples live in `docs/ADMIN_API.md`.
