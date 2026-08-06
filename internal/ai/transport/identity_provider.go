package transport

import (
	"context"

	"xiaodou/dai/internal/ai/platform"
)

// IdentityProvider 是 AI transport 查询统一身份信息的窄接口。
// 它可暂时为 nil（enrichment 返回空结果，不影响核心功能）。
type IdentityProvider interface {
	BatchGetUsers(ctx context.Context, userIDs []string) (map[string]*platform.InternalUser, error)
	BatchGetTenants(ctx context.Context, tenantIDs []string) (map[string]*platform.InternalTenant, error)
}
