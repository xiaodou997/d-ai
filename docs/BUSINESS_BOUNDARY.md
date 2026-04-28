# Business Boundary

Uni AI API is the AI Gateway business system. It owns provider routing, public model mapping, runtime API keys, model authorization, usage logs, billing calculation, and AI Gateway audit records.

URM remains the identity, account, recharge, grant, transaction, platform-admin, JWT-key, and system-application authority. Uni AI API may provide business convenience entries for common URM operations, but URM remains the source of truth.

## Uni AI API Owns

- Provider, endpoint, model, deployment, and price configuration.
- Tenant and user model authorization for AI usage.
- Tenant-owned and user-owned runtime API keys.
- Chat, Responses, Embeddings, and Images runtime paths.
- Usage logs, billable unit statistics, provider cost, platform cost, user cost, and API key quota cost.
- AI Gateway dashboard and AI Gateway audit.
- Business convenience management for tenants and users used by the AI Gateway console:
  - Tenant create, edit, delete, enable, and disable operations.
  - Tenant organization user create, enable, disable, and reset-password operations.
  - End-user list and status display.
  - Platform-admin tenant recharge shortcut from tenant management only.
  - Tenant recharge record query for all tenants.

## URM Owns

- Tenant and user account balances.
- Recharge, refund, grant, and transaction records as authoritative account data.
- Platform administrators and JWT keys.
- System applications and application authorization.
- Global system audit outside the AI Gateway business domain.

## Admin Console Rules

- The Uni AI API admin console should expose URM operations only when they are direct AI Gateway business conveniences.
- Tenant recharge is not a standalone menu. It is available only from tenant management operation entries. Refund, user recharge, account overview, full transaction history, grant logs, platform administrator management, JWT key management, system application management, and global audit remain in the URM console.
- Recharge records stay as a business operation menu because platform administrators need to review all tenant recharge records in this console.
- Dashboard statistics in this console must come from AI Gateway data, primarily `ai_usage_logs`.
- Backend authorization remains the security boundary; frontend menu filtering is only an experience layer.
