package server

import (
	"net/http"
	"strings"

	"xiaodou/dai/libs/go/httpx"
)

const (
	defaultMaxBodyBytes   = 64 << 20
	defaultMaxHeaderBytes = 32 << 10
)

// SecurityHeaders applies the browser-facing response headers to every
// response, including framework 404/405 and panic responses. HSTS is enabled
// only when the deployment is known to be HTTPS at its public boundary.
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data: blob:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=(), usb=()")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			if hsts {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoStoreAPI marks API, runtime, probe, and OpenAPI responses as non-cacheable.
// A deliberately public, versioned asset handler may override this header
// before writing its response.
func NoStoreAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isNonCacheablePath(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func isNonCacheablePath(path string) bool {
	for _, prefix := range []string{
		"/api", "/runtime", "/v1", "/v1beta", "/.well-known",
		"/docs", "/openapi.json", "/health", "/ready", "/metrics",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// RequestBodyLimit rejects oversized declared bodies early and caps streamed
// bodies for handlers that do not otherwise install a route-specific limit.
func RequestBodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				httpx.WriteProblem(w, httpx.Problem{
					Status: http.StatusRequestEntityTooLarge,
					Code:   "request_too_large",
				})
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DefaultMaxBodyBytes and DefaultMaxHeaderBytes are exported for configuration
// defaults and tests without duplicating security-sensitive limits.
func DefaultMaxBodyBytes() int64 { return defaultMaxBodyBytes }

func DefaultMaxHeaderBytes() int { return defaultMaxHeaderBytes }
