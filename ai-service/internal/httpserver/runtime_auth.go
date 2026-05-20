package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"xiaodou/uni-ai-api/internal/apikey"
	dbgen "xiaodou/uni-ai-api/internal/db/gen"
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
		keyHash := apikey.Hash(key)

		row, err := s.resolveAPIKey(r.Context(), keyHash)
		if err != nil {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
			return
		}

		if row.Status != "active" {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
			return
		}
		if row.ExpiresAt.Valid && !time.Now().Before(row.ExpiresAt.Time) {
			writeOpenAIError(w, http.StatusUnauthorized, "API key expired.", "invalid_api_key", "invalid_api_key")
			return
		}

		userID := ""
		if row.UserID.Valid {
			userID = row.UserID.String
		}
		ctx := context.WithValue(r.Context(), runtimeAuthContextKey{}, RuntimeAuth{APIKey: row})
		setRequestAPIKey(ctx, shortHash(row.ID.String()), row.TenantID, userID)

		keyID := row.ID
		go func() {
			_ = s.queries.TouchLastUsedAt(context.Background(), keyID)
		}()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveAPIKey checks cache first, falls back to DB, and writes back on miss.
func (s *Server) resolveAPIKey(ctx context.Context, keyHash string) (dbgen.GetAPIKeyByHashRow, error) {
	if s.apiKeyCache != nil {
		if row, ok := s.apiKeyCache.Get(ctx, keyHash); ok {
			return row, nil
		}
	}
	row, err := s.queries.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return dbgen.GetAPIKeyByHashRow{}, err
	}
	if s.apiKeyCache != nil {
		_ = s.apiKeyCache.Set(ctx, keyHash, row)
	}
	return row, nil
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
