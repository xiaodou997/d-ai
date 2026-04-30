package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"uni-ai-api/backend/internal/apikey"
	dbgen "uni-ai-api/backend/internal/db/gen"
)

type runtimeAuthContextKey struct{}

type RuntimeAuth struct {
	APIKey dbgen.GetAPIKeyByHashRow
}

func (s *Server) runtimeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := apikey.ExtractBearer(r.Header.Get("Authorization"))
		if err != nil {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
			return
		}

		row, err := s.queries.GetAPIKeyByHash(r.Context(), apikey.Hash(key))
		if err != nil {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
			return
		}

		userID := ""
		if row.UserID.Valid {
			userID = row.UserID.String
		}
		ctx := context.WithValue(r.Context(), runtimeAuthContextKey{}, RuntimeAuth{APIKey: row})
		setRequestAPIKey(ctx, shortHash(row.ID.String()), row.TenantID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func runtimeAuthFromContext(ctx context.Context) (RuntimeAuth, bool) {
	auth, ok := ctx.Value(runtimeAuthContextKey{}).(RuntimeAuth)
	return auth, ok
}

type openAIErrorResponse struct {
	Error openAIError `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func writeOpenAIError(w http.ResponseWriter, status int, message, errorType, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openAIErrorResponse{
		Error: openAIError{
			Message: message,
			Type:    errorType,
			Code:    code,
		},
	})
}

func requestIDFromContext(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}
