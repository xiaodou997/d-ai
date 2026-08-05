package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
)

type jwksOutput struct {
	Body auth.JWKSResponse
}

// registerJWKS 注册 JWKS 公钥端点，供 AI 网关和其他受信调用方按配置拉取
// 公钥验签。
func registerJWKS(api huma.API, jwt *auth.JWTService) {
	handler := func(_ context.Context, _ *struct{}) (*jwksOutput, error) {
		return &jwksOutput{Body: *jwt.GetJWKS()}, nil
	}
	routes := []struct{ id, path string }{
		{"get-jwks-well-known", "/.well-known/jwks.json"},
		{"get-jwks-public", "/public/jwks.json"},
	}
	for _, r := range routes {
		huma.Register(api, huma.Operation{
			OperationID: r.id,
			Method:      http.MethodGet,
			Path:        r.path,
			Summary:     "JWKS 公钥集",
			Tags:        []string{"oauth2"},
		}, handler)
	}
}
