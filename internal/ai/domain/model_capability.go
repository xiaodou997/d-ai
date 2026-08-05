package domain

import "strings"

// 模型能力命名识别 —— 单一事实源。
//
// 这里只做"按模型名给出默认能力/协议建议"，用于导入发现与创建 binding 时的
// 默认值；capability_type 在 DB 上是显式列，最终以显式值为准，命名识别仅兜底。
//
// 关键点：Gemini 原生生图走 generateContent（gemini_generate 格式），配合
// responseModalities:["TEXT","IMAGE"]，因此 Gemini 生图模型的协议是
// ProtocolGeminiGenerate 而非独立端点；OpenAI 系生图走 openai_images。

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

// InferModelCapabilityAndProtocol 按模型名（结合上游端点协议家族）推断默认能力与上游协议。
// endpointProtocol 取 EndpointProtocol 值（openai_compatible/anthropic/gemini），可为空。
//
// 判定顺序：embedding → image → audio → rerank → 家族默认 chat。
// 生图判定必须早于通用 gemini-/claude-/默认 chat 判定，否则生图模型会被误判为 chat。
func InferModelCapabilityAndProtocol(modelID, endpointProtocol string) (CapabilityType, UpstreamProtocol) {
	capability := inferCapabilityFromName(modelID)
	return capability, DefaultProtocolForCapability(capability, modelID, endpointProtocol)
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

// DefaultProtocolForCapability 在已知能力的前提下，按账号协议家族 + 模型名给出默认协议。
// modelID 仅用于 image 能力下细分"是否走 Gemini 原生生图"、chat 能力下细分 claude-/gemini- 家族，可传空串（此时按通用默认值处理）。
// 供外部能力数据源（如 external models 目录）命中能力但仍需要选协议时复用，避免重复维护一份协议选择规则。
func DefaultProtocolForCapability(capability CapabilityType, modelID, endpointProtocol string) UpstreamProtocol {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	switch capability {
	case CapabilityEmbedding:
		if endpointProtocol == string(EndpointProtocolGemini) {
			return ProtocolGeminiEmbeddings
		}
		return ProtocolOpenAIEmbeddings
	case CapabilityImage:
		if IsGeminiImageModel(lower) {
			// Gemini 原生生图：generateContent + responseModalities。
			return ProtocolGeminiGenerate
		}
		return ProtocolOpenAIImages
	case CapabilityChat:
		if strings.HasPrefix(lower, "claude-") {
			if endpointProtocol == string(EndpointProtocolAnthropic) {
				return ProtocolAnthropicMessages
			}
			return ProtocolOpenAIResponses
		}
		if strings.HasPrefix(lower, "gemini-") || strings.HasPrefix(lower, "gemma-") {
			return ProtocolGeminiGenerate
		}
		return ProtocolOpenAIResponses
	default:
		// video/audio_tts/audio_stt/rerank 暂无专属协议，占位使用 openai_chat。
		return ProtocolOpenAIChat
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
