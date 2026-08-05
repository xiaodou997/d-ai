package console

import (
	"context"
)

type apiRole string

const (
	apiRolePlatform apiRole = "platform"
	apiRoleTenant   apiRole = "tenant"
	apiRoleUser     apiRole = "user"
)

type apiContext struct {
	UserType int
	TenantID string
	UserID   string
	Role     apiRole
}

type apiContextKey struct{}

func roleForUserType(userType int) apiRole {
	switch userType {
	case 1, 2: // 超管、平台管理员
		return apiRolePlatform
	case 3: // 租户用户
		return apiRoleTenant
	case 4: // 终端用户
		return apiRoleUser
	default:
		return ""
	}
}

// 从 context 提取信息
func apiContextFromContext(ctx context.Context) (apiContext, bool) {
	ac, ok := ctx.Value(apiContextKey{}).(apiContext)
	return ac, ok
}

func roleFromAPIContext(ctx context.Context) apiRole {
	ac, ok := apiContextFromContext(ctx)
	if !ok {
		return ""
	}
	return ac.Role
}
