package clientruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	claudeformat "xiaodou/dai/internal/ai/formats/claude"
)

const (
	ClaudeProfileRevision = "claude-code@2.1.220+unihub.1"
	claudeBaseURL         = "https://api.anthropic.com"
	claudeUserAgent       = "claude-cli/2.1.220 (external, cli)"
)

var claudeRequiredBetas = []string{
	"claude-code-20250219",
	"oauth-2025-04-20",
	"interleaved-thinking-2025-05-14",
	"prompt-caching-scope-2026-01-05",
	"effort-2025-11-24",
	"context-management-2025-06-27",
	"extended-cache-ttl-2025-04-11",
}

type claudeProfileV21220 struct{}

func (claudeProfileV21220) revision() string {
	return ClaudeProfileRevision
}

func (claudeProfileV21220) supports(protocol domain.UpstreamProtocol) bool {
	return protocol == domain.ProtocolAnthropicMessages
}

func (claudeProfileV21220) prepare(in Invocation) (*WireRequest, error) {
	if in.Protocol != domain.ProtocolAnthropicMessages {
		return nil, fmt.Errorf("claude profile requires %q, got %q", domain.ProtocolAnthropicMessages, in.Protocol)
	}
	body, err := applyClaudeMessagesContract(in.Body, in.Model, in.Stream)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	headers := map[string]string{
		"Authorization":     "Bearer " + in.Credential.AccessToken,
		"Content-Type":      contentType,
		"Accept":            "application/json",
		"anthropic-version": "2023-06-01",
		"anthropic-beta":    mergeClaudeBetas(in.IncomingAnthropicBeta),
		"anthropic-dangerous-direct-browser-access": "true",
		"User-Agent":                  claudeUserAgent,
		"X-App":                       "cli",
		"X-Stainless-Lang":            "js",
		"X-Stainless-Package-Version": "0.94.0",
		"X-Stainless-OS":              "Linux",
		"X-Stainless-Arch":            "arm64",
		"X-Stainless-Runtime":         "node",
		"X-Stainless-Runtime-Version": "v24.3.0",
		"X-Stainless-Retry-Count":     "0",
		"X-Stainless-Timeout":         "600",
	}
	if in.Stream {
		headers["Accept"] = "text/event-stream"
	}
	return &WireRequest{
		Method:   http.MethodPost,
		URL:      claudeBaseURL + "/v1/messages",
		Headers:  headers,
		Body:     body,
		Protocol: domain.ProtocolAnthropicMessages,
	}, nil
}

func applyClaudeMessagesContract(body []byte, model string, stream bool) ([]byte, error) {
	sanitized, err := claudeformat.SanitizeOAuthRequestBody(body)
	if err != nil {
		return nil, fmt.Errorf("sanitize claude oauth request: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(sanitized, &doc); err != nil {
		return nil, fmt.Errorf("decode claude messages body: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("claude messages body must be an object")
	}
	if model = strings.TrimSpace(model); model != "" {
		doc["model"] = model
	}
	doc["stream"] = stream
	return json.Marshal(doc)
}

func mergeClaudeBetas(incoming string) string {
	seen := make(map[string]struct{})
	merged := make([]string, 0, len(claudeRequiredBetas)+4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsAny(value, "\r\n") {
			return
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, value)
	}
	for _, beta := range claudeRequiredBetas {
		add(beta)
	}
	for beta := range strings.SplitSeq(incoming, ",") {
		add(beta)
	}
	return strings.Join(merged, ",")
}
