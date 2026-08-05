package domain

// 模型授权(ai_tenant_model_grants / ai_user_model_grants)已在 v4 重构中删除，
// 由租户直属分组(ai_groups.tenant_id) + 用户分组限制(ai_user_groups)替代。
// 相关领域类型见 group.go（TenantGroup / UserGroup / Group / VisibleGroup）。
