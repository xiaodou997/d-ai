// Package transport implements the pure HTTP communication layer for upstream
// AI provider calls. It has zero business logic — no billing, no routing, no
// format conversion. It just makes HTTP requests and returns raw responses.
package transport

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/serving"
)

// Client implements serving.Transporter.
type Client struct {
	http *http.Client
}

// NewClient creates a transport client.
//
// headerTimeout bounds the upstream response-header wait (defaults to 120s).
// We deliberately do NOT set http.Client.Timeout because that is an absolute
// deadline covering the entire response body read — fatal for SSE streams,
// which can legitimately last minutes. The per-attempt deadline is carried by
// the caller-supplied context (ExecuteStep.runAttempt wraps it with
// PerAttemptTimeout); Do does not add — and must not prematurely cancel — its
// own context.
func NewClient(headerTimeout time.Duration) *Client {
	if headerTimeout <= 0 {
		headerTimeout = 120 * time.Second
	}
	return &Client{
		http: &http.Client{
			// No Timeout — see comment above.
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: headerTimeout,
			},
		},
	}
}

// Do executes an upstream HTTP request and returns the response.
// The caller is responsible for closing resp.Body.
//
// The request runs under the caller-supplied ctx, which already carries the
// per-attempt deadline. Do must NOT wrap it in its own context.WithTimeout:
// the resulting cancel func would have to fire either via defer (the instant
// Do returns — before the streaming body is read) or never (a leak). Either
// way, cancelling the request context makes every resp.Body.Read fail with
// context.Canceled, truncating SSE streams to whatever the transport had
// already buffered (~4 KiB, typically the first 1-2 events).
func (c *Client) Do(ctx context.Context, req *serving.UpstreamRequest) (*serving.UpstreamResponse, error) {
	url := resolveURL(req)

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for k, v := range req.Headers {
		// Synthetic header: carries the Gemini API key from execute.buildHeaders
		// to resolveURL above (where it becomes ?key=...). Never send to wire.
		if k == "x-gemini-api-key" {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http do %s %s: %w", req.Method, url, err)
	}

	return &serving.UpstreamResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       resp.Body,
	}, nil
}

// resolveURL handles protocol-specific URL construction.
// Gemini uses a query-param API key instead of a Bearer header.
//
// Note: this function does NOT mutate req.Headers. The "x-gemini-api-key" entry
// is left in place because Do() copies headers into the underlying http.Request
// via http.Header.Set, and we explicitly skip propagating that synthetic key
// to the wire below.
func resolveURL(req *serving.UpstreamRequest) string {
	url := req.URL

	// Gemini: replace {model} placeholder and append API key as query param
	if req.Protocol == domain.ProtocolGeminiGenerate || req.Protocol == domain.ProtocolGeminiEmbeddings {
		apiKey := req.Headers["x-gemini-api-key"]
		if apiKey != "" {
			if strings.Contains(url, "?") {
				url += "&key=" + apiKey
			} else {
				url += "?key=" + apiKey
			}
		}
		// Streaming variant
		if req.Headers["Accept"] == "text/event-stream" {
			url = strings.Replace(url, ":generateContent", ":streamGenerateContent", 1)
			if !strings.Contains(url, "alt=sse") {
				if strings.Contains(url, "?") {
					url += "&alt=sse"
				} else {
					url += "?alt=sse"
				}
			}
		}
	}

	return url
}
