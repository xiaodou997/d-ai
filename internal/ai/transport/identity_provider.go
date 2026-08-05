package transport

import (
	"context"

	"xiaodou/dai/internal/ai/urm"
)

// IdentityProvider 替代原 urm.Client 的身份信息查询能力。
// 合并后可直接用 URM 的 UserService/TenantService 实现，
// 也可暂时为 nil（enrichment 返回空结果，不影响核心功能）。
type IdentityProvider interface {
	BatchGetUsers(ctx context.Context, userIDs []string) (map[string]*urm.InternalUser, error)
	BatchGetTenants(ctx context.Context, tenantIDs []string) (map[string]*urm.InternalTenant, error)
}
