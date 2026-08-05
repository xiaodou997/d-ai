package gateway

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/urm"
)

// 委托 Token 的固定约束：URM 以此 scope 签发，ai-service 以此 audience 为目标下游。
const (
	delegatedTokenScope    = "ai.invoke"
	delegatedTokenAudience = "ai-service"
)

// delegatedTokenValidator 校验 URM 委托 Token 并返回其 claims（由 urm.JWKSValidator 实现）。
type delegatedTokenValidator interface {
	ValidateToken(ctx context.Context, token string) (*urm.Claims, error)
}

// RegisterInternalTaskRoutes 注册 OBO 内部任务入口 /internal/v1/tasks（仅委托 Token 可用）。
// 未配置 jwksValidator 时不注册，内部入口整体禁用。
func (s *Gateway) RegisterInternalTaskRoutes(r chi.Router) {
	if s.jwksValidator == nil {
		return
	}
	r.Route("/internal/v1", func(r chi.Router) {
		r.Post("/tasks", s.handleCreateInternalTask)
		r.Get("/tasks/{taskID}", s.handleGetInternalTask)
		r.Post("/tasks/{taskID}/cancel", s.handleCancelInternalTask)
	})
}

func (s *Gateway) handleCreateInternalTask(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withDelegatedTaskAuth(w, r, s.handleCreateTaskAuthorized)
}

func (s *Gateway) handleGetInternalTask(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withDelegatedTaskAuth(w, r, s.handleGetTaskAuthorized)
}

func (s *Gateway) handleCancelInternalTask(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withDelegatedTaskAuth(w, r, s.handleCancelTaskAuthorized)
}

// withDelegatedTaskAuth 认证 OBO 委托 Token，构造被代表主体的 Subject 后交给通用任务处理。
func (s *Gateway) withDelegatedTaskAuth(
	w http.ResponseWriter,
	r *http.Request,
	next func(http.ResponseWriter, *http.Request, taskCaller),
) {
	subject, ok := s.delegatedSubject(w, r)
	if !ok {
		return
	}
	if s.rejectIfBanned(w, r.Context(), subject.TenantID, subject.UserID) {
		return
	}
	next(w, r, taskCaller{Subject: subject})
}

// delegatedSubject 校验委托 Token 的签名与全部委托约束（principal_type / scope / aud /
// 调用方白名单），把被代表主体展开为运行时 Subject；actor 服务身份记入 ActorClientID 供审计。
func (s *Gateway) delegatedSubject(w http.ResponseWriter, r *http.Request) (coreidentity.Subject, bool) {
	token := delegatedBearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "Missing token.", "invalid_request_error", "missing_token")
		return coreidentity.Subject{}, false
	}
	claims, err := s.jwksValidator.ValidateToken(r.Context(), token)
	if err != nil {
		s.logger.Warn("delegated token validation failed", zap.Error(err))
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid token.", "invalid_request_error", "invalid_token")
		return coreidentity.Subject{}, false
	}
	if claims.PrincipalType != "delegated" {
		writeOpenAIError(w, http.StatusForbidden, "Token is not a delegation token.", "invalid_request_error", "forbidden")
		return coreidentity.Subject{}, false
	}
	if claims.Scope != delegatedTokenScope || !slices.Contains(claims.Audience, delegatedTokenAudience) {
		writeOpenAIError(w, http.StatusForbidden, "Token not authorized for this service.", "invalid_request_error", "forbidden")
		return coreidentity.Subject{}, false
	}
	if len(s.delegationAllowedClients) > 0 && !slices.Contains(s.delegationAllowedClients, claims.ClientID) {
		writeOpenAIError(w, http.StatusForbidden, "Delegating service is not allowed.", "invalid_request_error", "forbidden")
		return coreidentity.Subject{}, false
	}
	if strings.TrimSpace(claims.TenantID) == "" {
		writeOpenAIError(w, http.StatusForbidden, "Delegation token has no tenant subject.", "invalid_request_error", "forbidden")
		return coreidentity.Subject{}, false
	}

	// 计费/权限主体由 billing_scope 决定（§8.4）：user 带用户主体，tenant 只带租户。
	scope := coreidentity.ScopeTenant
	userID := ""
	if claims.BillingScope == "user" {
		if strings.TrimSpace(claims.UserID) == "" {
			writeOpenAIError(w, http.StatusForbidden, "User-scoped delegation has no user subject.", "invalid_request_error", "forbidden")
			return coreidentity.Subject{}, false
		}
		scope = coreidentity.ScopeUser
		userID = claims.UserID
	}

	return coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodDelegated,
		RequestSource: coreidentity.RequestSourceDelegated,
		Scope:         scope,
		TenantID:      claims.TenantID,
		UserID:        userID,
		ActorClientID: claims.ClientID,
	}, true
}

// delegatedBearerToken 从 Authorization 头解析 Bearer 令牌（大小写不敏感）。
func delegatedBearerToken(header string) string {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
