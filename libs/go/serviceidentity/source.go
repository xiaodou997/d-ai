package serviceidentity

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

var DefaultInternalCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1/128",
	"fc00::/7",
}

type SourceResolver struct {
	trusted []netip.Prefix
}

func NewSourceResolver(trustedProxyCIDRs []string) (*SourceResolver, error) {
	prefixes, err := ParseCIDRs(trustedProxyCIDRs)
	if err != nil {
		return nil, fmt.Errorf("trusted proxy CIDRs: %w", err)
	}
	return &SourceResolver{trusted: prefixes}, nil
}

func ParseCIDRs(values []string) ([]netip.Prefix, error) {
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			if addr, addrErr := netip.ParseAddr(value); addrErr == nil {
				prefix = ExactPrefix(addr)
			} else {
				return nil, fmt.Errorf("invalid CIDR %q", value)
			}
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func (r *SourceResolver) Resolve(req *http.Request) (netip.Addr, error) {
	peer, err := parseRemoteAddr(req.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	if !containsAddr(r.trusted, peer) {
		return peer, nil
	}

	chain, chainErr := forwardedChain(req.Header.Get("X-Forwarded-For"))
	if chainErr != nil {
		return netip.Addr{}, chainErr
	}
	if len(chain) == 0 {
		if raw := strings.TrimSpace(req.Header.Get("X-Real-IP")); raw != "" {
			addr, parseErr := netip.ParseAddr(raw)
			if parseErr != nil {
				return netip.Addr{}, fmt.Errorf("invalid X-Real-IP")
			}
			return addr.Unmap(), nil
		}
		return peer, nil
	}

	// Walk from the directly connected proxy toward the original caller and
	// discard only hops which are explicitly trusted.
	for i := len(chain) - 1; i >= 0; i-- {
		if !containsAddr(r.trusted, chain[i]) {
			return chain[i], nil
		}
	}
	return chain[0], nil
}

func forwardedChain(raw string) ([]netip.Addr, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	result := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		addr, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid X-Forwarded-For")
		}
		result = append(result, addr.Unmap())
	}
	return result, nil
}

func parseRemoteAddr(raw string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		host = raw
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote address %q", raw)
	}
	return addr.Unmap(), nil
}

func ExactPrefix(addr netip.Addr) netip.Prefix {
	addr = addr.Unmap()
	if addr.Is4() {
		return netip.PrefixFrom(addr, 32)
	}
	return netip.PrefixFrom(addr, 128)
}

func Contains(prefix string, addr netip.Addr) bool {
	p, err := netip.ParsePrefix(prefix)
	return err == nil && p.Contains(addr.Unmap())
}

func containsAddr(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}

func IsInternal(addr netip.Addr, prefixes []netip.Prefix) bool {
	return containsAddr(prefixes, addr)
}
