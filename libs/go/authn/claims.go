package authn

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 URM 颁发的 JWT 载荷契约，跨服务共享。普通用户令牌与
// principal_type=service 的服务令牌共用本结构。
type Claims struct {
	UserID        string `json:"user_id,omitempty"`
	TenantID      string `json:"tenant_id,omitempty"`
	Username      string `json:"username,omitempty"`
	Role          string `json:"role,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	UserType      int    `json:"user_type,omitempty"`
	PrincipalType string `json:"principal_type,omitempty"`
	TokenUse      string `json:"token_use,omitempty"`
	InstanceID    string `json:"instance_id,omitempty"`
	SourceCIDR    string `json:"source_cidr,omitempty"`
	Scope         string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// IsService 判断是否为 URM 服务平面颁发的短期访问令牌。
func (c *Claims) IsService() bool {
	return c.PrincipalType == "service" && c.TokenUse == "access"
}

type ctxKey struct{}

var claimsKey = ctxKey{}

// WithClaims 把校验通过的 Claims 存入上下文。
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext 取出上下文中的 Claims；未认证时返回 nil, false。
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}
