package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	"xiaodou/dai/libs/go/httpx"
)

type authTokenResponse struct {
	AccessToken       string `json:"accessToken"`
	ExpiresIn         int64  `json:"expiresIn"`
	RefreshExpiresIn  int64  `json:"refreshExpiresIn"`
	MFARequired       bool   `json:"mfaRequired,omitempty"`
	MFAChallengeToken string `json:"mfaChallengeToken,omitempty"`
}

type authTokenOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      authTokenResponse
}

type authLogoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		Success bool `json:"success"`
	}
}

const refreshCookieName = "dai_refresh_token"

func refreshCookie(value string, maxAge int64, secure bool) http.Cookie {
	cookie := http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api/auth",
		MaxAge:   int(maxAge),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
	if maxAge > 0 {
		cookie.Expires = time.Now().UTC().Add(time.Duration(maxAge) * time.Second)
	}
	return cookie
}

func clearRefreshCookie(secure bool) http.Cookie {
	return http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/auth",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// authHandlers handles unified Portal credentials and session refresh.
type authHandlers struct {
	repo              *authpg.AuthRepository
	sessions          *auth.SessionService
	activations       *auth.ActivationService
	mfa               *auth.MFAService
	recentAuth        *auth.RecentAuthService
	limiter           *auth.LoginRateLimiter
	activationLimiter *auth.LoginRateLimiter
	mfaLimiter        *auth.LoginRateLimiter
	secureCookies     bool
	log               *zap.Logger
}

func newAuthHandlers(d Deps) *authHandlers {
	return &authHandlers{
		repo:              authpg.NewAuthRepository(d.Pool),
		sessions:          d.Sessions,
		activations:       d.Activations,
		mfa:               d.MFA,
		recentAuth:        d.RecentAuth,
		limiter:           auth.NewLoginRateLimiter(d.Redis),
		activationLimiter: auth.NewScopedRateLimiter(d.Redis, "dai:auth:activation:"),
		mfaLimiter:        auth.NewScopedRateLimiter(d.Redis, "dai:auth:mfa:"),
		secureCookies:     d.SecureCookies,
		log:               d.Logger,
	}
}

type loginInput struct {
	Body struct {
		Username string `json:"username" minLength:"1" doc:"用户名或邮箱"`
		Password string `json:"password" minLength:"1"`
	}
}

type refreshInput struct {
	RefreshTokenCookie string `cookie:"dai_refresh_token" required:"true"`
}

type activateAccountInput struct {
	Body struct {
		Token    string `json:"token" minLength:"1"`
		Password string `json:"password" minLength:"12" maxLength:"72"`
	}
}

type passwordPolicyOutput struct {
	Body auth.PasswordPolicy
}

type mfaVerifyInput struct {
	Body struct {
		ChallengeToken string `json:"challengeToken" minLength:"1"`
		Code           string `json:"code" minLength:"6" maxLength:"6"`
	}
}

type mfaEnrollmentOutput struct {
	Body auth.MFAEnrollment
}

type mfaCodeInput struct {
	Body struct {
		Code string `json:"code" minLength:"6" maxLength:"6"`
	}
}

type recentAuthInput struct {
	Body struct {
		Password string `json:"password" minLength:"1"`
		Code     string `json:"code,omitempty" minLength:"6" maxLength:"6"`
	}
}

type loginPrincipal struct {
	UserID            string
	Username          string
	TenantID          string
	UserType          int
	UserTypeDisplay   string
	CredentialVersion int64
	MFAEnabled        bool
}

func (h *authHandlers) authenticateUser(ctx context.Context, username, password string) (loginPrincipal, *httpx.AppError) {
	u, err := h.repo.GetPortalUserForLogin(ctx, username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名/邮箱或密码错误")
	}
	if u.Status != "active" {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名/邮箱或密码错误")
	}
	if u.CredentialState != "active" {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名/邮箱或密码错误")
	}
	if u.UserType >= 3 {
		active, err := h.repo.CheckTenantActive(ctx, u.TenantID)
		if err != nil {
			return loginPrincipal{}, httpx.ErrInternal.WithCause(err)
		}
		if !active {
			return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名/邮箱或密码错误")
		}
	}

	_ = h.repo.UpdateLoginTime(ctx, u.UserID, nowUTC())

	return loginPrincipal{
		UserID:            u.UserID,
		Username:          u.Username,
		TenantID:          u.TenantID,
		UserType:          u.UserType,
		UserTypeDisplay:   userTypeDisplayName(u.UserType),
		CredentialVersion: u.CredentialVersion,
		MFAEnabled:        u.MFAEnabled,
	}, nil
}

func registerAuthPublic(api huma.API, d Deps) {
	h := newAuthHandlers(d)
	huma.Register(api, huma.Operation{
		OperationID: "auth-login",
		Method:      "POST",
		Path:        "/api/auth/login",
		Summary:     "账号密码登录",
		Tags:        []string{"auth"},
		Middlewares: huma.Middlewares{requireSameOrigin(api), requestClientMetadata(api)},
	}, h.login)
	huma.Register(api, huma.Operation{
		OperationID: "auth-refresh",
		Method:      "POST",
		Path:        "/api/auth/refresh",
		Summary:     "刷新登录凭证",
		Tags:        []string{"auth"},
		Middlewares: huma.Middlewares{requireSameOrigin(api)},
	}, h.refresh)
	huma.Register(api, huma.Operation{
		OperationID: "auth-mfa-verify", Method: "POST", Path: "/api/auth/mfa/verify",
		Summary: "验证管理员 MFA", Tags: []string{"auth"},
		Middlewares: huma.Middlewares{requireSameOrigin(api), requestClientMetadata(api)},
	}, h.verifyMFA)
	huma.Register(api, huma.Operation{
		OperationID: "auth-password-policy", Method: "GET", Path: "/api/auth/password-policy",
		Summary: "获取密码策略", Tags: []string{"auth"},
	}, func(context.Context, *struct{}) (*passwordPolicyOutput, error) {
		return &passwordPolicyOutput{Body: auth.CurrentPasswordPolicy()}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "auth-activate-account", Method: "POST", Path: "/api/auth/activate",
		Summary: "使用一次性令牌激活账号或设置重置后的密码", Tags: []string{"auth"},
		Middlewares: huma.Middlewares{requireSameOrigin(api), requestClientMetadata(api)},
	}, h.activateAccount)
}

func (h *authHandlers) activateAccount(ctx context.Context, input *activateAccountInput) (*messageOutput, error) {
	if h.activations == nil {
		return nil, httpx.ErrUnavailable.WithDetail("激活服务不可用")
	}
	dimensions := auth.LoginRateDimensions{Account: strings.ToLower(strings.TrimSpace(input.Body.Token)), IP: requestClientIP(ctx)}
	decision, err := h.activationLimiter.Check(ctx, dimensions)
	if err != nil {
		return nil, httpx.ErrUnavailable.WithDetail("激活服务暂不可用，请稍后重试")
	}
	if !decision.Allowed {
		return nil, httpx.ErrTooManyReqs.WithDetail("激活尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(decision.RetryAfter)})
	}
	err = h.activations.Activate(ctx, strings.TrimSpace(input.Body.Token), input.Body.Password)
	if err != nil {
		retryAfter, rateErr := h.activationLimiter.RecordFailure(ctx, dimensions)
		if rateErr != nil {
			return nil, httpx.ErrUnavailable.WithDetail("激活服务暂不可用，请稍后重试")
		}
		if retryAfter > 0 {
			return nil, httpx.ErrTooManyReqs.WithDetail("激活尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(retryAfter)})
		}
		switch {
		case errors.Is(err, auth.ErrWeakPassword):
			return nil, httpx.ErrBadRequest.WithDetail(auth.CurrentPasswordPolicy().Description)
		case errors.Is(err, auth.ErrExpiredActivationToken):
			return nil, httpx.ErrConflict.WithDetail("激活凭证已过期，请联系管理员重新签发")
		case errors.Is(err, auth.ErrUsedActivationToken):
			return nil, httpx.ErrConflict.WithDetail("激活凭证已使用，请勿重复提交")
		case errors.Is(err, auth.ErrInvalidActivationToken):
			return nil, httpx.ErrUnauthorized.WithDetail("激活凭证无效")
		default:
			return nil, httpx.ErrInternal.WithCause(err)
		}
	}
	if err := h.activationLimiter.Reset(ctx, dimensions); err != nil {
		return nil, httpx.ErrUnavailable.WithDetail("激活服务暂不可用，请稍后重试")
	}
	out := &messageOutput{}
	out.Body.Message = "密码设置成功，请登录"
	return out, nil
}

func (h *authHandlers) login(ctx context.Context, input *loginInput) (*authTokenOutput, error) {
	username := strings.TrimSpace(input.Body.Username)
	dimensions := auth.LoginRateDimensions{Account: strings.ToLower(username), IP: requestClientIP(ctx), Tenant: h.repo.LookupTenantForLogin(ctx, username)}
	decision, err := h.limiter.Check(ctx, dimensions)
	if err != nil {
		h.audit(ctx, authpg.AuditEvent{EventType: "user_login", PrincipalType: "user", Decision: "error", ReasonCode: "login_rate_limiter_unavailable", ReasonMessage: "登录限速服务不可用"})
		return nil, httpx.ErrUnavailable.WithDetail("登录服务暂不可用，请稍后重试")
	}
	if !decision.Allowed {
		h.audit(ctx, authpg.AuditEvent{EventType: "user_login", PrincipalType: "user", Decision: "deny", ReasonCode: "login_rate_limited", ReasonMessage: "登录尝试触发限速"})
		return nil, httpx.ErrTooManyReqs.WithDetail("登录尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(decision.RetryAfter)})
	}
	p, appErr := h.authenticateUser(ctx, username, input.Body.Password)
	if appErr != nil {
		retryAfter, rateErr := h.limiter.RecordFailure(ctx, dimensions)
		if rateErr != nil {
			h.audit(ctx, authpg.AuditEvent{EventType: "user_login", PrincipalType: "user", Decision: "error", ReasonCode: "login_rate_limiter_unavailable", ReasonMessage: "登录失败计数不可用"})
			return nil, httpx.ErrUnavailable.WithDetail("登录服务暂不可用，请稍后重试")
		}
		h.audit(ctx, authpg.AuditEvent{
			EventType: "user_login", PrincipalType: "user", Decision: "deny",
			ReasonCode: appErr.Code, ReasonMessage: appErr.Detail,
			Metadata: map[string]any{"username": username},
		})
		if retryAfter > 0 {
			return nil, httpx.ErrTooManyReqs.WithDetail("登录尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(retryAfter)})
		}
		return nil, appErr
	}
	dimensions.Tenant = p.TenantID
	if err := h.limiter.Reset(ctx, dimensions); err != nil {
		h.audit(ctx, authpg.AuditEvent{EventType: "user_login", PrincipalType: principalType(p.UserType), UserID: p.UserID, Decision: "error", ReasonCode: "login_rate_limiter_unavailable", ReasonMessage: "登录成功后无法清理限速状态"})
		return nil, httpx.ErrUnavailable.WithDetail("登录服务暂不可用，请稍后重试")
	}
	if p.UserType <= 2 && p.MFAEnabled {
		if h.mfa == nil {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务不可用")
		}
		challenge, err := h.mfa.CreateChallenge(ctx, auth.Principal{UserID: p.UserID, Username: p.Username, TenantID: p.TenantID, UserType: p.UserType, UserTypeDisplay: p.UserTypeDisplay, CredentialVersion: p.CredentialVersion})
		if err != nil {
			h.audit(ctx, authpg.AuditEvent{EventType: "user_login", PrincipalType: "admin", UserID: p.UserID, Decision: "error", ReasonCode: "mfa_challenge_failed", ReasonMessage: "创建 MFA 挑战失败"})
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
		}
		return &authTokenOutput{Body: authTokenResponse{MFARequired: true, MFAChallengeToken: challenge}}, nil
	}
	if h.recentAuth == nil || h.recentAuth.Mark(ctx, p.UserID, "password") != nil {
		return nil, httpx.ErrUnavailable.WithDetail("近期认证服务暂不可用，请稍后重试")
	}
	pair, err := h.sessions.Create(ctx, auth.Principal{
		UserID: p.UserID, Username: p.Username, TenantID: p.TenantID,
		UserType: p.UserType, UserTypeDisplay: p.UserTypeDisplay,
		CredentialVersion: p.CredentialVersion,
	})
	if err != nil {
		h.log.Error("generate token pair failed", principalLogFields(p.UserID, p.TenantID, zap.Error(err))...)
		h.audit(ctx, authpg.AuditEvent{
			EventType: "user_login", PrincipalType: principalType(p.UserType), UserID: p.UserID,
			Decision: "error", ReasonCode: "token_generation_failed", ReasonMessage: "生成登录凭证失败",
		})
		return nil, httpx.ErrInternal.WithCause(err)
	}
	h.audit(ctx, authpg.AuditEvent{
		EventType: "user_login", PrincipalType: principalType(p.UserType), UserID: p.UserID,
		Decision: "success", Metadata: map[string]any{"username": p.Username, "userType": p.UserType},
	})
	h.log.Info("user logged in", principalLogFields(p.UserID, p.TenantID)...)
	return &authTokenOutput{
		SetCookie: []http.Cookie{refreshCookie(pair.RefreshToken, pair.RefreshExpiresIn, h.secureCookies)},
		Body: authTokenResponse{
			AccessToken: pair.AccessToken, ExpiresIn: pair.ExpiresIn, RefreshExpiresIn: pair.RefreshExpiresIn,
		},
	}, nil
}

func (h *authHandlers) verifyMFA(ctx context.Context, input *mfaVerifyInput) (*authTokenOutput, error) {
	if h.mfa == nil {
		return nil, httpx.ErrUnavailable.WithDetail("MFA 服务不可用")
	}
	dimensions := auth.LoginRateDimensions{Account: strings.ToLower(strings.TrimSpace(input.Body.ChallengeToken)), IP: requestClientIP(ctx)}
	decision, err := h.mfaLimiter.Check(ctx, dimensions)
	if err != nil {
		return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
	}
	if !decision.Allowed {
		return nil, httpx.ErrTooManyReqs.WithDetail("MFA 尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(decision.RetryAfter)})
	}
	principal, err := h.mfa.VerifyChallenge(ctx, strings.TrimSpace(input.Body.ChallengeToken), strings.TrimSpace(input.Body.Code))
	if err != nil {
		retryAfter, rateErr := h.mfaLimiter.RecordFailure(ctx, dimensions)
		if rateErr != nil {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
		}
		h.audit(ctx, authpg.AuditEvent{EventType: "mfa_verify", PrincipalType: "admin", Decision: "deny", ReasonCode: "invalid_mfa_code", ReasonMessage: "MFA 验证失败"})
		if retryAfter > 0 {
			return nil, httpx.ErrTooManyReqs.WithDetail("MFA 尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(retryAfter)})
		}
		if errors.Is(err, auth.ErrMFAUnavailable) {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
		}
		return nil, httpx.ErrUnauthorized.WithDetail("MFA 验证码无效或已过期")
	}
	if err := h.mfaLimiter.Reset(ctx, dimensions); err != nil {
		return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
	}
	if h.recentAuth == nil || h.recentAuth.Mark(ctx, principal.UserID, "totp") != nil {
		return nil, httpx.ErrUnavailable.WithDetail("近期认证服务暂不可用，请稍后重试")
	}
	pair, err := h.sessions.Create(ctx, principal)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	h.audit(ctx, authpg.AuditEvent{EventType: "mfa_verify", PrincipalType: "admin", UserID: principal.UserID, Decision: "success"})
	return &authTokenOutput{
		SetCookie: []http.Cookie{refreshCookie(pair.RefreshToken, pair.RefreshExpiresIn, h.secureCookies)},
		Body:      authTokenResponse{AccessToken: pair.AccessToken, ExpiresIn: pair.ExpiresIn, RefreshExpiresIn: pair.RefreshExpiresIn},
	}, nil
}

func (h *authHandlers) refresh(ctx context.Context, input *refreshInput) (*authTokenOutput, error) {
	refreshToken := strings.TrimSpace(input.RefreshTokenCookie)
	if refreshToken == "" {
		return nil, httpx.ErrUnauthorized.WithDetail("Refresh Token 缺失或已过期")
	}
	pair, principal, err := h.sessions.Rotate(ctx, refreshToken)
	auditBase := authpg.AuditEvent{
		EventType: "token_refresh", PrincipalType: principalType(principal.UserType),
		UserID: principal.UserID,
	}
	if err != nil {
		auditBase.Decision = "deny"
		auditBase.ReasonCode = "invalid_refresh_token"
		if errors.Is(err, auth.ErrRefreshTokenReused) {
			auditBase.ReasonCode = "refresh_token_reused"
		}
		auditBase.ReasonMessage = "Refresh Token 无效或已过期"
		h.audit(ctx, auditBase)
		if errors.Is(err, auth.ErrTenantInactive) {
			return nil, httpx.ErrForbidden.WithDetail("租户已被停用或暂停，请重新登录")
		}
		return nil, httpx.ErrUnauthorized.WithDetail("Refresh Token 无效或已过期")
	}
	auditBase.Decision = "success"
	h.audit(ctx, auditBase)
	return &authTokenOutput{
		SetCookie: []http.Cookie{refreshCookie(pair.RefreshToken, pair.RefreshExpiresIn, h.secureCookies)},
		Body: authTokenResponse{
			AccessToken: pair.AccessToken, ExpiresIn: pair.ExpiresIn, RefreshExpiresIn: pair.RefreshExpiresIn,
		},
	}, nil
}

func principalType(userType int) string {
	if userType == 1 || userType == 2 {
		return "admin"
	}
	return "user"
}

func (h *authHandlers) audit(ctx context.Context, event authpg.AuditEvent) {
	event.RequestID = middleware.GetReqID(ctx)
	if err := h.repo.RecordAuditEvent(ctx, event); err != nil {
		h.log.Warn("record auth audit event failed", zap.String("event_type", event.EventType), zap.Error(err))
	}
}
