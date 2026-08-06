package transport

import (
	"context"
	"errors"
)

var ErrEndUserNotFound = errors.New("end user not found")

type IdentityUser struct {
	UserID   string
	TenantID string
	Username string
	Email    *string
	Nickname *string
	Avatar   *string
}

type IdentityTenant struct {
	TenantID   string
	TenantName string
}

// IdentityProvider 是 AI transport 查询统一身份信息的窄接口。
// 它可暂时为 nil（enrichment 返回空结果，不影响核心功能）。
type IdentityProvider interface {
	BatchGetUsers(ctx context.Context, userIDs []string) (map[string]*IdentityUser, error)
	BatchGetTenants(ctx context.Context, tenantIDs []string) (map[string]*IdentityTenant, error)
}
