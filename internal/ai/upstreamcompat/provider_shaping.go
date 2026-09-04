package upstreamcompat

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

type RequestMeta struct {
	IsStream              bool
	ClientPath            string
	ContentType           string
	RequestID             string
	IncomingAnthropicBeta string
	SelectedCredential    *domain.OAuthCredential
	StickyIdentity        string
	OutboundUserAgent     string
}

// AnthropicAPIKeyHeaders returns the compatible authentication headers used
// for static Anthropic API-key accounts. Both headers carry the same
// credential; Authorization uses the standard Bearer scheme.
func AnthropicAPIKeyHeaders(apiKey string) map[string]string {
	return map[string]string{
		"Authorization":     "Bearer " + apiKey,
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
	}
}

func BuildURL(c *domain.RouteCandidate, meta RequestMeta) (string, error) {
	if c.FixedProviderType != "" {
		return "", fmt.Errorf(
			"upstreamcompat: fixed provider %q requires the versioned client runtime",
			c.FixedProviderType,
		)
	}

	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	path := c.RequestPath
	if path == "" {
		var err error
		path, err = DefaultPath(c.Protocol, meta.ClientPath)
		if err != nil {
			return "", err
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if u, err := url.Parse(base); err == nil {
		if basePath := strings.TrimRight(u.Path, "/"); basePath != "" {
			if trimmed, ok := strings.CutPrefix(path, basePath+"/"); ok {
				path = "/" + trimmed
			}
		}
	}
	out := base + path
	if strings.Contains(out, "{model}") {
		out = strings.ReplaceAll(out, "{model}", c.UpstreamModel)
	}
	return out, nil
}

func DefaultPath(protocol domain.UpstreamProtocol, clientPath string) (string, error) {
	switch protocol {
	case domain.ProtocolOpenAIChat:
		return "/v1/chat/completions", nil
	case domain.ProtocolOpenAIResponses:
		return "/v1/responses", nil
	case domain.ProtocolOpenAIEmbeddings:
		return "/v1/embeddings", nil
	case domain.ProtocolOpenAIImages:
		if strings.Contains(clientPath, "/images/edits") {
			return "/v1/images/edits", nil
		}
		return "/v1/images/generations", nil
	case domain.ProtocolAnthropicMessages:
		return "/v1/messages", nil
	case domain.ProtocolGeminiGenerate:
		return "/v1beta/models/{model}:generateContent", nil
	case domain.ProtocolGeminiEmbeddings:
		return "/v1beta/models/{model}:embedContent", nil
	default:
		return "", fmt.Errorf("upstreamcompat: unsupported default path protocol %q", protocol)
	}
}

func BuildHeaders(c *domain.RouteCandidate, meta RequestMeta) map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if meta.ContentType != "" {
		headers["Content-Type"] = meta.ContentType
	}
	if meta.IsStream {
		headers["Accept"] = "text/event-stream"
	}
	if shouldForwardUserAgent(c) && strings.TrimSpace(meta.OutboundUserAgent) != "" {
		headers["user-agent"] = strings.TrimSpace(meta.OutboundUserAgent)
	}

	cred := meta.SelectedCredential
	if cred != nil {
		headers["Authorization"] = "Bearer " + cred.AccessToken
	} else {
		scheme := c.EndpointAuthScheme
		if scheme == "" || scheme == domain.EndpointAuthFormatDefault {
			switch c.Protocol {
			case domain.ProtocolAnthropicMessages:
				scheme = domain.EndpointAuthAnthropicAPIKey
			case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
				scheme = domain.EndpointAuthGeminiAPIKey
			default:
				scheme = domain.EndpointAuthBearer
			}
		}
		switch scheme {
		case domain.EndpointAuthAnthropicAPIKey:
			for key, value := range AnthropicAPIKeyHeaders(c.APIKeyCiphertext) {
				headers[key] = value
			}
		case domain.EndpointAuthGeminiAPIKey:
			headers["x-goog-api-key"] = c.APIKeyCiphertext
		case domain.EndpointAuthCustomHeader:
			if c.EndpointAuthHeader != "" {
				headers[c.EndpointAuthHeader] = c.APIKeyCiphertext
			}
		default:
			headers["Authorization"] = "Bearer " + c.APIKeyCiphertext
		}
	}

	for k, v := range c.ExtraHeaders {
		headers[k] = v
	}
	return headers
}

func shouldForwardUserAgent(c *domain.RouteCandidate) bool {
	if c == nil {
		return false
	}
	switch c.FixedProviderType {
	case domain.FixedProviderCodex, domain.FixedProviderClaudeOAuth, domain.FixedProviderGeminiCLI:
		return false
	default:
		return true
	}
}

func UnwrapResponseBody(c *domain.RouteCandidate, body []byte) []byte {
	if c == nil {
		return body
	}
	if c.FixedProviderType != domain.FixedProviderGeminiCLI &&
		c.FixedProviderType != domain.FixedProviderAntigravity {
		return body
	}
	if len(body) == 0 {
		return body
	}
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Response) == 0 {
		return body
	}
	return env.Response
}

func ApplyRequestBodyTransform(c *domain.RouteCandidate, meta RequestMeta, body []byte) ([]byte, error) {
	return body, nil
}
