package asynctask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

var disallowedWebhookPrefixes = []netip.Prefix{
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

var ErrUnsafeWebhookTarget = errors.New("unsafe webhook target")

// WebhookRequest is a fully materialized notification. Payload is sent
// byte-for-byte; callers must not mutate it while Send is running.
type WebhookRequest struct {
	URL     string
	Payload []byte
}

// WebhookSender is the outbound seam. Production uses the guarded HTTPS
// adapter; tests may inject a local adapter without weakening production DNS
// and address checks.
type WebhookSender interface {
	Send(context.Context, WebhookRequest) (statusCode int, err error)
}

type httpWebhookSender struct {
	client *http.Client
}

// NewWebhookSender returns the production HTTPS adapter. DNS resolution and
// dialing are one operation: only the exact, already-vetted IPs are dialed, so
// a hostname cannot pass validation and then rebind to a private address.
func NewWebhookSender() WebhookSender {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialTLSContext = nil
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = guardedWebhookDialContext(net.DefaultResolver, dialer)
	return newHTTPWebhookSender(&http.Client{Transport: transport})
}

func guardedWebhookDialContext(resolver *net.Resolver, dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split webhook address: %w", err)
		}
		var addrs []netip.Addr
		if literal, err := netip.ParseAddr(host); err == nil {
			addrs = []netip.Addr{literal}
		} else {
			addrs, err = resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve webhook host: %w", err)
			}
		}
		if len(addrs) == 0 {
			return nil, errors.New("webhook host resolved to no addresses")
		}
		for _, addr := range addrs {
			if !isPublicWebhookAddr(addr) {
				return nil, fmt.Errorf("%w: host resolved to %s", ErrUnsafeWebhookTarget, addr)
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
		return nil, fmt.Errorf("dial webhook host: %w", dialErr)
	}
}

func newHTTPWebhookSender(client *http.Client) *httpWebhookSender {
	if client == nil {
		client = &http.Client{}
	}
	cloned := *client
	cloned.Timeout = 10 * time.Second
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &httpWebhookSender{client: &cloned}
}

func (s *httpWebhookSender) Send(ctx context.Context, delivery WebhookRequest) (int, error) {
	target, err := normalizeWebhookURL(delivery.URL)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnsafeWebhookTarget, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(delivery.Payload))
	if err != nil {
		return 0, fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "UniHub-Webhook/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send webhook: %w", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func normalizeWebhookURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("webhook URL is invalid")
	}
	if !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.Hostname() == "" || u.Opaque != "" {
		return "", errors.New("webhook URL must be an absolute HTTPS URL")
	}
	if u.User != nil {
		return "", errors.New("webhook URL must not contain user information")
	}
	if u.Fragment != "" {
		return "", errors.New("webhook URL must not contain a fragment")
	}
	u.Scheme = "https"
	return u.String(), nil
}

func isPublicWebhookAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return false
	}
	for _, prefix := range disallowedWebhookPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func webhookRetryDelay(attempt int) (time.Duration, bool) {
	delays := [...]time.Duration{
		10 * time.Second,
		time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	if attempt < 1 || attempt > len(delays) {
		return 0, false
	}
	return delays[attempt-1], true
}

func (e *Engine) webhookWorkerLoop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.WebhookPollInterval)
	defer ticker.Stop()
	for {
		worked, err := e.claimAndDeliverWebhook(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			e.logger.Warn("async task webhook worker: delivery failed", zap.Error(err))
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-e.deliveryWake:
		case <-ticker.C:
		}
	}
}

func (e *Engine) claimAndDeliverWebhook(ctx context.Context) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	delivery, ok, err := e.store.claimDelivery(ctx, e.workerID, e.cfg.WebhookLeaseTTL)
	if err != nil || !ok {
		return false, err
	}
	e.deliverWebhook(ctx, delivery)
	return true, nil
}

func (e *Engine) deliverWebhook(ctx context.Context, delivery claimedDelivery) {
	statusCode, err := e.webhookSender.Send(ctx, WebhookRequest{
		URL: delivery.URL, Payload: delivery.Payload,
	})
	if ctx.Err() != nil {
		return
	}
	e.finishWebhookDelivery(ctx, delivery, classifyWebhookResult(
		delivery.Attempt, delivery.MaxAttempts, statusCode, err,
	))
}

func classifyWebhookResult(attempt, maxAttempts, statusCode int, err error) deliveryOutcome {
	if err == nil && statusCode >= 200 && statusCode < 300 {
		return deliveryOutcome{Status: "delivered", StatusCode: statusCode}
	}
	lastError := ""
	if err != nil {
		lastError = err.Error()
	} else {
		lastError = fmt.Sprintf("webhook returned HTTP %d", statusCode)
	}
	if statusCode == http.StatusGone || errors.Is(err, ErrUnsafeWebhookTarget) {
		return deliveryOutcome{
			Status: "failed", StatusCode: statusCode, LastError: lastError,
		}
	}
	delay, retry := webhookRetryDelay(attempt)
	if attempt >= maxAttempts || !retry {
		return deliveryOutcome{
			Status: "failed", StatusCode: statusCode, LastError: lastError,
		}
	}
	return deliveryOutcome{
		Status: "pending", StatusCode: statusCode, LastError: lastError, RetryAfter: delay,
	}
}

func (e *Engine) finishWebhookDelivery(ctx context.Context, delivery claimedDelivery, outcome deliveryOutcome) {
	if len(outcome.LastError) > 1024 {
		outcome.LastError = outcome.LastError[:1024]
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	written, err := e.store.finishDelivery(writeCtx, delivery.ID, e.workerID, outcome)
	if err != nil {
		e.logger.Warn("async task webhook: writing delivery result failed",
			zap.String("delivery_id", delivery.ID), zap.Error(err))
		return
	}
	if !written {
		return
	}
	if outcome.Status == "pending" {
		e.signalDelivery()
	}
}
