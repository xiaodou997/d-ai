package transport

import (
	"context"

	"xiaodou/dai/internal/auth"
	authports "xiaodou/dai/internal/auth/ports"
	billingports "xiaodou/dai/internal/billing/ports"
	billingsvc "xiaodou/dai/internal/billing/service"
	systemports "xiaodou/dai/internal/system/ports"
	tenantports "xiaodou/dai/internal/tenant/ports"
	userports "xiaodou/dai/internal/user/ports"
	"xiaodou/dai/libs/go/httpx"
)

// adminHandlers 承载 /api/v1 管理资源端点（JWT + 用户类型守卫）。沿用 v1 admin
// handler 的逻辑（部分内联 SQL + 已搬 repo），输出强类型 DTO、错误 problem+json。
type adminHandlers struct {
	tenantReader       tenantports.AdminTenantReader
	tenantStatusWriter tenantports.AdminTenantStatusWriter
	tenantWriter       tenantports.AdminTenantWriter
	accountRepo        userports.AdminAccountReader
	accountWriter      userports.AdminAccountWriter
	endUserRepo        userports.AdminEndUserReader
	endUserWriter      userports.AdminEndUserWriter
	systemRepo         systemports.AdminDashboardReader
	deduction          *billingsvc.DeductionService
	accountQueries     billingports.AccountQueryReader
	security           authports.AccountSecurityWriter
	activations        *auth.ActivationService
	rechargeSvc        *billingsvc.RechargeService
	authAuditReader    authports.AuthAuditLogReader
}

type activationCredentialOutput struct {
	Body struct {
		ActivationToken     string `json:"activationToken"`
		ActivationExpiresIn int64  `json:"activationExpiresIn"`
	}
}

func setActivationOutput(out *activationCredentialOutput, result userports.ActivationCredentialResult) {
	out.Body.ActivationToken = result.Token
	out.Body.ActivationExpiresIn = result.ExpiresIn
}

func newAdminTenantHandlers(d adminTenantModule) *adminHandlers {
	return &adminHandlers{
		tenantReader:       d.TenantReader,
		tenantStatusWriter: d.TenantStatusWriter,
		tenantWriter:       d.TenantWriter,
		security:           d.Security,
		activations:        d.Activations,
	}
}

func newAdminUsersHandlers(d adminUsersModule) *adminHandlers {
	return &adminHandlers{
		accountRepo:   d.AdminAccounts,
		accountWriter: d.AdminAccountWriter,
		security:      d.Security,
		activations:   d.Activations,
	}
}

func newAdminFinanceHandlers(d adminFinanceModule) *adminHandlers {
	return &adminHandlers{
		deduction:       d.Deduction,
		accountQueries:  d.AccountQueries,
		rechargeSvc:     d.Recharge,
		authAuditReader: d.AuthAuditLogs,
	}
}

func newAdminUsageBillingHandlers(d adminUsageBillingModule) *adminHandlers {
	return &adminHandlers{deduction: d.Deduction}
}

func newAdminDashboardHandlers(d adminDashboardModule) *adminHandlers {
	return &adminHandlers{systemRepo: d.Dashboard}
}

func newAdminEndUsersHandlers(d adminEndUsersModule) *adminHandlers {
	return &adminHandlers{
		endUserRepo:   d.AdminEndUsers,
		endUserWriter: d.AdminEndUserWriter,
		security:      d.Security,
		activations:   d.Activations,
	}
}

func actorFromClaims(c *auth.Claims) auth.Actor {
	return auth.ActorFromClaims(c)
}

// userIDOf 安全取出 Claims 的 UserID（nil 返回空串）。
func userIDOf(c *auth.Claims) string {
	if c == nil {
		return ""
	}
	return c.UserID
}

func (h *adminHandlers) syncUserSecurity(ctx context.Context, userID, status string) error {
	if h.security == nil {
		return nil
	}
	if err := h.security.SyncUserStatus(ctx, userID, status); err != nil {
		return httpx.ErrUnavailable.WithCause(err)
	}
	return nil
}

func (h *adminHandlers) invalidateUserSessions(ctx context.Context, userID string) error {
	if h.security == nil {
		return nil
	}
	if err := h.security.InvalidateUserSessions(ctx, userID); err != nil {
		return httpx.ErrUnavailable.WithCause(err)
	}
	return nil
}

func (h *adminHandlers) syncTenantSecurity(ctx context.Context, tenantID, status string, restoredUserIDs []string) error {
	if h.security == nil {
		return nil
	}
	if err := h.security.SyncTenantStatus(ctx, tenantID, status, restoredUserIDs); err != nil {
		return httpx.ErrUnavailable.WithCause(err)
	}
	return nil
}

// adminStatusFromInt 把前端整型状态映射为存储字符串。
func adminStatusFromInt(value int) string {
	switch value {
	case 2:
		return "disabled"
	case 3:
		return "suspended"
	default:
		return "active"
	}
}

// adminTenantStatusToInt 把存储字符串状态映射为前端整型。
func adminTenantStatusToInt(value string) int {
	switch value {
	case "disabled":
		return 2
	case "suspended":
		return 3
	default:
		return 1
	}
}

// tenantStatusText 返回租户状态中文展示名。
func tenantStatusText(status int) string {
	switch status {
	case 1:
		return "启用"
	case 2:
		return "停用"
	case 3:
		return "欠费封禁"
	default:
		return "未知"
	}
}
