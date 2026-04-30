package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

func (s *Server) adminAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return
		}
		s.writeAdminAuditLog(r, ww.Status())
	})
}

func (s *Server) writeAdminAuditLog(r *http.Request, status int) {
	admin, _ := adminFromContext(r.Context())
	actor := admin.Actor
	action := strings.ToLower(r.Method) + " " + r.URL.Path
	objectType, objectID := adminAuditObject(r.URL.Path)
	result := "success"
	if status >= 400 {
		result = "failed"
	}
	summary, _ := json.Marshal(map[string]any{
		"request_id": requestIDFromContext(r.Context()),
		"method":     r.Method,
		"path":       r.URL.Path,
		"query":      r.URL.RawQuery,
		"role":       admin.Role,
		"tenant_id":  admin.TenantID,
		"user_id":    admin.UserID,
		"remote_ip":  r.RemoteAddr,
		"user_agent": r.UserAgent(),
	})
	if _, err := s.queries.CreateAuditLog(r.Context(), dbgen.CreateAuditLogParams{
		Actor:          optionalTextValue(actor),
		Action:         action,
		ObjectType:     optionalTextValue(objectType),
		ObjectID:       optionalTextValue(objectID),
		RequestSummary: summary,
		Result:         result,
		HttpStatus:     optionalInt4Value(int32(status)),
	}); err != nil {
		s.logger.Error("record admin audit log failed", "error", err, "request_id", requestIDFromContext(r.Context()))
	}
}

func adminAuditObject(path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" {
		parts = parts[2:]
	}
	if len(parts) == 0 {
		return "", ""
	}
	switch parts[0] {
	case "providers":
		if len(parts) >= 2 {
			if len(parts) >= 4 && parts[2] == "endpoints" {
				return "provider_endpoint", parts[3]
			}
			return "provider", parts[1]
		}
		return "provider", ""
	case "upstream-deployments":
		if len(parts) >= 2 {
			if len(parts) >= 4 && parts[2] == "cost-prices" {
				return "upstream_deployment_cost_price", parts[3]
			}
			return "upstream_deployment", parts[1]
		}
		return "upstream_deployment", ""
	case "models":
		if len(parts) >= 2 {
			if len(parts) >= 4 && parts[2] == "routes" {
				return "model_route", parts[3]
			}
			if len(parts) >= 4 && parts[2] == "prices" {
				return "model_price", parts[3]
			}
			return "model", parts[1]
		}
		return "model", ""
	case "limit-policies":
		if len(parts) >= 2 {
			return "runtime_limit_policy", parts[1]
		}
		return "runtime_limit_policy", ""
	case "tenants":
		if len(parts) >= 2 {
			return "tenant", parts[1]
		}
	}
	return parts[0], ""
}

func (s *Server) handleAdminListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeAdminError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = int32(parsed)
	}

	rows, err := s.queries.ListAuditLogs(r.Context(), limit)
	if err != nil {
		s.writeAdminServerError(w, r, "list admin audit logs failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}
