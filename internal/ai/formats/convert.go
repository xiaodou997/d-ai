// Package formats provides client-protocol detection plus minimal per-protocol
// helpers (model rewrite, OAuth body sanitisation, vendor-specific request
// body modifications, usage extraction) for the passthrough gateway. There is
// no canonical hub or cross-protocol body translation here — the gateway
// forwards request and response bytes verbatim.
//
// 注意（账号级路由后）：路由按账号 default_protocol 家族匹配候选（OpenAI 系的
// 5 个子协议同属 openai_compatible 家族；见 adapters/postgres/routes.go 的
// accountProtocolFamily），但候选回填的 wire 协议（c.Protocol）取 client 的真实
// 细粒度协议，家族内 1:1 透传。因此 client_protocol == upstream_protocol 对全部
// 8 个协议成立：每个 OpenAI 子协议各走自己的上游路径（/v1/responses、
// /v1/embeddings、/v1/images、/v1/completions），gemini_embeddings 走 :embedContent。
// 仍不做跨家族翻译——跨家族 shape 不符才会以上游 4xx 暴露。
package formats

import (
	"net/http"
	"strings"

	"xiaodou/dai/internal/ai/domain"
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
	case strings.Contains(path, "/images/generations"), strings.Contains(path, "/images/edits"):
		return domain.ProtocolOpenAIImages
	default:
		return domain.ProtocolOpenAIChat
	}
}
