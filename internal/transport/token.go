package transport

import (
	"context"
	"net/http"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	"xiaodou/dai/libs/go/httpx"
)

type oauth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// authHandlers serves the unified Portal's password and refresh token grants.
type authHandlers struct {
	repo *authpg.AuthRepository
	jwt  *auth.JWTService
	log  *zap.Logger
}

func newAuthHandlers(d Deps) *authHandlers {
	return &authHandlers{
		repo: authpg.NewAuthRepository(d.Pool),
		jwt:  d.JWT,
		log:  d.Logger,
	}
}

func (h *authHandlers) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("invalid form body"))
		return
	}
	switch r.PostFormValue("grant_type") {
	case "password":
		h.passwordGrant(w, r)
	case "refresh_token":
		h.refreshGrant(w, r)
	default:
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("unsupported grant_type"))
	}
}

type loginPrincipal struct {
	UserID          string
	Username        string
	TenantID        string
	UserType        int
	UserTypeDisplay string
}

func (h *authHandlers) authenticateUser(ctx context.Context, username, password string) (loginPrincipal, *httpx.AppError) {
	u, err := h.repo.GetPortalUserForLogin(ctx, username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名或密码错误")
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

	switch {
	case u.UserType <= 2:
		_ = h.repo.UpdateSystemUserLoginTime(ctx, u.UserID, nowUTC())
	case u.UserType == 3:
		_ = h.repo.UpdateTenantUserLoginTime(ctx, u.UserID, nowUTC())
	case u.UserType == 4:
		_ = h.repo.UpdateEndUserLoginTime(ctx, u.UserID, nowUTC())
	}

	return loginPrincipal{
		UserID:          u.UserID,
		Username:        u.Username,
		TenantID:        u.TenantID,
		UserType:        u.UserType,
		UserTypeDisplay: userTypeDisplayName(u.UserType),
	}, nil
}

func (h *authHandlers) passwordGrant(w http.ResponseWriter, r *http.Request) {
	p, appErr := h.authenticateUser(r.Context(), r.PostFormValue("username"), r.PostFormValue("password"))
	if appErr != nil {
		writeProblemRaw(w, r, appErr)
		return
	}
	pair, err := h.jwt.GenerateTokenPair(p.UserID, p.Username, p.TenantID, p.UserType, p.UserTypeDisplay)
	if err != nil {
		h.log.Error("generate token pair failed", principalLogFields(p.UserID, p.TenantID, zap.Error(err))...)
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	h.log.Info("user logged in", principalLogFields(p.UserID, p.TenantID)...)
	writeJSON(w, http.StatusOK, oauth2TokenResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken,
		TokenType: "Bearer", ExpiresIn: pair.ExpiresIn,
	})
}

func (h *authHandlers) refreshGrant(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.PostFormValue("refresh_token")
	if refreshToken == "" {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("Missing refresh_token"))
		return
	}
	claims, err := h.jwt.ParseToken(refreshToken)
	if err != nil || claims.PrincipalType != "user" || claims.TokenUse != "refresh" {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("Refresh Token 无效或已过期"))
		return
	}
	if claims.UserType >= 3 {
		active, err := h.repo.CheckTenantActive(r.Context(), claims.TenantID)
		if err != nil {
			writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
			return
		}
		if !active {
			writeProblemRaw(w, r, httpx.ErrForbidden.WithDetail("租户已被停用或暂停，请重新登录"))
			return
		}
	}
	pair, err := h.jwt.RefreshTokenPair(refreshToken, true)
	if err != nil {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("Refresh Token 无效或已过期"))
		return
	}
	writeJSON(w, http.StatusOK, oauth2TokenResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken,
		TokenType: "Bearer", ExpiresIn: pair.ExpiresIn,
	})
}
