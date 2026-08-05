package formats

import "xiaodou/dai/internal/ai/domain"

// 转换矩阵：哪些格式对可跨协议转换 + 候选格式的偏好排序（抄 Aether matrix.rs 的
// request_candidate_api_format_preference 分桶）。embeddings/images/rerank 不在
// 生成类格式集合里，因此一律不可转换，只能同格式 passthrough。

// generationFormats 是支持 any-to-any 跨协议转换的「生成类」格式集合。
var generationFormats = map[FormatID]struct{}{
	FormatOpenAIChat:      {},
	FormatOpenAIResponses: {},
	FormatClaudeMessages:  {},
	FormatGeminiGenerate:  {},
}

// standardFormatOrder 给候选 provider 格式一个稳定优先级（数值小者优先），
// 用于偏好桶内的确定性排序。
var standardFormatOrder = []FormatID{
	FormatOpenAIChat,
	FormatOpenAIResponses,
	FormatClaudeMessages,
	FormatGeminiGenerate,
}

// formatFamilyKind 拆出格式的家族与子类型，用于偏好分桶。
func formatFamilyKind(f FormatID) (family, kind string) {
	switch f {
	case FormatOpenAIChat:
		return "openai", "chat"
	case FormatOpenAIResponses:
		return "openai", "responses"
	case FormatClaudeMessages:
		return "claude", "messages"
	case FormatGeminiGenerate:
		return "gemini", "generate"
	default:
		return "", ""
	}
}

// IsGenerationFormat 报告该格式是否属于可跨协议转换的生成类。
func IsGenerationFormat(f FormatID) bool {
	_, ok := generationFormats[f]
	return ok
}

// ConversionSupported 报告 src→dst 是否可由转换层完成（两端都须是生成类格式）。
func ConversionSupported(src, dst FormatID) bool {
	return IsGenerationFormat(src) && IsGenerationFormat(dst)
}

// CandidatePreference 返回把 provider 格式作为 client 格式的转换目标的偏好序对
// (bucket, priority)，越小越优先；ok=false 表示该 provider 格式不可作为目标。
//
//	bucket: 0 完全同格式（零转换）> 1 同子类型 > 2 同家族 > 3 跨家族
//	priority: standardFormatOrder 中的次序，仅用于桶内稳定排序
func CandidatePreference(client, provider FormatID) (bucket, priority int, ok bool) {
	if !IsGenerationFormat(client) || !IsGenerationFormat(provider) {
		return 0, 0, false
	}
	cf, ck := formatFamilyKind(client)
	pf, pk := formatFamilyKind(provider)
	switch {
	case client == provider:
		bucket = 0
	case ck == pk:
		bucket = 1
	case cf == pf:
		bucket = 2
	default:
		bucket = 3
	}
	for i, f := range standardFormatOrder {
		if f == provider {
			priority = i
			break
		}
	}
	return bucket, priority, true
}

// FormatIDForProtocol 把细粒度 UpstreamProtocol 映射到生成类 FormatID。
// 非生成类协议（embeddings/images/completions）返回 ok=false——它们不进转换层。
func FormatIDForProtocol(p domain.UpstreamProtocol) (FormatID, bool) {
	switch p {
	case domain.ProtocolOpenAIChat:
		return FormatOpenAIChat, true
	case domain.ProtocolOpenAIResponses:
		return FormatOpenAIResponses, true
	case domain.ProtocolAnthropicMessages:
		return FormatClaudeMessages, true
	case domain.ProtocolGeminiGenerate:
		return FormatGeminiGenerate, true
	default:
		return "", false
	}
}
