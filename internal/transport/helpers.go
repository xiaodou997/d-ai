package transport

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"xiaodou/dai/libs/go/httpx"
)

// writeJSON 写出标准 application/json 响应（用于 chi 原生 handler，如 OAuth2 token）。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeProblemRaw 在 chi 原生 handler 中写出统一 problem+json 错误，并关联 request_id。
func writeProblemRaw(w http.ResponseWriter, r *http.Request, ae *httpx.AppError) {
	httpx.WriteProblem(w, ae.Problem(chimw.GetReqID(r.Context())))
}

// isValidClientType 校验登录端类型（决定查哪张用户表）。
func isValidClientType(t string) bool {
	return t == "admin" || t == "tenant" || t == "customer"
}

// userTypeDisplayName 返回用户类型的中文展示名。
func userTypeDisplayName(userType int) string {
	switch userType {
	case 1:
		return "超级管理员"
	case 2:
		return "平台管理员"
	case 3:
		return "租户用户"
	case 4:
		return "终端用户"
	default:
		return "未知"
	}
}

func nowUTC() time.Time { return time.Now().UTC() }

func joinScopes(scopes []string) string { return strings.Join(scopes, " ") }
