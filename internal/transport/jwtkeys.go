package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/httpx"
)

type jwtKeysOutput struct {
	Body struct {
		Keys  []auth.KeyInfo `json:"keys"`
		Total int            `json:"total"`
	}
}

// registerJWTKeys 注册 JWT 签名密钥管理端点（仅 super_admin capability）。
func registerJWTKeys(api huma.API, d jwtKeysModule) {
	jwtSvc := d.auth.JWT
	superAdmin := huma.Middlewares{userAuth(api, d.auth.JWT, d.auth.Blacklist), requireCapability(api, auth.CapabilitySuperAdmin)}

	huma.Register(api, huma.Operation{OperationID: "list-jwt-keys", Method: http.MethodGet, Path: "/api/v1/jwt-keys",
		Summary: "JWT 签名密钥列表", Tags: []string{"jwt-keys"}, Middlewares: superAdmin},
		func(ctx context.Context, _ *struct{}) (*jwtKeysOutput, error) {
			keys, err := jwtSvc.ListKeys()
			if err != nil {
				return nil, httpx.ErrInternal.WithCause(err)
			}
			out := &jwtKeysOutput{}
			out.Body.Keys = keys
			out.Body.Total = len(keys)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "rotate-jwt-key", Method: http.MethodPost, Path: "/api/v1/jwt-keys/rotate",
		Summary: "轮换 JWT 签名密钥", Tags: []string{"jwt-keys"}, Middlewares: superAdmin},
		func(ctx context.Context, _ *struct{}) (*messageOutput, error) {
			if err := jwtSvc.RotateKey(); err != nil {
				return nil, httpx.ErrInternal.WithCause(err)
			}
			out := &messageOutput{}
			out.Body.Message = "密钥轮换成功，旧密钥将在 24 小时后失效"
			return out, nil
		})
}
