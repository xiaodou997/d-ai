package console

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"xiaodou/unihub/ai-service/internal/httpx"
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

func (s *Console) apiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 本地 JWKS 验证 JWT Token（URM 是唯一的 auth 来源）
		if s.jwksValidator == nil {
			writeErr(w, http.StatusUnauthorized, BizErrTokenInvalid, "authentication not configured")
			return
		}

		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeErr(w, http.StatusUnauthorized, BizErrTokenInvalid, "missing token")
			return
		}

		claims, err := s.jwksValidator.ValidateToken(r.Context(), token)
		if err != nil {
			s.logger.Warn("api auth token validation failed",
				zap.Error(err),
				zap.String("request_id", requestIDFromContext(r.Context())),
			)
			writeErr(w, http.StatusUnauthorized, BizErrTokenInvalid, "invalid token")
			return
		}

		role := roleForUserType(claims.UserType)

		// client_id 校验：非平台管理员（userType 1/2）的 token 必须携带与本服务匹配的 client_id
		if s.urmClientID != "" && claims.UserType != 1 && claims.UserType != 2 {
			if claims.ClientID != s.urmClientID {
				writeErr(w, http.StatusForbidden, BizErrForbidden, "token not authorized for this service")
				return
			}
		}

		// 实时封禁检查
		if s.banSubscriber != nil && claims.UserID != "" && s.banSubscriber.IsBanned(claims.UserID) {
			writeErr(w, http.StatusForbidden, BizErrAccountBanned, "account banned")
			return
		}

		// 构建上下文
		ctx := r.Context()
		ctx = context.WithValue(ctx, apiContextKey{}, apiContext{
			UserType: claims.UserType,
			TenantID: claims.TenantID,
			UserID:   claims.UserID,
			Role:     role,
		})
		httpx.SetIdentity(ctx, claims.TenantID, claims.UserID, string(role))
		ctx = withAdminContext(ctx, adminContext{
			Actor:    claims.UserID,
			Role:     adminRole(role),
			TenantID: claims.TenantID,
			UserID:   claims.UserID,
			UserType: claims.UserType,
		})

		// 粗粒度访问控制由 Routes() 的 requireRole 子路由完成；handler 内部仍按角色
		// 过滤数据范围（scopedUsageFilters 等）。
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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
