// Package formats provides client-protocol detection plus minimal per-protocol
// helpers (model rewrite, OAuth body sanitisation, vendor-specific request
// body modifications, usage extraction) for the strict 1:1 passthrough
// gateway. There is no longer a canonical hub or any cross-protocol
// translation here — the route filter in adapters/postgres/routes.go
// guarantees that client_protocol == upstream_protocol, and the gateway
// forwards request and response bytes verbatim.
package formats

import (
	"net/http"
	"strings"

	"xiaodou/uni-ai-api/internal/domain"
)

// DetectClientProtocol returns the upstream protocol that matches the client's
// request path. Strict 1:1 routing keys off this value: a request to
// /v1/chat/completions is treated as ProtocolOpenAIChat even if a tool tries
// to call it with a Claude-shaped body — routing will only consider
// openai_chat deployments and body-shape mismatches surface as upstream 4xx
// instead of a (now-removed) gateway-side translation.
func DetectClientProtocol(r *http.Request) domain.UpstreamProtocol {
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/v1beta/models/"):
		// Native Gemini: /v1beta/models/{model}:{action}. Embedding actions
		// end with :embedContent; everything else (generateContent /
		// streamGenerateContent) is generation.
		if strings.HasSuffix(path, ":embedContent") {
			return domain.ProtocolGeminiEmbeddings
		}
		return domain.ProtocolGeminiGenerate
	case strings.HasSuffix(path, "/messages"):
		return domain.ProtocolAnthropicMessages
	case strings.Contains(path, "/responses"):
		return domain.ProtocolOpenAIResponses
	case strings.Contains(path, "/embeddings"):
		return domain.ProtocolOpenAIEmbeddings
	case strings.Contains(path, "/images/generations"):
		return domain.ProtocolOpenAIImages
	default:
		return domain.ProtocolOpenAIChat
	}
}
