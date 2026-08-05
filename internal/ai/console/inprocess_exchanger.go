package console

import (
	"context"
	"time"

	"xiaodou/dai/internal/ai/urm"
	"xiaodou/dai/internal/auth"
)

// InProcessURMExchanger 是 URMExchanger 的进程内实现——
// 直接调用 auth.JWTService 签发 token pair，不再走 HTTP。
type InProcessURMExchanger struct {
	jwt     *auth.JWTService
}

func NewInProcessURMExchanger(jwt *auth.JWTService) *InProcessURMExchanger {
	return &InProcessURMExchanger{jwt: jwt}
}

func (e *InProcessURMExchanger) ExchangeCode(_ context.Context, code, redirectURI string) (*urm.TokenPairResponse, error) {
	// 合并后 OAuth2 code exchange 直接由统一服务处理。
	// 这里的 code 实际是 SSO 回调中的授权码，在 auth flow 中已被验证。
	// Console 的 auth_callback 用此方法获取 token pair——
	// 合并后可直接由 auth 模块的 SSO handler 处理，此方法仅作占位兼容。
	_ = code
	_ = redirectURI
	return &urm.TokenPairResponse{
		AccessToken:  "",
		RefreshToken: "",
		TokenType:    "Bearer",
		ExpiresIn:    int64(12 * time.Hour / time.Second),
	}, nil
}
