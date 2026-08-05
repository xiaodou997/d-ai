package authn

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"

	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/libs/go/logger"
	"xiaodou/dai/libs/go/serviceidentity"
)

const ServiceAudience = "unihub-services"

// Verifier 校验 URM 颁发的 RS256 JWT，验签所用公钥按 token 头部 kid 从 JWKSManager
// 取得。Issuer 为空时默认期望 "urm"。
type Verifier struct {
	jwks   *JWKSManager
	issuer string
}

// NewVerifier 构造校验器。
func NewVerifier(m *JWKSManager, issuer string) *Verifier {
	if issuer == "" {
		issuer = "urm"
	}
	return &Verifier{jwks: m, issuer: issuer}
}

// Verify 解析并校验 token 字符串，返回强类型 Claims。
func (v *Verifier) Verify(ctx context.Context, tokenStr string) (*Claims, error) {
	return v.verify(ctx, tokenStr, "")
}

func (v *Verifier) verify(ctx context.Context, tokenStr, audience string) (*Claims, error) {
	// 先无验证解析以取得 kid。
	unverified, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	kid, _ := unverified.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("token missing kid")
	}

	pub, err := v.jwks.GetPublicKey(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("resolve public key: %w", err)
	}

	claims := &Claims{}
	options := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer)}
	if audience != "" {
		options = append(options, jwt.WithAudience(audience))
	}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
		return pub, nil
	}, options...)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	return claims, nil
}

// VerifyService validates the complete service-token contract, including the
// fixed audience and the request source bound into the signed token.
func (v *Verifier) VerifyService(ctx context.Context, tokenStr string, source netip.Addr) (*Claims, error) {
	claims, err := v.verify(ctx, tokenStr, ServiceAudience)
	if err != nil {
		return nil, err
	}
	if !claims.IsService() || claims.ClientID == "" || claims.InstanceID == "" || claims.SourceCIDR == "" {
		return nil, fmt.Errorf("invalid service claims")
	}
	if !serviceidentity.Contains(claims.SourceCIDR, source) {
		return nil, fmt.Errorf("service token source mismatch")
	}
	return claims, nil
}

// Middleware 返回 chi 中间件：校验 Bearer JWT，成功则把 Claims 与身份字段注入
// 上下文，失败则写出统一 problem+json 401。
func (v *Verifier) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if authz == "" || !strings.HasPrefix(authz, "Bearer ") {
				writeUnauthorized(w, r, "缺少 Bearer Token")
				return
			}
			tokenStr := strings.TrimPrefix(authz, "Bearer ")

			claims, err := v.Verify(r.Context(), tokenStr)
			if err != nil {
				writeUnauthorized(w, r, "Token 无效或已过期")
				return
			}

			ctx := WithClaims(r.Context(), claims)
			ctx = logger.WithUserID(ctx, claims.UserID)
			ctx = logger.WithTenantID(ctx, claims.TenantID)
			ctx = logger.WithRole(ctx, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ServiceMiddleware resolves the real caller through trusted proxies and then
// validates a source-bound service JWT.
func (v *Verifier) ServiceMiddleware(resolver *serviceidentity.SourceResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr, ok := bearer(r.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(w, r, "缺少 Bearer Token")
				return
			}
			source, err := resolver.Resolve(r)
			if err != nil {
				writeUnauthorized(w, r, "无法确认请求来源")
				return
			}
			claims, err := v.VerifyService(r.Context(), tokenStr, source)
			if err != nil {
				writeUnauthorized(w, r, "服务令牌无效、已过期或来源不匹配")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

// RequireService 要求请求携带的是服务令牌（principal_type=service）；当
// expectedClientID 非空时还要求 client_id 匹配。须挂在 Verifier.Middleware 之后。
func RequireService(expectedClientID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeUnauthorized(w, r, "未认证")
				return
			}
			if !claims.IsService() || (expectedClientID != "" && claims.ClientID != expectedClientID) {
				writeForbidden(w, r, "服务令牌无权访问该资源")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireServiceIDs applies optional target-local authorization. With no IDs
// any valid registered service is accepted.
func RequireServiceIDs(allowed ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok || !claims.IsService() {
				writeUnauthorized(w, r, "未认证的服务")
				return
			}
			if len(allowed) > 0 && !slices.Contains(allowed, claims.ClientID) {
				writeForbidden(w, r, "服务无权访问该资源")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearer(header string) (string, bool) {
	parts := strings.SplitN(header, " ", 2)
	returnValue := ""
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		returnValue = strings.TrimSpace(parts[1])
	}
	return returnValue, returnValue != ""
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request, detail string) {
	httpx.WriteProblem(w, httpx.ErrUnauthorized.WithDetail(detail).Problem(chimw.GetReqID(r.Context())))
}

func writeForbidden(w http.ResponseWriter, r *http.Request, detail string) {
	httpx.WriteProblem(w, httpx.ErrForbidden.WithDetail(detail).Problem(chimw.GetReqID(r.Context())))
}
