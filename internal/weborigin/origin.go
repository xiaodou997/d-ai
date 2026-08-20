package weborigin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type contextKey int

const (
	originContextKey contextKey = iota
	clientIPContextKey
)

// Resolver is the single trust boundary for public origins and client IPs.
// Forwarded headers are only considered when the direct peer belongs to one
// of the configured trusted proxy networks. A configured public origin always
// wins over request headers, which prevents Host-header poisoning in links.
type Resolver struct {
	publicOrigin   string
	trustedProxies []*net.IPNet
}

// NewResolver validates the public origin and trusted proxy CIDR list once at
// startup so request handling never has to make configuration decisions.
func NewResolver(publicBaseURL string, trustedProxyCIDRs []string) (*Resolver, error) {
	origin, err := NormalizePublicOrigin(publicBaseURL)
	if err != nil {
		return nil, err
	}
	proxies := make([]*net.IPNet, 0, len(trustedProxyCIDRs))
	for _, raw := range trustedProxyCIDRs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, errors.New("invalid trusted proxy CIDR: " + value)
		}
		proxies = append(proxies, network)
	}
	return &Resolver{publicOrigin: origin, trustedProxies: proxies}, nil
}

// NormalizePublicOrigin validates an absolute origin without allowing a path,
// query, fragment, userinfo, whitespace, or an unsupported scheme.
func NormalizePublicOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.ContainsAny(raw, "\r\n\t ") {
		return "", errors.New("public base URL must not contain whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("public base URL must be an absolute origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("public base URL scheme must be http or https")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("public base URL must not contain a path, query, or fragment")
	}
	if parsed.Hostname() == "" || strings.ContainsAny(parsed.Host, ",;\\") {
		return "", errors.New("public base URL host is invalid")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// PublicOrigin returns the configured origin, if any.
func (r *Resolver) PublicOrigin() string {
	if r == nil {
		return ""
	}
	return r.publicOrigin
}

// FromRequest returns the trusted public origin represented by a request.
func (r *Resolver) FromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	if r != nil && r.publicOrigin != "" {
		return r.publicOrigin
	}

	if r == nil || !r.isTrustedPeer(req) {
		return ""
	}
	host := forwardedHost(req)
	if host == "" {
		return ""
	}
	scheme := requestScheme(req)
	if forwardedSchemeValue := forwardedScheme(req); forwardedSchemeValue != "" {
		scheme = forwardedSchemeValue
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}

// ClientIP returns the canonical client IP. Forwarded values are ignored
// unless the direct peer is trusted, and multi-hop X-Forwarded-For is walked
// from the nearest proxy to the first untrusted address.
func (r *Resolver) ClientIP(req *http.Request) string {
	if req == nil {
		return ""
	}
	fallback := remoteIP(req.RemoteAddr)
	if fallback == "" {
		fallback = "unknown"
	}
	if r == nil || !r.isTrustedPeer(req) {
		return fallback
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		var first string
		for _, value := range strings.Split(xff, ",") {
			ip := parseIP(value)
			if ip == "" {
				continue
			}
			if first == "" {
				first = ip
			}
			if !r.isTrustedIP(net.ParseIP(ip)) {
				return ip
			}
		}
		if first != "" {
			return first
		}
	}
	if xr := parseIP(req.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	if forwarded := r.forwardedClientIP(req); forwarded != "" {
		return forwarded
	}
	return fallback
}

func (r *Resolver) forwardedClientIP(req *http.Request) string {
	values := strings.Split(req.Header.Get("Forwarded"), ",")
	var first string
	for index := len(values) - 1; index >= 0; index-- {
		ip := parseIP(forwardedParameter(values[index], "for"))
		if ip == "" {
			continue
		}
		first = ip
		if !r.isTrustedIP(net.ParseIP(ip)) {
			return ip
		}
	}
	return first
}

func (r *Resolver) isTrustedPeer(req *http.Request) bool {
	if r == nil || len(r.trustedProxies) == 0 || req == nil {
		return false
	}
	return r.isTrustedIP(net.ParseIP(remoteIP(req.RemoteAddr)))
}

func (r *Resolver) isTrustedIP(ip net.IP) bool {
	if r == nil || ip == nil {
		return false
	}
	for _, network := range r.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func requestScheme(req *http.Request) string {
	if req != nil && req.TLS != nil {
		return "https"
	}
	return "http"
}

func forwardedHost(req *http.Request) string {
	if value := normalizeHost(firstForwardedValue(req.Header.Get("X-Forwarded-Host"))); value != "" {
		return value
	}
	return normalizeHost(forwardedParameter(req.Header.Get("Forwarded"), "host"))
}

func forwardedScheme(req *http.Request) string {
	value := strings.ToLower(firstForwardedValue(req.Header.Get("X-Forwarded-Proto")))
	if value == "http" || value == "https" {
		return value
	}
	value = strings.ToLower(forwardedParameter(req.Header.Get("Forwarded"), "proto"))
	if value == "http" || value == "https" {
		return value
	}
	return ""
}

func forwardedParameter(value, name string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, field := range strings.Split(value, ";") {
		key, raw, ok := strings.Cut(strings.TrimSpace(field), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), name) {
			continue
		}
		return strings.Trim(strings.TrimSpace(raw), "\"")
	}
	return ""
}

func firstForwardedValue(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}

func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n\t /\\,;") {
		return ""
	}
	parsed, err := url.Parse("//" + value)
	if err != nil || parsed.Host != value || parsed.User != nil || parsed.Hostname() == "" {
		return ""
	}
	return parsed.Host
}

func parseIP(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "\"")
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func remoteIP(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return parseIP(host)
	}
	return parseIP(value)
}

func WithOrigin(ctx context.Context, origin string) context.Context {
	return context.WithValue(ctx, originContextKey, strings.TrimRight(strings.TrimSpace(origin), "/"))
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	origin, _ := ctx.Value(originContextKey).(string)
	return strings.TrimRight(strings.TrimSpace(origin), "/")
}

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPContextKey, strings.TrimSpace(ip))
}

func ClientIPFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ip, _ := ctx.Value(clientIPContextKey).(string)
	return strings.TrimSpace(ip)
}

// ClientIPFromRequest reads the value installed by Middleware and falls back
// to the direct peer for handlers used without the top-level middleware.
func ClientIPFromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	if ip := ClientIPFromContext(req.Context()); ip != "" {
		return ip
	}
	if ip := remoteIP(req.RemoteAddr); ip != "" {
		return ip
	}
	return "unknown"
}

// Resolve returns an absolute same-origin URL when a trusted origin is
// available and otherwise preserves the relative path for non-HTTP callers.
func Resolve(ctx context.Context, path string) string {
	path = "/" + strings.TrimLeft(path, "/")
	if origin := FromContext(ctx); origin != "" {
		return origin + path
	}
	return path
}

// Middleware installs the trusted origin and client IP for all HTTP planes.
// The variadic form keeps non-runtime callers source-compatible while making
// production wiring pass the validated resolver explicitly.
func Middleware(next http.Handler, resolvers ...*Resolver) http.Handler {
	var resolver *Resolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	if resolver == nil {
		resolver, _ = NewResolver("", nil)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := WithOrigin(req.Context(), resolver.FromRequest(req))
		ctx = WithClientIP(ctx, resolver.ClientIP(req))
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}
