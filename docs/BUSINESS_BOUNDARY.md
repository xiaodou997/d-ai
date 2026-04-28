# Business Boundary

Uni AI API is the AI Gateway business system. It owns provider routing, public model mapping, runtime API keys, model authorization, usage logs, billing calculation, and AI Gateway audit records.

URM remains the identity, account, recharge, grant, transaction, platform-admin, JWT-key, and system-application authority.

## Uni AI API Owns

- Provider, endpoint, model, deployment, and price configuration.
- Tenant and user model authorization for AI usage.
- Tenant-owned and user-owned runtime API keys.
- Chat, Responses, Embeddings, and Images runtime paths.
- Usage logs, billable unit statistics, provider cost, platform cost, user cost, and API key quota cost.
- AI Gateway dashboard and AI Gateway audit.

## URM Owns

- Tenant and user account balances.
- Recharge, refund, grant, and transaction records.
- Platform administrators and JWT keys.
- System applications and application authorization.
- Global system audit outside the AI Gateway business domain.

## Admin Console Rules

- The Uni AI API admin console should not duplicate URM account or system administration pages.
- URM-domain console pages should be removed from the business console instead of kept behind hidden routes.
- Dashboard statistics in this console must come from AI Gateway data, primarily `ai_usage_logs`.
- Backend authorization remains the security boundary; frontend menu filtering is only an experience layer.
