package httpx

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// RequestLogger returns middleware that installs a per-request LogContext,
// recovers panics (emitting a 500 when nothing was written yet), and logs a
// structured access line on completion. Shared by both HTTP planes so identity
// recorded by their auth middleware lands in the same log entry.
func RequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			ctx, logCtx := WithLogContext(r.Context())
			r = r.WithContext(ctx)
			requestID := middleware.GetReqID(ctx)
			if requestID != "" {
				w.Header().Set("X-Request-ID", requestID)
			}

			defer func() {
				if recovered := recover(); recovered != nil {
					if ww.Status() == 0 {
						ww.Header().Set("Content-Type", "application/json; charset=utf-8")
						ww.WriteHeader(http.StatusInternalServerError)
						_, _ = ww.Write([]byte(`{"error":"internal server error"}`))
					}
					logger.Error("HTTP request panic",
						append([]zap.Field{
							zap.Any("error", recovered),
							zap.Stack("stack"),
							zap.String("request_id", requestID),
							zap.String("method", r.Method),
							zap.String("path", r.URL.Path),
						}, IdentityFields(ctx)...)...,
					)
				}

				elapsed := time.Since(start)
				status := responseStatus(ww)
				routePath := routePattern(r)

				fields := []zap.Field{
					zap.String("request_id", requestID),
					zap.String("method", r.Method),
					zap.String("path", routePath),
					zap.Int("status", status),
					zap.Duration("latency", elapsed),
					zap.String("client_ip", r.RemoteAddr),
					zap.Int("bytes", ww.BytesWritten()),
					zap.String("user_agent", r.UserAgent()),
				}
				if logCtx.TenantID != "" {
					fields = append(fields, zap.String("tenant_id", logCtx.TenantID))
				}
				if logCtx.UserID != "" {
					fields = append(fields, zap.String("user_id", logCtx.UserID))
				}
				if logCtx.Role != "" {
					fields = append(fields, zap.String("role", logCtx.Role))
				}
				if logCtx.APIKeyIDHash != "" {
					fields = append(fields, zap.String("api_key_id_hash", logCtx.APIKeyIDHash))
				}

				switch {
				case status >= 500:
					logger.Error("HTTP Request", fields...)
				case status >= 400:
					logger.Warn("HTTP Request", fields...)
				default:
					logger.Info("HTTP Request", fields...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func responseStatus(w middleware.WrapResponseWriter) int {
	if w.Status() == 0 {
		return http.StatusOK
	}
	return w.Status()
}

func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}
