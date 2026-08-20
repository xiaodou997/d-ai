package logger

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"xiaodou/dai/internal/weborigin"
)

// ChiRequestLogger returns an http.Handler middleware that logs every HTTP request
// with a unified format across the D-AI server.
//
// Fields (snake_case, Go convention):
//
//	request_id, method, path, status, latency, client_ip, bytes
//
// Plus optional fields extracted from request context:
//
//	tenant_id, user_id, role, api_key_id_hash
//
// Log level by status:
//
//	5xx → Error, 4xx → Warn, otherwise → Info
func ChiRequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			ctx, errorRecorder := withRequestErrorRecorder(r.Context())
			r = r.WithContext(ctx)

			// Extract request ID from chi context
			requestID := middleware.GetReqID(r.Context())
			if requestID != "" {
				w.Header().Set("X-Request-ID", requestID)
			}

			defer func() {
				if recovered := recover(); recovered != nil {
					if ww.Status() == 0 {
						ww.WriteHeader(http.StatusInternalServerError)
					}
					logger.Error("HTTP request panic",
						zap.Any("error", recovered),
						zap.Stack("stack"),
						zap.String("request_id", requestID),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
				}

				latency := time.Since(start)
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}

				// Resolve route pattern for cleaner path logging
				path := r.URL.Path
				if rctx := chi.RouteContext(r.Context()); rctx != nil {
					if pattern := rctx.RoutePattern(); pattern != "" {
						path = pattern
					}
				}

				fields := []zap.Field{
					zap.String("request_id", requestID),
					zap.String("method", r.Method),
					zap.String("path", path),
					zap.Int("status", status),
					zap.Duration("latency", latency),
					zap.String("client_ip", weborigin.ClientIPFromRequest(r)),
					zap.Int("bytes", ww.BytesWritten()),
				}

				// Extract identity fields from context (if set by auth middleware)
				if v, ok := r.Context().Value(contextKeyTenantID).(string); ok && v != "" {
					fields = append(fields, zap.String("tenant_id", v))
				}
				if v, ok := r.Context().Value(contextKeyUserID).(string); ok && v != "" {
					fields = append(fields, zap.String("user_id", v))
				}
				if v, ok := r.Context().Value(contextKeyRole).(string); ok && v != "" {
					fields = append(fields, zap.String("role", v))
				}
				if v, ok := r.Context().Value(contextKeyAPIKeyHash).(string); ok && v != "" {
					fields = append(fields, zap.String("api_key_id_hash", v))
				}
				if status >= http.StatusInternalServerError && errorRecorder.err != nil {
					fields = append(fields, zap.Error(errorRecorder.err))
					if cause := rootCause(errorRecorder.err); cause != nil && cause != errorRecorder.err {
						fields = append(fields, zap.String("error_cause", cause.Error()))
					}
				}

				switch {
				case status >= http.StatusInternalServerError:
					logger.Error("HTTP Request", fields...)
				case status >= http.StatusBadRequest:
					logger.Warn("HTTP Request", fields...)
				default:
					logger.Info("HTTP Request", fields...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// Context keys for request identity fields.
// Services should use these to set identity info via context.WithValue.
type contextKeyType struct{}

var (
	contextKeyTenantID   = contextKeyType{}
	contextKeyUserID     = contextKeyType{}
	contextKeyRole       = contextKeyType{}
	contextKeyAPIKeyHash = contextKeyType{}
)

// WithTenantID returns a copy of the request context with tenant_id set.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, contextKeyTenantID, tenantID)
}

// WithUserID returns a copy of the request context with user_id set.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, contextKeyUserID, userID)
}

// WithRole returns a copy of the request context with role set.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, contextKeyRole, role)
}

// WithAPIKeyHash returns a copy of the request context with api_key_id_hash set.
func WithAPIKeyHash(ctx context.Context, hash string) context.Context {
	return context.WithValue(ctx, contextKeyAPIKeyHash, hash)
}
