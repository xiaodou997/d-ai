// Package formats provides client-protocol detection plus minimal per-protocol
// helpers (model rewrite, OAuth body sanitisation, vendor-specific request
// body modifications, usage extraction) for the passthrough gateway. There is
// no canonical hub or cross-protocol body translation here — the gateway
// forwards request and response bytes verbatim.
//
// 账号端点声明精确 wire 格式，零转换候选直接透传；开启协议转换的分组才会进入
// bridge runtime。每个 OpenAI 子协议走自己的默认路径，gemini_embeddings 走
// :embedContent；自定义路径由端点的 path_override 覆盖。
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
