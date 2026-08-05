package transport

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/internal/auth"
)

// verifyPKCE 校验 PKCE（RFC 7636，S256）：base64url(sha256(verifier)) 是否等于授权码绑定的 challenge。
// 使用常量时间比较，避免计时侧信道。
func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// validNext 校验登录成功后的回跳目标：必须是本 issuer 下的 /api/oauth2/authorize 公网地址，
// 防止登录页被当作开放重定向（open redirect）跳板。
func (h *authHandlers) validNext(next string) bool {
	if next == "" || h.sso.IssuerURL == "" {
		return false
	}
	want := strings.TrimRight(h.sso.IssuerURL, "/") + "/api/oauth2/authorize"
	if !strings.HasPrefix(next, want+"?") && next != want {
		return false
	}
	u, err := url.Parse(next)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// redirectURIAllowed 判断 /authorize 的 redirect_uri 是否命中配置的前缀白名单。
// 未配置任何白名单时拒绝一切（安全默认：fail closed）。
func (h *authHandlers) redirectURIAllowed(redirectURI string) bool {
	for _, allowed := range h.sso.AllowedRedirectURIs {
		if allowed != "" && strings.HasPrefix(redirectURI, allowed) {
			return true
		}
	}
	return false
}

// ssoLogin 同时处理托管登录页的 GET（渲染表单）与 POST（校验凭证并回跳 authorize）。
func (h *authHandlers) ssoLogin(w http.ResponseWriter, r *http.Request) {
	if h.session == nil || !h.session.IsEnabled() {
		writeProblemRaw(w, r, httpx.ErrUnauthorized.WithDetail("SSO session is not available"))
		return
	}
	if err := r.ParseForm(); err != nil {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("invalid login request"))
		return
	}

	next := r.FormValue("next")
	clientType := strings.TrimSpace(r.FormValue("client_type"))
	if !h.validNext(next) || !isValidClientType(clientType) {
		writeProblemRaw(w, r, httpx.ErrBadRequest.WithDetail("invalid or missing next / client_type"))
		return
	}

	// 已持有匹配会话时直接回跳，避免重复登录。
	if data, ok := h.sessionFromCookie(r.Context(), r, clientType); ok && (data.ClientType == "" || data.ClientType == clientType) {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}

	if r.Method == http.MethodGet {
		renderLoginPage(w, http.StatusOK, loginPageData{
			Next:       next,
			ClientType: clientType,
			TermsURL:   legalDocumentURL(h.legal, "terms"),
			PrivacyURL: legalDocumentURL(h.legal, "privacy"),
		})
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	p, aerr := h.authenticateUser(r.Context(), clientType, urmClientID, username, password)
	if aerr != nil {
		// 凭证错误回到登录页内联提示，而非 problem+json，保证整页表单体验。
		renderLoginPage(w, http.StatusUnauthorized, loginPageData{
			Next:       next,
			ClientType: clientType,
			Username:   username,
			Error:      aerr.Detail,
			TermsURL:   legalDocumentURL(h.legal, "terms"),
			PrivacyURL: legalDocumentURL(h.legal, "privacy"),
		})
		return
	}

	h.setSessionCookie(w, r, auth.SessionData{
		UserID:          p.UserID,
		Username:        p.Username,
		UserType:        p.UserType,
		UserTypeDisplay: p.UserTypeDisplay,
		TenantID:        p.TenantID,
		ClientType:      clientType,
		ClientID:        urmClientID,
	})
	h.log.Info("sso login", principalLogFields(p.UserID, p.TenantID, clientType, urmClientID)...)
	http.Redirect(w, r, next, http.StatusFound)
}

type loginPageData struct {
	Next       string
	ClientType string
	Username   string
	Error      string
	TermsURL   string
	PrivacyURL string
}

// loginTheme 与 Portal 的 ds-theme-{admin,tenant,customer} 主题色严格对齐，
// 使登录页从配色上即可区分平台：admin 紫 / tenant 蓝 / customer 陶土橙。
type loginTheme struct {
	Label       string // 平台名（醒目徽标）
	Tagline     string // 平台副标题
	Accent      string
	AccentHover string
	AccentSoft  string
	Paper       string
	Ink         string
	Muted       string
	Line        string
}

var loginThemes = map[string]loginTheme{
	"admin": {
		Label: "管理平台", Tagline: "Admin · 平台运营与系统管理",
		Accent: "#6e56cf", AccentHover: "#5b45b5", AccentSoft: "#eceaf9",
		Paper: "#f6f7f9", Ink: "#1d1d24", Muted: "#66697a", Line: "#e6e9ef",
	},
	"tenant": {
		Label: "租户控制台", Tagline: "Tenant · 租户资源与成员管理",
		Accent: "#2f6ad0", AccentHover: "#2556b0", AccentSoft: "#e4edfa",
		Paper: "#f5f7fa", Ink: "#182235", Muted: "#5f6b82", Line: "#e3e8ef",
	},
	"customer": {
		Label: "用户中心", Tagline: "Customer · 个人账户与服务",
		Accent: "#b0603c", AccentHover: "#98502f", AccentSoft: "#f4e5db",
		Paper: "#f6f4ee", Ink: "#1b1a17", Muted: "#6b6760", Line: "#e4e0d6",
	},
}

func renderLoginPage(w http.ResponseWriter, status int, data loginPageData) {
	theme, ok := loginThemes[data.ClientType]
	if !ok {
		theme = loginThemes["admin"]
	}
	// 主题色在 Go 侧拼成可信 CSS 变量注入，避免 html/template 的 CSS 上下文转义。
	vars := template.CSS(fmt.Sprintf(
		":root{--accent:%s;--accent-hover:%s;--accent-soft:%s;--paper:%s;--ink:%s;--muted:%s;--line:%s;}",
		theme.Accent, theme.AccentHover, theme.AccentSoft, theme.Paper, theme.Ink, theme.Muted, theme.Line,
	))
	view := struct {
		loginPageData
		loginTheme
		Vars template.CSS
	}{loginPageData: data, loginTheme: theme, Vars: vars}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 登录页不可被缓存，避免回退键泄露表单态。
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = loginTemplate.Execute(w, view)
}

// 服务端渲染的登录页：表单 action 留空（相对当前 URL）以自动保留 edge 前缀，
// next / client_type 以隐藏域随表单回传；配色由 Vars 按平台注入。
var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Label}}登录</title>
<style>
  {{.Vars}}
  :root { color-scheme: light; }
  * { box-sizing: border-box; }
  body { margin: 0; min-height: 100dvh; display: grid; place-items: center;
    font-family: -apple-system, "Segoe UI", system-ui, "PingFang SC", sans-serif;
    color: var(--ink); padding: 24px;
    background:
      radial-gradient(80% 60% at 50% -10%, var(--accent-soft), transparent 60%),
      linear-gradient(180deg, var(--paper), #ffffff); }
  .panel { width: min(440px, 100%); border: 1px solid var(--line); border-radius: 26px;
    background: #ffffff; padding: 40px 36px;
    box-shadow: 0 30px 70px -36px color-mix(in srgb, var(--accent) 40%, #0b1020); }
  .badge { display: inline-flex; align-items: center; gap: 8px; margin: 16px 0 6px;
    padding: 7px 14px 7px 12px; border-radius: 999px; font-size: 14px; font-weight: 800;
    color: var(--accent); background: var(--accent-soft);
    border: 1px solid color-mix(in srgb, var(--accent) 22%, transparent); }
  .badge .dot { width: 9px; height: 9px; border-radius: 50%; background: var(--accent); }
  h1 { margin: 14px 0 4px; font-size: 25px; line-height: 1.15; }
  .tagline { margin: 0 0 26px; color: var(--muted); font-size: 13px; }
  label { display: grid; gap: 7px; font-size: 13px; font-weight: 600; margin-bottom: 15px; color: var(--ink); }
  input { min-height: 48px; border: 1px solid var(--line); border-radius: 13px; padding: 0 14px;
    font-size: 15px; background: #fff; color: var(--ink); transition: border-color .15s, box-shadow .15s; }
  input:focus { outline: none; border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent); }
  button { width: 100%; min-height: 50px; margin-top: 8px; border: 0; border-radius: 13px;
    background: var(--accent); color: #fff; font-size: 15px; font-weight: 800; cursor: pointer;
    transition: background .15s; }
  button:hover { background: var(--accent-hover); }
  .error { margin: 0 0 18px; padding: 11px 14px; border-radius: 11px;
    background: #fef2f2; color: #b91c1c; font-size: 13px; border: 1px solid #fecaca; }
  .foot { margin: 22px 0 0; text-align: center; font-size: 12px; color: var(--muted); }
  .foot + .foot { margin-top: 10px; }
  .foot a { color: inherit; }
</style>
</head>
<body>
  <div class="panel">
    <div class="badge"><span class="dot"></span>{{.Label}}</div>
    <h1>统一身份登录</h1>
    <p class="tagline">{{.Tagline}}</p>
    {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
    <form method="post" action="">
      <input type="hidden" name="next" value="{{.Next}}">
      <input type="hidden" name="client_type" value="{{.ClientType}}">
      <label>用户名
        <input name="username" autocomplete="username" autofocus value="{{.Username}}">
      </label>
      <label>密码
        <input name="password" type="password" autocomplete="current-password">
      </label>
      <button type="submit">登录 {{.Label}}</button>
    </form>
    <p class="foot">登录后将自动返回你访问的应用</p>
    <p class="foot"><a href="{{.TermsURL}}" target="_blank" rel="noreferrer">服务条款</a> · <a href="{{.PrivacyURL}}" target="_blank" rel="noreferrer">隐私政策</a></p>
  </div>
</body>
</html>`))
