package formats

import "fmt"

// 流式转换调度入口：按 FormatID 取对应的 Provider（上游 SSE→帧）与 Emitter
// （帧→客户端 SSE）。跨协议流式转换 = NewStreamProvider(provider 格式) 串联
// NewStreamEmitter(client 格式)。生成类 4 格式（openai:chat / openai:responses /
// claude:messages / gemini:generate）均已实现，流式与非流式一样 4×4 全互转。
//
// fallbackModel 是身份解析的兜底模型名（通常为上游真实/逻辑模型），当上游 SSE
// 自身未带 model 时用它填充帧。

// NewStreamProvider 返回某格式的流式 Provider。
func NewStreamProvider(format FormatID, fallbackModel string) (StreamProvider, error) {
	switch format {
	case FormatOpenAIChat:
		return newOpenAIChatProvider(fallbackModel), nil
	case FormatOpenAIResponses:
		return newResponsesProvider(fallbackModel), nil
	case FormatClaudeMessages:
		return newClaudeProvider(fallbackModel), nil
	case FormatGeminiGenerate:
		return newGeminiProvider(fallbackModel), nil
	default:
		return nil, fmt.Errorf("formats: streaming provider not implemented for %q", format)
	}
}

// NewStreamEmitter 返回某格式的流式 Emitter。
func NewStreamEmitter(format FormatID) (StreamEmitter, error) {
	switch format {
	case FormatOpenAIChat:
		return newOpenAIChatEmitter(), nil
	case FormatOpenAIResponses:
		return newResponsesEmitter(), nil
	case FormatClaudeMessages:
		return newClaudeEmitter(), nil
	case FormatGeminiGenerate:
		return newGeminiEmitter(), nil
	default:
		return nil, fmt.Errorf("formats: streaming emitter not implemented for %q", format)
	}
}

// StreamConvertible 报告 src→dst 是否支持流式转换（两端都须有 Provider/Emitter）。
func StreamConvertible(src, dst FormatID) bool {
	_, perr := NewStreamProvider(src, "")
	_, eerr := NewStreamEmitter(dst)
	return perr == nil && eerr == nil
}
