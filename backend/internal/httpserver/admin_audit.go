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
	actor, _ := r.Context().Value(adminActorContextKey{}).(string)
	action := strings.ToLower(r.Method) + " " + r.URL.Path
	objectType, objectID := adminAuditObject(r.URL.Path)
	result := "success"
	if status >= 400 {
		result = "failed"
	}
	summary, _ := json.Marshal(map[string]any{
		"method": r.Method,
		"path":   r.URL.Path,
		"query":  r.URL.RawQuery,
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
	if len(parts) < 2 || parts[0] != "admin" {
		return "", ""
	}
	switch parts[1] {
	case "providers":
		if len(parts) >= 3 {
			if len(parts) >= 5 && parts[3] == "endpoints" {
				return "provider_endpoint", parts[4]
			}
			return "provider", parts[2]
		}
		return "provider", ""
	case "upstream-deployments":
		if len(parts) >= 3 {
			if len(parts) >= 5 && parts[3] == "cost-prices" {
				return "upstream_deployment_cost_price", parts[4]
			}
			return "upstream_deployment", parts[2]
		}
		return "upstream_deployment", ""
	case "models":
		if len(parts) >= 3 {
			if len(parts) >= 5 && parts[3] == "routes" {
				return "model_route", parts[4]
			}
			if len(parts) >= 5 && parts[3] == "prices" {
				return "model_price", parts[4]
			}
			return "model", parts[2]
		}
		return "model", ""
	case "limit-policies":
		if len(parts) >= 3 {
			return "runtime_limit_policy", parts[2]
		}
		return "runtime_limit_policy", ""
	case "tenants":
		if len(parts) >= 3 {
			return "tenant", parts[2]
		}
	}
	return parts[1], ""
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