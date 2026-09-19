package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
)

type lookupNetIPFunc func(context.Context, string, string) ([]netip.Addr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

var disallowedUpstreamPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

func newUpstreamHTTPClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: transport,
		// Provider endpoints are expected to be final API targets. Following a
		// redirect would create a second destination that bypasses the address
		// pinned for this attempt.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c *Client) resolvePublicTarget(ctx context.Context, host string) ([]netip.Addr, error) {
	if host == "" {
		return nil, errors.New("upstream destination host is required")
	}
	allowed := c.allowAddress
	if allowed == nil {
		allowed = isPublicUpstreamAddr
	}
	var addrs []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{literal}
	} else {
		lookup := c.lookupNetIP
		if lookup == nil {
			lookup = net.DefaultResolver.LookupNetIP
		}
		resolved, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve upstream destination %q: %w", host, err)
		}
		addrs = resolved
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("upstream destination %q resolved to no addresses", host)
	}
	vetted := make([]netip.Addr, 0, len(addrs))
	for _, addr := range addrs {
		addr = addr.Unmap()
		if !allowed(addr) {
			return nil, fmt.Errorf("upstream destination %q resolved to a disallowed address: %s", host, addr)
		}
		vetted = append(vetted, addr)
	}
	return vetted, nil
}

func (c *Client) dialPublicTarget(ctx context.Context, network, address string, dial dialContextFunc) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse upstream destination %q: %w", address, err)
	}
	addrs, err := c.resolvePublicTarget(ctx, host)
	if err != nil {
		return nil, err
	}
	if dial == nil {
		dial = c.directDialContext
	}
	if dial == nil {
		dialer := &net.Dialer{}
		dial = dialer.DialContext
	}
	var dialErr error
	for _, addr := range addrs {
		conn, err := dial(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		dialErr = errors.Join(dialErr, err)
	}
	if dialErr == nil {
		dialErr = errors.New("no vetted upstream address could be dialed")
	}
	return nil, dialErr
}

func (c *Client) pinRequestForProxy(req *http.Request) (*http.Request, string, error) {
	if req == nil || req.URL == nil {
		return nil, "", errors.New("upstream request URL is required")
	}
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		switch req.URL.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return nil, "", fmt.Errorf("unsupported upstream URL scheme %q", req.URL.Scheme)
		}
	}
	addrs, err := c.resolvePublicTarget(req.Context(), host)
	if err != nil {
		return nil, "", err
	}
	pinned := req.Clone(req.Context())
	urlCopy := *req.URL
	urlCopy.Host = net.JoinHostPort(addrs[0].String(), port)
	pinned.URL = &urlCopy
	pinned.Host = req.Host
	if pinned.Host == "" {
		pinned.Host = req.URL.Host
	}
	return pinned, host, nil
}

func isPublicUpstreamAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() ||
		addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range disallowedUpstreamPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func tlsConfigForServer(base *tls.Config, serverName string) *tls.Config {
	if base == nil {
		return &tls.Config{ServerName: serverName}
	}
	cfg := base.Clone()
	cfg.ServerName = serverName
	return cfg
}

func dialTLSFirstHop(
	ctx context.Context,
	network, address, serverName string,
	dial dialContextFunc,
	baseTLS *tls.Config,
) (net.Conn, error) {
	if dial == nil {
		dialer := &net.Dialer{}
		dial = dialer.DialContext
	}
	raw, err := dial(ctx, network, address)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(raw, tlsConfigForServer(baseTLS, serverName))
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return tlsConn, nil
}
