package transport

import (
	"context"
	"errors"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	"xiaodou/dai/libs/go/httpx"
)

type authTokenResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken,omitempty"`
	ExpiresIn        int64  `json:"expiresIn"`
	RefreshExpiresIn int64  `json:"refreshExpiresIn"`
}

type authTokenOutput struct {
	Body authTokenResponse
}

// authHandlers handles unified Portal credentials and session refresh.
type authHandlers struct {
	repo     *authpg.AuthRepository
	sessions *auth.SessionService
	log      *zap.Logger
}

func newAuthHandlers(d Deps) *authHandlers {
	return &authHandlers{
		repo:     authpg.NewAuthRepository(d.Pool),
		sessions: d.Sessions,
		log:      d.Logger,
	}
}

type loginInput struct {
	Body struct {
		Username string `json:"username" minLength:"1" doc:"用户名或邮箱"`
		Password string `json:"password" minLength:"1"`
	}
}

type refreshInput struct {
	Body struct {
		RefreshToken string `json:"refreshToken" minLength:"1"`
	}
}

type loginPrincipal struct {
	UserID            string
	Username          string
	TenantID          string
	UserType          int
	UserTypeDisplay   string
	CredentialVersion int64
}

func (h *authHandlers) authenticateUser(ctx context.Context, username, password string) (loginPrincipal, *httpx.AppError) {
	u, err := h.repo.GetPortalUserForLogin(ctx, username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名/邮箱或密码错误")
	}
	if u.Status != "active" {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("账户已被禁用，请联系管理员")
	}
	if u.UserType >= 3 {
		active, err := h.repo.CheckTenantActive(ctx, u.TenantID)
		if err != nil {
			return loginPrincipal{}, httpx.ErrInternal.WithCause(err)
		}
		if !active {
			return loginPrincipal{}, httpx.ErrForbidden.WithDetail("租户已被停用或暂停")
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
	}, h.login)
	huma.Register(api, huma.Operation{
		OperationID: "auth-refresh",
		Method:      "POST",
		Path:        "/api/auth/refresh",
		Summary:     "刷新登录凭证",
		Tags:        []string{"auth"},
	}, h.refresh)
}

func (h *authHandlers) login(ctx context.Context, input *loginInput) (*authTokenOutput, error) {
	username := strings.TrimSpace(input.Body.Username)
	p, appErr := h.authenticateUser(ctx, username, input.Body.Password)
	if appErr != nil {
		h.audit(ctx, authpg.AuditEvent{
			EventType: "user_login", PrincipalType: "user", Decision: "deny",
			ReasonCode: appErr.Code, ReasonMessage: appErr.Detail,
			Metadata: map[string]any{"username": username},
		})
		return nil, appErr
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
	return &authTokenOutput{Body: authTokenResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken,
		ExpiresIn: pair.ExpiresIn, RefreshExpiresIn: pair.RefreshExpiresIn,
	}}, nil
}

func (h *authHandlers) refresh(ctx context.Context, input *refreshInput) (*authTokenOutput, error) {
	refreshToken := strings.TrimSpace(input.Body.RefreshToken)
	if refreshToken == "" {
		return nil, httpx.ErrBadRequest.WithDetail("缺少 refreshToken")
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
	return &authTokenOutput{Body: authTokenResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken,
		ExpiresIn: pair.ExpiresIn, RefreshExpiresIn: pair.RefreshExpiresIn,
	}}, nil
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
