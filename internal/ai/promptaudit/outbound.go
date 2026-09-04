package promptaudit

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var disallowedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("fec0::/10"),
}

func NormalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.Hostname() == "" || u.Opaque != "" {
		return "", errors.New("prompt audit base_url must be an absolute HTTPS URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("prompt audit base_url must not contain credentials, query or fragment")
	}
	u.Scheme = "https"
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "/v1" {
		u.Path = ""
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func ChatCompletionsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/chat/completions", nil
}

func NewSecureHTTPClient(endpoint Endpoint) (*http.Client, error) {
	if _, err := NormalizeBaseURL(endpoint.BaseURL); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: time.Duration(endpoint.TimeoutMS) * time.Millisecond, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	transport.DialContext = guardedDialContext(net.DefaultResolver, dialer)
	timeout := time.Duration(endpoint.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func guardedDialContext(resolver *net.Resolver, dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		var addrs []netip.Addr
		if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
			addrs = []netip.Addr{literal}
		} else {
			addrs, err = resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		if len(addrs) == 0 {
			return nil, errors.New("prompt audit host resolved to no addresses")
		}
		for _, addr := range addrs {
			if !isPublicAddr(addr) {
				return nil, fmt.Errorf("unsafe prompt audit target: %s", addr)
			}
		}
		var dialErr error
		for _, addr := range addrs {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = errors.Join(dialErr, err)
		}
		return nil, dialErr
	}
}

func isPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range disallowedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
