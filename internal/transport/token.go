package transport

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	"xiaodou/dai/internal/config"
	serviceaccesssvc "xiaodou/dai/internal/serviceaccess"
)

// urmClientID 是 URM 自身作为业务系统的标识（登录 URM 后台时使用）。
const urmClientID = "urm"
const ssoSessionCookieName = "urm_sso_session"

// ssoCookieName 按 client_type 隔离 SSO 会话 cookie：admin/tenant/customer 三端在同一
// 浏览器（dev 下同 localhost，cookie 不区分端口）各持独立会话、互不覆盖，可同时登录。
// 与 v4「处处按 client_type 区分」一致；生产分子域时也无副作用。空 client_type 退回旧的
// 无后缀共享名，仅用于兼容清理历史 cookie。
func ssoCookieName(clientType string) string {
	if clientType == "" {
		return ssoSessionCookieName
	}
	return ssoSessionCookieName + "_" + clientType
}

// oauth2TokenResponse 是 OAuth2 token 端点的标准成功响应（snake_case）。
type oauth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

type ssoSessionService interface {
	IsEnabled() bool
	CreateSession(ctx context.Context, data auth.SessionData) (string, error)
	GetSession(ctx context.Context, sessionID string) (*auth.SessionData, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// authHandlers 承载 OAuth2 token 端点（chi 原生：form 输入 / 标准 OAuth2 JSON 输出，
// 不经 Huma）。authorize/exchange/SSO 登出与 token 的 session cookie + 审计见 b-2b。
type authHandlers struct {
	repo                        *authpg.AuthRepository
	jwt                         *auth.JWTService
	session                     ssoSessionService
	sso                         config.SSOConfig
	legal                       config.LegalConfig
	log                         *zap.Logger
	access                      *serviceaccesssvc.Service
	checkSessionPrincipalActive func(context.Context, string, string, int) (bool, error)
}

func newAuthHandlers(d Deps) *authHandlers {
	repo := authpg.NewAuthRepository(d.Pool)
	return &authHandlers{
		repo:                        repo,
		jwt:                         d.JWT,
		session:                     d.Session,
		sso:                         d.SSO,
		legal:                       d.Legal,
		log:                         d.Logger,
		access:                      d.ServiceAccess,
		checkSessionPrincipalActive: repo.CheckSessionPrincipalActive,
	}
}

// token 是 POST /api/oauth2/token 入口，按 grant_type 分发。
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
	case "authorization_code":
		h.authorizationCodeGrant(w, r)
	default:
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("unsupported grant_type"))
	}
}

// loginPrincipal 是凭证校验通过后的用户主体，供 token 端点与 SSO 托管登录页共用。
type loginPrincipal struct {
	UserID          string
	Username        string
	TenantID        string
	UserType        int
	UserTypeDisplay string
}

// authenticateUser 校验用户名/密码并完成账户状态、租户状态与（非 URM 时的）业务系统授权检查，
// 是 ROPC（password grant）与 SSO 托管登录页共享的唯一鉴别入口。
// clientID == urm 时只做身份认证（授权交由后续 /authorize 的 ensureClientGrant 把关）。
func (h *authHandlers) authenticateUser(ctx context.Context, clientType, clientID, username, password string) (loginPrincipal, *httpx.AppError) {
	isURM := clientID == urmClientID
	var userID, passwordHash, tenantID, userTypeDisplay string
	var userType int
	userStatus := "active"

	switch clientType {
	case "admin":
		u, err := h.repo.GetSystemUserForLogin(ctx, username)
		if err != nil {
			return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名或密码错误")
		}
		userID, passwordHash, userType, userStatus = u.UserID, u.PasswordHash, int(u.UserType), u.Status
		userTypeDisplay = userTypeDisplayName(userType)
	case "tenant":
		u, err := h.repo.GetTenantUserForLogin(ctx, username)
		if err != nil {
			return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名或密码错误")
		}
		userID, passwordHash, tenantID, userStatus = u.UserID, u.PasswordHash, u.TenantID, u.Status
		userType, userTypeDisplay = 3, "租户用户"
	case "customer":
		u, err := h.repo.GetEndUserForLogin(ctx, username)
		if err != nil {
			return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名或密码错误")
		}
		userID, passwordHash, tenantID, userStatus = u.UserID, u.PasswordHash, u.TenantID, u.Status
		userType, userTypeDisplay = 4, "终端用户"
	default:
		return loginPrincipal{}, httpx.ErrBadRequest.WithDetail("Missing or invalid X-Client-Type header")
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("用户名或密码错误")
	}
	if userStatus != "active" {
		return loginPrincipal{}, httpx.ErrUnauthorized.WithDetail("账户已被禁用，请联系管理员")
	}

	if isURM {
		if clientType == "tenant" || clientType == "customer" {
			active, err := h.repo.CheckTenantActive(ctx, tenantID)
			if err != nil {
				return loginPrincipal{}, httpx.ErrInternal.WithCause(err)
			}
			if !active {
				return loginPrincipal{}, httpx.ErrForbidden.WithDetail("租户已被停用或暂停")
			}
		}
	} else if err := h.checkClientAccess(ctx, userType, userID, tenantID, clientID); err != nil {
		return loginPrincipal{}, err
	}

	switch clientType {
	case "admin":
		_ = h.repo.UpdateSystemUserLoginTime(ctx, userID, nowUTC())
	case "tenant":
		_ = h.repo.UpdateTenantUserLoginTime(ctx, userID, nowUTC())
	case "customer":
		_ = h.repo.UpdateEndUserLoginTime(ctx, userID, nowUTC())
	}

	return loginPrincipal{
		UserID:          userID,
		Username:        username,
		TenantID:        tenantID,
		UserType:        userType,
		UserTypeDisplay: userTypeDisplay,
	}, nil
}

func (h *authHandlers) passwordGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientType := r.Header.Get("X-Client-Type")
	if !isValidClientType(clientType) {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("Missing or invalid X-Client-Type header"))
		return
	}
	clientID := r.Header.Get("X-Client-Id")
	if clientID == "" {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("Missing X-Client-Id header"))
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")

	p, aerr := h.authenticateUser(ctx, clientType, clientID, username, password)
	if aerr != nil {
		writeProblemRaw(w, r, aerr)
		return
	}

	pair, err := h.jwt.GenerateTokenPair(p.UserID, p.Username, p.TenantID, p.UserType, p.UserTypeDisplay, clientID, clientType)
	if err != nil {
		h.log.Error("generate token pair failed",
			principalLogFields(p.UserID, p.TenantID, clientType, clientID, zap.Error(err))...,
		)
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	h.log.Info("user logged in", principalLogFields(p.UserID, p.TenantID, clientType, clientID)...)
	h.setSessionCookie(w, r, auth.SessionData{
		UserID:          p.UserID,
		Username:        p.Username,
		UserType:        p.UserType,
		UserTypeDisplay: p.UserTypeDisplay,
		TenantID:        p.TenantID,
		ClientType:      clientType,
		ClientID:        clientID,
	})

	writeJSON(w, http.StatusOK, oauth2TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *authHandlers) authorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := strings.TrimSpace(r.PostFormValue("code"))
	redirectURI := strings.TrimSpace(r.PostFormValue("redirect_uri"))
	codeVerifier := strings.TrimSpace(r.PostFormValue("code_verifier"))
	if code == "" || redirectURI == "" {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("missing code or redirect_uri"))
		return
	}
	if codeVerifier == "" {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("missing PKCE code_verifier"))
		return
	}
	rec, err := h.repo.ConsumeAuthCode(ctx, code, "", redirectURI)
	if err != nil {
		h.log.Warn("consume auth code failed",
			principalLogFields("", "", "", "", zap.String("redirectUri", redirectURI), zap.Error(err))...,
		)
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	if rec == nil {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("authorization code invalid or expired"))
		return
	}
	// PKCE 校验：S256(code_verifier) 必须等于授权时绑定的 code_challenge。
	if !verifyPKCE(codeVerifier, rec.CodeChallenge) {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("PKCE verification failed"))
		return
	}
	pair, err := h.jwt.GenerateTokenPair(
		rec.UserID,
		rec.Username,
		rec.TenantID,
		rec.UserType,
		userTypeDisplayName(rec.UserType),
		rec.ClientID,
		rec.ClientType,
	)
	if err != nil {
		h.log.Error("generate token pair from auth code failed",
			principalLogFields(rec.UserID, rec.TenantID, rec.ClientType, rec.ClientID, zap.Error(err))...,
		)
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	h.setSessionCookie(w, r, auth.SessionData{
		UserID:          rec.UserID,
		Username:        rec.Username,
		UserType:        rec.UserType,
		UserTypeDisplay: userTypeDisplayName(rec.UserType),
		TenantID:        rec.TenantID,
		ClientType:      rec.ClientType,
		ClientID:        rec.ClientID,
	})

	writeJSON(w, http.StatusOK, oauth2TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *authHandlers) refreshGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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

	if claims.ClientID != "" {
		if claims.ClientID == urmClientID {
			if claims.UserType == 3 || claims.UserType == 4 {
				active, err := h.repo.CheckTenantActive(ctx, claims.TenantID)
				if err != nil {
					writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
					return
				}
				if !active {
					writeProblemRaw(w, r, httpx.ErrForbidden.WithDetail("租户已被停用或暂停，请重新登录"))
					return
				}
			}
		} else if accessErr := h.checkClientAccess(ctx, claims.UserType, claims.UserID, claims.TenantID, claims.ClientID); accessErr != nil {
			writeProblemRaw(w, r, accessErr)
			return
		}
	}

	pair, err := h.jwt.RefreshTokenPair(refreshToken, true)
	if err != nil {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("Refresh Token 无效或已过期"))
		return
	}
	writeJSON(w, http.StatusOK, oauth2TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *authHandlers) authorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.session == nil || !h.session.IsEnabled() {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("SSO session is not available"))
		return
	}
	if err := r.ParseForm(); err != nil {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("invalid authorize request"))
		return
	}
	responseType := r.FormValue("response_type")
	clientID := strings.TrimSpace(r.FormValue("client_id"))
	clientType := strings.TrimSpace(r.FormValue("client_type"))
	redirectURI := strings.TrimSpace(r.FormValue("redirect_uri"))
	state := r.FormValue("state")
	codeChallenge := strings.TrimSpace(r.FormValue("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(r.FormValue("code_challenge_method"))
	if responseType != "code" {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("unsupported response_type"))
		return
	}
	if clientID == "" || redirectURI == "" || !isValidClientType(clientType) {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("missing client_id, client_type or redirect_uri"))
		return
	}
	// redirect_uri 前缀白名单：防止开放重定向把授权码引到外部站点。
	if !h.redirectURIAllowed(redirectURI) {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("redirect_uri is not allowed"))
		return
	}
	// PKCE 对公开客户端（浏览器 SPA）强制启用，仅支持 S256。
	if codeChallenge == "" || codeChallengeMethod != "S256" {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("missing or unsupported PKCE code_challenge (S256 required)"))
		return
	}

	sessionData, ok := h.sessionFromCookie(ctx, r, clientType)
	if !ok {
		h.redirectToLogin(w, r)
		return
	}
	// 会话类型与目标门户不匹配时，不再硬 403 造成死路，而是跳到登录页让用户以正确
	// 身份重新登录（cookie 已按 client_type 隔离，正常情况下不会走到这里——这是兜底）。
	if sessionData.ClientType != "" && sessionData.ClientType != clientType {
		h.invalidateSessionAndRedirectToLogin(w, r, clientType)
		return
	}
	if !userTypeAllowedForClientType(sessionData.UserType, clientType) {
		h.invalidateSessionAndRedirectToLogin(w, r, clientType)
		return
	}
	principalActive, err := h.sessionPrincipalActive(ctx, sessionData)
	if err != nil {
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	if !principalActive {
		h.invalidateSessionAndRedirectToLogin(w, r, clientType)
		return
	}
	if err := h.ensureClientGrant(ctx, sessionData.UserID, sessionData.TenantID, sessionData.UserType, clientID); err != nil {
		// For the URM client, Forbidden can only mean that the tenant changed
		// state between principal validation and grant validation.
		if clientID == urmClientID && err.Status == http.StatusForbidden {
			h.invalidateSessionAndRedirectToLogin(w, r, clientType)
			return
		}
		writeProblemRaw(w, r, err)
		return
	}

	code, err := randomCode()
	if err != nil {
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	if err := h.repo.CreateAuthCode(ctx, authpg.AuthCodeRecord{
		Code:                code,
		ClientID:            clientID,
		ClientType:          clientType,
		UserID:              sessionData.UserID,
		Username:            sessionData.Username,
		TenantID:            sessionData.TenantID,
		UserType:            sessionData.UserType,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().UTC().Add(5 * time.Minute),
	}); err != nil {
		h.log.Error("create auth code failed",
			principalLogFields(sessionData.UserID, sessionData.TenantID, clientType, clientID, zap.Error(err))...,
		)
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}

	target, err := url.Parse(redirectURI)
	if err != nil {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("invalid redirect_uri"))
		return
	}
	q := target.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (h *authHandlers) ssoLogout(w http.ResponseWriter, r *http.Request) {
	// 指定 client_type 时只登出该门户（保留其它端的会话）；未指定则三端全部登出。
	clientType := strings.TrimSpace(r.URL.Query().Get("client_type"))
	names := []string{"admin", "tenant", "customer"}
	if isValidClientType(clientType) {
		names = []string{clientType}
	}
	for _, ct := range names {
		if h.session != nil && h.session.IsEnabled() {
			if cookie, err := r.Cookie(ssoCookieName(ct)); err == nil && cookie.Value != "" {
				_ = h.session.DeleteSession(r.Context(), cookie.Value)
			}
		}
		http.SetCookie(w, h.expiredSessionCookie(ct))
	}
	// 兼容清理历史遗留的无后缀共享 cookie。
	http.SetCookie(w, h.expiredSessionCookie(""))
	target := r.URL.Query().Get("post_logout_redirect_uri")
	if target == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *authHandlers) setSessionCookie(w http.ResponseWriter, r *http.Request, data auth.SessionData) {
	if h.session == nil || !h.session.IsEnabled() {
		return
	}
	sessionID, err := h.session.CreateSession(r.Context(), data)
	if err != nil {
		h.log.Warn("create SSO session failed",
			principalLogFields(data.UserID, data.TenantID, data.ClientType, data.ClientID, zap.Error(err))...,
		)
		return
	}
	http.SetCookie(w, h.sessionCookie(data.ClientType, sessionID))
}

func (h *authHandlers) sessionFromCookie(ctx context.Context, r *http.Request, clientType string) (*auth.SessionData, bool) {
	cookie, err := r.Cookie(ssoCookieName(clientType))
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	data, err := h.session.GetSession(ctx, cookie.Value)
	if err != nil || data == nil {
		return nil, false
	}
	return data, true
}

// redirectToLogin 把未持有 SSO 会话的 /authorize 请求 302 到 URM 自己托管的登录页。
// 登录页与回跳 next 都以 IssuerURL（含 edge 前缀）为基准拼成公网绝对地址，
// 因此登录页落在 URM 源、绝不会再命中客户端 SPA 的受保护路由 —— 从架构上杜绝重定向环路。
func (h *authHandlers) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if h.sso.IssuerURL == "" {
		writeProblemRaw(w, r, httpx.ErrInternal.WithDetail("sso issuer_url not configured"))
		return
	}
	loginURL, err := url.Parse(strings.TrimRight(h.sso.IssuerURL, "/") + "/api/oauth2/login")
	if err != nil {
		writeProblemRaw(w, r, httpx.ErrInternal.WithCause(err))
		return
	}
	// next 是浏览器回到 /authorize 时要访问的公网绝对地址（带原始查询串）。
	next := strings.TrimRight(h.sso.IssuerURL, "/") + "/api/oauth2/authorize?" + r.URL.RawQuery
	q := loginURL.Query()
	q.Set("next", next)
	q.Set("client_type", r.URL.Query().Get("client_type"))
	loginURL.RawQuery = q.Encode()
	http.Redirect(w, r, loginURL.String(), http.StatusFound)
}

func (h *authHandlers) invalidateSessionAndRedirectToLogin(w http.ResponseWriter, r *http.Request, clientType string) {
	if cookie, err := r.Cookie(ssoCookieName(clientType)); err == nil && cookie.Value != "" {
		if h.session != nil && h.session.IsEnabled() {
			_ = h.session.DeleteSession(r.Context(), cookie.Value)
		}
	}
	http.SetCookie(w, h.expiredSessionCookie(clientType))
	h.redirectToLogin(w, r)
}

func (h *authHandlers) sessionCookie(clientType, value string) *http.Cookie {
	return &http.Cookie{
		Name:     ssoCookieName(clientType),
		Value:    value,
		Path:     "/",
		Domain:   h.sso.CookieDomain,
		MaxAge:   int(h.sso.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.sso.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func (h *authHandlers) expiredSessionCookie(clientType string) *http.Cookie {
	c := h.sessionCookie(clientType, "")
	c.MaxAge = -1
	c.Expires = time.Unix(0, 0)
	return c
}

func (h *authHandlers) ensureClientGrant(ctx context.Context, userID, tenantID string, userType int, clientID string) *httpx.AppError {
	if clientID == urmClientID {
		if userType == 3 || userType == 4 {
			active, err := h.repo.CheckTenantActive(ctx, tenantID)
			if err != nil {
				return httpx.ErrInternal.WithCause(err)
			}
			if !active {
				return httpx.ErrForbidden.WithDetail("租户已被停用或暂停")
			}
		}
		return nil
	}
	return h.checkClientAccess(ctx, userType, userID, tenantID, clientID)
}

func (h *authHandlers) sessionPrincipalActive(ctx context.Context, data *auth.SessionData) (bool, error) {
	if h.checkSessionPrincipalActive != nil {
		return h.checkSessionPrincipalActive(ctx, data.UserID, data.TenantID, data.UserType)
	}
	return h.repo.CheckSessionPrincipalActive(ctx, data.UserID, data.TenantID, data.UserType)
}

func (h *authHandlers) checkClientAccess(ctx context.Context, userType int, userID, tenantID, clientID string) *httpx.AppError {
	if h.access == nil {
		return httpx.New("service_access_unavailable", http.StatusServiceUnavailable, "Service Access Unavailable")
	}
	err := h.access.Check(ctx, userType, userID, tenantID, clientID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, serviceaccesssvc.ErrDenied):
		return httpx.New("service_access_denied", http.StatusForbidden, "Service Access Denied")
	default:
		return httpx.New("service_access_unavailable", http.StatusServiceUnavailable, "Service Access Unavailable")
	}
}

func userTypeAllowedForClientType(userType int, clientType string) bool {
	switch clientType {
	case "admin":
		return userType == 1 || userType == 2
	case "tenant":
		return userType == 3
	case "customer":
		return userType == 4
	default:
		return false
	}
}

func randomCode() (string, error) {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
