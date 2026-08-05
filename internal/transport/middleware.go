package transport

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/serviceidentity"
	"xiaodou/dai/internal/auth"
)

type ctxKey int

const (
	clientIDCtxKey ctxKey = iota
	serviceClaimsCtxKey
	userClaimsCtxKey
)

// serviceAuth 是 Huma 中间件：校验调用方携带的 URM 服务令牌（principal_type=service
// 且 token_use=access），通过则把 client_id 注入上下文，否则写出统一 problem+json 401。
// 供 /internal/v1/* 内部服务端点使用（urm 作为颁发方用本地 jwtService 自验，无需 JWKS）。
func serviceAuth(api huma.API, jwtSvc *auth.JWTService, resolver *serviceidentity.SourceResolver) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token, ok := bearerToken(ctx.Header("Authorization"))
		if !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "缺少或非法的 Authorization 头")
			return
		}
		if jwtSvc == nil || resolver == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "服务身份校验未配置")
			return
		}
		req := &http.Request{RemoteAddr: ctx.RemoteAddr(), Header: make(http.Header)}
		req.Header.Set("X-Forwarded-For", ctx.Header("X-Forwarded-For"))
		req.Header.Set("X-Real-IP", ctx.Header("X-Real-IP"))
		source, sourceErr := resolver.Resolve(req)
		claims, err := jwtSvc.ParseToken(token)
		if err != nil || sourceErr != nil || claims.PrincipalType != "service" || claims.TokenUse != "access" ||
			claims.ClientID == "" || claims.InstanceID == "" || !slices.Contains(claims.Audience, "unihub-services") ||
			!serviceidentity.Contains(claims.SourceCIDR, source) {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "无效的服务令牌")
			return
		}
		ctx = huma.WithValue(ctx, clientIDCtxKey, claims.ClientID)
		next(huma.WithValue(ctx, serviceClaimsCtxKey, claims))
	}
}

// userAuth 是 Huma 中间件：校验用户 JWT（urm 本地 jwtService 验签）并做黑名单/强制
// 登出检查，通过则把完整 Claims 注入上下文。供 /api/oauth2 受保护端点使用。
func userAuth(api huma.API, jwtSvc *auth.JWTService, blacklist *auth.BlacklistService) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token, ok := bearerToken(ctx.Header("Authorization"))
		if !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "缺少 Bearer Token")
			return
		}
		claims, err := jwtSvc.ParseToken(token)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Token 无效或已过期")
			return
		}
		// 黑名单：单 token 撤销（jti）与用户级强制登出时间戳（封号/全端登出）
		if blacklist != nil && claims.PrincipalType == "user" {
			if blacklist.IsBlacklisted(claims.ID) {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Token 已撤销")
				return
			}
			if claims.IssuedAt != nil {
				if logoutTime := blacklist.GetUserLogoutTime(claims.UserID); logoutTime > 0 && claims.IssuedAt.Unix() < logoutTime {
					_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Token 已撤销")
					return
				}
			}
		}
		next(huma.WithValue(ctx, userClaimsCtxKey, claims))
	}
}

// requireUserType 要求当前用户类型在 allowed 内，否则 403。须挂在 userAuth 之后。
// 用户类型：1 超管 / 2 平台管理员 / 3 租户用户 / 4 终端用户。
func requireUserType(api huma.API, allowed ...int) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := ctx.Context().Value(userClaimsCtxKey).(*auth.Claims)
		if !ok || claims == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "未认证")
			return
		}
		if slices.Contains(allowed, claims.UserType) {
			next(ctx)
			return
		}
		_ = huma.WriteErr(api, ctx, http.StatusForbidden, "权限不足")
	}
}

// clientIDFromCtx 取出 serviceAuth 注入的调用方 client_id（用于扣费审计）。
func clientIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(clientIDCtxKey).(string)
	return v
}

// serviceClaimsFromCtx 取出 serviceAuth 注入的完整服务令牌 Claims（含 instance_id）。
func serviceClaimsFromCtx(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(serviceClaimsCtxKey).(*auth.Claims)
	return c
}

// userClaimsFromCtx 取出 userAuth 注入的用户 Claims。
func userClaimsFromCtx(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(userClaimsCtxKey).(*auth.Claims)
	return c
}

// bearerToken 从 Authorization 头解析 Bearer 令牌。
func bearerToken(header string) (string, bool) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
