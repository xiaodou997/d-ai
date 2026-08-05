package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/apikey"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/httpx"
	"xiaodou/dai/libs/go/serviceaccess"
)

type runtimeAuthContextKey struct{}

type RuntimeAuth struct {
	Subject coreidentity.Subject
}

func (s *Gateway) runtimeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := extractRuntimeAPIKey(r.Header)
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

		subject, err := s.buildRuntimeAuthSubject(r.Context(), row)
		if err != nil {
			s.logger.Error("runtime auth: build identity failed",
				gatewayLogFields(r.Context(), row.TenantID, row.UserID.String, zap.Error(err))...,
			)
			writeOpenAIError(w, http.StatusInternalServerError, "Internal server error.", "internal_error", "internal_error")
			return
		}
		if s.rejectIfBanned(w, r.Context(), subject.TenantID, subject.UserID) {
			return
		}
		if s.serviceAccess == nil {
			writeOpenAIError(w, http.StatusServiceUnavailable, "Service access unavailable.", "service_access_unavailable", "service_access_unavailable")
			return
		}
		if err := s.serviceAccess.Check(r.Context(), 4, subject.UserID, subject.TenantID, s.expectedClientID, s.expectedClientID); err != nil {
			if errors.Is(err, serviceaccess.ErrDenied) {
				writeOpenAIError(w, http.StatusForbidden, "Service access denied.", "service_access_denied", "service_access_denied")
			} else {
				writeOpenAIError(w, http.StatusServiceUnavailable, "Service access unavailable.", "service_access_unavailable", "service_access_unavailable")
			}
			return
		}
		if subject.GroupID == "" {
			writeOpenAIError(w, http.StatusForbidden, "API key is not bound to an upstream group.", "invalid_request_error", "api_key_has_no_group")
			return
		}
		ctx := context.WithValue(r.Context(), runtimeAuthContextKey{}, RuntimeAuth{Subject: subject})
		httpx.SetAPIKey(ctx, shortHash(runtimeAuthUUIDString(row.ID)), row.TenantID, subject.UserID)

		keyID := row.ID
		go func() {
			_ = s.queries.TouchLastUsedAt(context.Background(), keyID)
		}()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractRuntimeAPIKey accepts both the OpenAI Bearer convention and the
// Anthropic x-api-key convention. If callers send both, they must identify the
// same key so intermediaries cannot create an ambiguous credential context.
func extractRuntimeAPIKey(headers http.Header) (string, error) {
	authorization := strings.TrimSpace(headers.Get("Authorization"))
	anthropicKey := strings.TrimSpace(headers.Get("x-api-key"))
	if authorization == "" && anthropicKey == "" {
		return "", errors.New("missing api key")
	}

	var bearerKey string
	if authorization != "" {
		var err error
		bearerKey, err = apikey.ExtractBearer(authorization)
		if err != nil {
			return "", err
		}
	}
	if anthropicKey != "" && !strings.HasPrefix(anthropicKey, apikey.Prefix) {
		return "", errors.New("invalid api key prefix")
	}
	if bearerKey != "" && anthropicKey != "" && bearerKey != anthropicKey {
		return "", errors.New("conflicting api key headers")
	}
	if bearerKey != "" {
		return bearerKey, nil
	}
	return anthropicKey, nil
}

// resolveAPIKey checks cache first, falls back to DB, and writes back on miss.
func (s *Gateway) resolveAPIKey(ctx context.Context, keyHash string) (dbgen.GetAPIKeyByHashRow, error) {
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

func (s *Gateway) buildRuntimeAuthSubject(ctx context.Context, row dbgen.GetAPIKeyByHashRow) (coreidentity.Subject, error) {
	return buildRuntimeAuthSubjectRecord(runtimeAPIKeyRecordFromHashRow(row))
}

func runtimeAuthUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
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
