package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
type apiTenantIDKey struct{}
type apiUserIDKey struct{}
type apiUserTypeKey struct{}

func (s *Server) apiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 如果配置了静态 AdminToken，允许直接访问
		if s.validLocalAdminToken(r) {
			ctx := r.Context()
			setRequestIdentity(ctx, "", "", string(apiRolePlatform))
			ctx = withAdminContext(ctx, adminContext{
				Actor:    "local_admin",
				Role:     adminRolePlatform,
				UserType: 1,
			})
			ctx = context.WithValue(ctx, apiContextKey{}, apiContext{
				UserType: 1,
				TenantID: "",
				UserID:   "",
				Role:     apiRolePlatform,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 本地 JWKS 验证 JWT Token
		if s.jwksValidator == nil {
			writeAPIError(w, http.StatusUnauthorized, "authentication not configured")
			return
		}

		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeAPIError(w, http.StatusUnauthorized, "missing token")
			return
		}

		claims, err := s.jwksValidator.ValidateToken(r.Context(), token)
		if err != nil {
			s.logger.Warn("api auth token validation failed",
				"error", err,
				"request_id", requestIDFromContext(r.Context()),
			)
			writeAPIError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		role := roleForUserType(claims.UserType)

		// app_key 校验：非平台管理员（userType 1/2）的 token 必须携带与本服务匹配的 app_key
		if s.urmAppKey != "" && claims.UserType != 1 && claims.UserType != 2 {
			if claims.AppKey != s.urmAppKey {
				writeAPIError(w, http.StatusForbidden, "token not authorized for this service")
				return
			}
		}

		// 构建上下文
		ctx := r.Context()
		ctx = context.WithValue(ctx, apiContextKey{}, apiContext{
			UserType: claims.UserType,
			TenantID: claims.TenantID,
			UserID:   claims.UserID,
			Role:     role,
		})
		ctx = context.WithValue(ctx, apiTenantIDKey{}, claims.TenantID)
		ctx = context.WithValue(ctx, apiUserIDKey{}, claims.UserID)
		ctx = context.WithValue(ctx, apiUserTypeKey{}, claims.UserType)
		setRequestIdentity(ctx, claims.TenantID, claims.UserID, string(role))
		ctx = withAdminContext(ctx, adminContext{
			Actor:    claims.UserID,
			Role:     adminRole(role),
			TenantID: claims.TenantID,
			UserID:   claims.UserID,
			UserType: claims.UserType,
		})

		// 权限检查
		if !apiRequestAllowed(role, r.Method, r.URL.Path) {
			writeAPIError(w, http.StatusForbidden, "forbidden")
			return
		}

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

func apiRequestAllowed(role apiRole, method string, path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "api" || parts[1] != "v1" {
		return false
	}

	resourcePath := parts[2:] // 去掉 api/v1 前缀

	// 平台角色可以访问所有资源
	if role == apiRolePlatform {
		return true
	}

	// 租户和用户的权限检查
	return tenantOrUserAPIRequestAllowed(role, method, resourcePath)
}

func tenantOrUserAPIRequestAllowed(role apiRole, method string, resourcePath []string) bool {
	if len(resourcePath) == 0 {
		return false
	}

	resource := resourcePath[0]

	switch role {
	case apiRoleTenant:
		// 租户可访问的资源
		switch resource {
		case "tenant-api-keys", "tenant-model-grants", "user-prices":
			return true // 这些资源后端会自动过滤为该租户的数据
		case "dashboard", "usage-logs", "usage-summary", "tenant-usage-logs":
			return method == http.MethodGet
		case "tenants":
			// 租户可以访问 /api/v1/tenants/me/* 路径
			if len(resourcePath) >= 2 && resourcePath[1] == "me" {
				return true
			}
			return false
		case "tenant-users":
			// 租户管理自己的终端用户
			return true
		case "models":
			// 租户可以查看模型列表和价格参考
			return method == http.MethodGet
		case "upstream-deployments", "providers", "limit-policies", "audit-logs":
			return false // 仅平台可访问
		}

	case apiRoleUser:
		// 用户可访问的资源
		switch resource {
		case "user-api-keys", "user-model-grants", "user-usage-logs", "user-usage-summary":
			return true // 这些资源后端会自动过滤为该用户的数据
		case "dashboard":
			return method == http.MethodGet
		case "models":
			// 用户可以查看模型列表（用于选择模型）
			return method == http.MethodGet
		case "tenants", "tenant-api-keys", "tenant-model-grants", "user-prices", "tenant-users":
			return false // 仅租户和平台可访问
		case "upstream-deployments", "providers", "limit-policies", "audit-logs":
			return false // 仅平台可访问
		}
	}

	return false
}

// 从 context 提取信息
func apiContextFromContext(ctx context.Context) (apiContext, bool) {
	ac, ok := ctx.Value(apiContextKey{}).(apiContext)
	return ac, ok
}

func tenantIDFromAPIContext(ctx context.Context) string {
	ac, ok := apiContextFromContext(ctx)
	if !ok {
		return ""
	}
	return ac.TenantID
}

func userIDFromAPIContext(ctx context.Context) string {
	ac, ok := apiContextFromContext(ctx)
	if !ok {
		return ""
	}
	return ac.UserID
}

func userTypeFromAPIContext(ctx context.Context) int {
	ac, ok := apiContextFromContext(ctx)
	if !ok {
		return 0
	}
	return ac.UserType
}

func roleFromAPIContext(ctx context.Context) apiRole {
	ac, ok := apiContextFromContext(ctx)
	if !ok {
		return ""
	}
	return ac.Role
}

// 处理 /me 伪路径：将 "me" 替换为实际的 tenantID 或 userID
func resolveMePath(path string, ctx context.Context) string {
	ac, ok := apiContextFromContext(ctx)
	if !ok {
		return path
	}

	// 替换 /tenants/me/ 为 /tenants/{actualTenantID}/
	path = strings.Replace(path, "/tenants/me/", "/tenants/"+ac.TenantID+"/", 1)
	// 替换 /users/me/ 为 /users/{actualUserID}/
	path = strings.Replace(path, "/users/me/", "/users/"+ac.UserID+"/", 1)

	return path
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + message + `"}`))
}

func writeAPIJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.Encode(data)
}
