# Confirmed Decisions And Open Questions

These decisions are confirmed and should be followed during implementation.

## Confirmed

1. Backend Go module path: `uni-ai-api/backend`.
2. Runtime API key prefix: `sk-ai-`.
3. Provider credentials encryption: use an environment-provided master key in MVP, then move to KMS later if needed.
4. API base URL style: OpenAI-compatible `/v1`, so SDK users only change `baseURL` and `apiKey`.
5. PostgreSQL is the primary database. Redis is used for conversation stickiness, rate limits, health state, and quota reservations.
6. Provider configuration must support custom providers with selectable protocol type.
7. The current runtime supports OpenAI-compatible Chat Completions, Responses, Embeddings, and Images Generations; Anthropic remains reserved in schema only.

## Still Open

1. Token counting library.
   - Recommended: start with provider `usage` when available, fallback to local tokenizer or conservative estimation for providers that omit usage.

2. First production provider list.
   - Recommended providers: DeepSeek, SiliconFlow, OpenRouter, Azure OpenAI, DashScope OpenAI-compatible mode.

3. Anthropic adapter delivery phase.
   - Recommended: keep the schema and adapter interface ready in MVP, implement the native Anthropic protocol after OpenAI-compatible providers are stable.
