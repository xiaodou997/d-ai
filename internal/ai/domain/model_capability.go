package domain

import "strings"

// 模型能力命名识别 —— 单一事实源。
//
// 这里只按模型名给出能力建议，用于导入发现与创建 binding 时的默认值；
// capability_type 在 DB 上是显式列，最终以显式值为准。API 格式不从模型名推断。

// openAIImageModelFragments 是 OpenAI 系生图模型命名片段。
var openAIImageModelFragments = []string{
	"dall-e-",
	"gpt-image-",
	"stable-diffusion",
	"sdxl",
	"imagen-",
	"image-generation",
}

// geminiImageModelAliases 是不含 gemini/gemma 家族前缀、但确为 Gemini 原生生图的别名。
var geminiImageModelAliases = []string{
	"nano-banana",
}

// embeddingModelFragments / audioSTT / audioTTS / rerank 命名片段。
var (
	embeddingModelFragments = []string{"text-embedding-", "embedding-", "-embed", "embed-"}
	audioSTTModelFragments  = []string{"whisper-", "-transcription", "-asr"}
	audioTTSModelFragments  = []string{"tts-", "-tts", "text-to-speech"}
	rerankModelFragments    = []string{"rerank", "re-rank"}
)

// IsGeminiImageModel 判断模型名是否为 Gemini 原生生图模型。
// 规则：gemini/gemma 家族且模型名含 "-image"（gemini-3.1-flash-image 等），
// 或命中显式别名（nano-banana）。注意 gpt-image- 等 OpenAI 系不在此列。
func IsGeminiImageModel(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if containsAnyFragment(lower, geminiImageModelAliases...) {
		return true
	}
	if strings.HasPrefix(lower, "gemini-") || strings.HasPrefix(lower, "gemma-") {
		return strings.Contains(lower, "-image")
	}
	return false
}

// IsOpenAIImageModel 判断模型名是否为 OpenAI 系生图模型。
func IsOpenAIImageModel(modelID string) bool {
	return containsAnyFragment(strings.ToLower(strings.TrimSpace(modelID)), openAIImageModelFragments...)
}

// InferModelCapability 仅按模型名推断能力类型（不含协议）。用于 binding 默认能力。
func InferModelCapability(modelID string) CapabilityType {
	return inferCapabilityFromName(modelID)
}

// ProtocolSupportsCapability reports whether an exact upstream wire protocol
// can serve a model capability without relying on a different endpoint format.
// Model bindings remain account/pool scoped; this function only validates that
// an active target has at least one usable request endpoint for the binding.
func ProtocolSupportsCapability(protocol UpstreamProtocol, capability CapabilityType) bool {
	switch capability {
	case CapabilityEmbedding:
		return protocol == ProtocolOpenAIEmbeddings || protocol == ProtocolGeminiEmbeddings
	case CapabilityImage:
		return protocol == ProtocolOpenAIImages || protocol == ProtocolGeminiGenerate
	case CapabilityChat:
		return protocol == ProtocolOpenAIChat ||
			protocol == ProtocolOpenAIResponses ||
			protocol == ProtocolAnthropicMessages ||
			protocol == ProtocolGeminiGenerate
	default:
		// The gateway currently carries video/audio/rerank over provider-specific
		// payloads without a dedicated wire-protocol enum. Keep those capabilities
		// eligible whenever the target has an active endpoint.
		return protocol != ""
	}
}

func AnyProtocolSupportsCapability(protocols []UpstreamProtocol, capability CapabilityType) bool {
	for _, protocol := range protocols {
		if ProtocolSupportsCapability(protocol, capability) {
			return true
		}
	}
	return false
}

// inferCapabilityFromName 仅按模型名判定能力类型，不涉及协议选择。
func inferCapabilityFromName(modelID string) CapabilityType {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case containsAnyFragment(lower, embeddingModelFragments...):
		return CapabilityEmbedding
	case IsGeminiImageModel(lower):
		return CapabilityImage
	case containsAnyFragment(lower, openAIImageModelFragments...):
		return CapabilityImage
	case containsAnyFragment(lower, audioSTTModelFragments...):
		return CapabilityAudioSTT
	case containsAnyFragment(lower, audioTTSModelFragments...):
		return CapabilityAudioTTS
	case containsAnyFragment(lower, rerankModelFragments...):
		return CapabilityRerank
	default:
		return CapabilityChat
	}
}

func containsAnyFragment(s string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(s, fragment) {
			return true
		}
	}
	return false
}
