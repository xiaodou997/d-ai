package weborigin

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type contextKey struct{}

// FromRequest returns the public origin represented by an incoming request.
// The tunnel terminates TLS before forwarding to D-AI, so forwarded host and
// protocol headers are honored when present.
func FromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}

	scheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	if scheme != "http" && scheme != "https" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}

func WithOrigin(ctx context.Context, origin string) context.Context {
	return context.WithValue(ctx, contextKey{}, strings.TrimRight(strings.TrimSpace(origin), "/"))
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	origin, _ := ctx.Value(contextKey{}).(string)
	return strings.TrimRight(strings.TrimSpace(origin), "/")
}

// Resolve returns an absolute same-origin URL when the request origin is
// available and otherwise preserves the relative path for non-HTTP callers.
func Resolve(ctx context.Context, path string) string {
	path = "/" + strings.TrimLeft(path, "/")
	if origin := FromContext(ctx); origin != "" {
		return origin + path
	}
	return path
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithOrigin(r.Context(), FromRequest(r))))
	})
}

func firstForwardedValue(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}
