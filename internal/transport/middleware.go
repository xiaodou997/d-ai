package transport

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/weborigin"
)

type ctxKey int

const (
	userClaimsCtxKey ctxKey = iota
	requestClientIPCtxKey
)

func requestClientMetadata(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		ip := weborigin.ClientIPFromContext(ctx.Context())
		if ip == "" {
			ip = ctx.RemoteAddr()
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
		}
		if strings.TrimSpace(ip) == "" {
			ip = "unknown"
		}
		next(huma.WithValue(ctx, requestClientIPCtxKey, ip))
	}
}

func requestClientIP(ctx context.Context) string {
	if value, ok := ctx.Value(requestClientIPCtxKey).(string); ok && value != "" {
		return value
	}
	return "unknown"
}

// requireSameOrigin protects cookie-bearing state changes from cross-site
// requests. Requests without Origin/Referer are kept compatible with native
// API clients; browser requests that provide either header must match Host.
func requireSameOrigin(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		trustedOrigin := weborigin.FromContext(ctx.Context())
		valid := sameOriginMatches(ctx.Header("Origin"), ctx.Header("Referer"), ctx.Host(), ctx.TLS() != nil)
		if trustedOrigin != "" {
			valid = sameOriginMatchesOrigin(ctx.Header("Origin"), ctx.Header("Referer"), trustedOrigin)
		}
		if !valid {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "请求来源不受信任")
			return
		}
		next(ctx)
	}
}

func sameOriginMatchesOrigin(originHeader, refererHeader, expectedOrigin string) bool {
	origin := strings.TrimSpace(originHeader)
	if origin == "" {
		origin = strings.TrimSpace(refererHeader)
	}
	if origin == "" {
		return true
	}
	parsedExpected, err := url.Parse(expectedOrigin)
	if err != nil || parsedExpected.Scheme == "" || parsedExpected.Host == "" || parsedExpected.User != nil {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, parsedExpected.Scheme) && strings.EqualFold(parsed.Host, parsedExpected.Host)
}

func sameOriginMatches(originHeader, refererHeader, host string, tls bool) bool {
	origin := strings.TrimSpace(originHeader)
	if origin == "" {
		origin = strings.TrimSpace(refererHeader)
	}
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || !strings.EqualFold(parsed.Host, host) {
		return false
	}
	return !tls || parsed.Scheme == "https"
}

func requireRecentAuth(api huma.API, recent *auth.RecentAuthService) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := ctx.Context().Value(userClaimsCtxKey).(*auth.Claims)
		if !ok || claims == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "未认证")
			return
		}
		if recent == nil {
			_ = huma.WriteErr(api, ctx, http.StatusServiceUnavailable, "近期认证服务不可用")
			return
		}
		valid, err := recent.Check(ctx.Context(), claims.UserID)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusServiceUnavailable, "近期认证服务不可用")
			return
		}
		if !valid {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "该敏感操作需要近期重新认证")
			return
		}
		next(ctx)
	}
}

// userAuth 是 Huma 中间件：校验用户 JWT 并做黑名单/强制
// 登出检查，通过则把完整 Claims 注入上下文。
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
		if !isUserAccessClaims(claims) {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Token 类型无效")
			return
		}
		// 黑名单：单 token 撤销（jti）与用户级强制登出时间戳（封号/全端登出）
		if blacklist != nil {
			if blacklist.IsBlacklisted(ctx.Context(), claims.ID) {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Token 已撤销")
				return
			}
			if claims.IssuedAt != nil {
				if logoutTime := blacklist.GetUserLogoutTime(ctx.Context(), claims.UserID); logoutTime > 0 && claims.IssuedAt.Unix() < logoutTime {
					_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Token 已撤销")
					return
				}
			}
		}
		next(huma.WithValue(ctx, userClaimsCtxKey, claims))
	}
}

func isUserAccessClaims(claims *auth.Claims) bool {
	return claims != nil && claims.PrincipalType == "user" && claims.TokenUse == "access" && claims.SessionID != ""
}

// requireCapability enforces a normalized backend capability. Capability
// checks are the only route authorization primitive.
func requireCapability(api huma.API, capability auth.Capability) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := ctx.Context().Value(userClaimsCtxKey).(*auth.Claims)
		if !ok || claims == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "未认证")
			return
		}
		actor := auth.ActorFromClaims(claims)
		if actor.Has(capability) {
			next(ctx)
			return
		}
		_ = huma.WriteErr(api, ctx, http.StatusForbidden, "权限不足")
	}
}

func requireAnyCapability(api huma.API, capabilities ...auth.Capability) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := ctx.Context().Value(userClaimsCtxKey).(*auth.Claims)
		if !ok || claims == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "未认证")
			return
		}
		actor := auth.ActorFromClaims(claims)
		for _, capability := range capabilities {
			if actor.Has(capability) {
				next(ctx)
				return
			}
		}
		_ = huma.WriteErr(api, ctx, http.StatusForbidden, "权限不足")
	}
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
