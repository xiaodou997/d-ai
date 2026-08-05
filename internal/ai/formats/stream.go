package formats

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// ============================================================================
// 流式协议转换（P-B）：两侧式翻译
// ============================================================================
//
// 抄 Aether 的流式设计：上游 SSE 行 → Provider.PushLine → []StreamFrame（中性帧）
// → Emitter.Emit → 客户端 SSE 字节。Provider 负责把某格式的上游 SSE 解析成中性帧，
// Emitter 负责把中性帧重放成另一格式的客户端 SSE。两者由 StreamFrame 桥接，于是
// 跨协议流式转换 = 源格式 Provider + 目标格式 Emitter（同格式应走 passthrough，
// 不经此层）。
//
// P-B 首批落地 openai:chat 与 claude:messages 两侧；流式 registry 见
// stream_registry.go。ContentPart（流式多模态块）暂不在帧集合内（极罕见，留后续）。

// StreamEvent 标记中性流帧的事件类型。
type StreamEvent int

const (
	EvStart              StreamEvent = iota // 流开始
	EvTextDelta                             // 正文文本增量
	EvReasoningDelta                        // 思考文本增量
	EvReasoningSignature                    // 思考签名
	EvToolCallStart                         // 工具调用开始（含 index/call_id/name）
	EvToolCallArgsDelta                     // 工具调用参数增量（JSON 片段）
	EvToolResultDelta                       // 工具结果增量
	EvFinish                                // 流结束（含 finish_reason/usage）
	EvUnknown                               // 无法识别的上游帧（透传或丢弃）
)

// StreamFrame 是格式无关的流式中性帧。
type StreamFrame struct {
	ID    string
	Model string
	Event StreamEvent

	// text / reasoning / signature
	Text string
	// tool call
	ToolIndex int
	CallID    string
	Name      string
	Arguments string
	// tool result
	ToolUseID string
	Content   string
	// finish — FinishReason 用 OpenAI 口径（stop/length/tool_calls/content_filter）
	FinishReason string
	HasFinish    bool
	Usage        *Usage
	// unknown
	Unknown json.RawMessage
}

// StreamProvider 把某格式的上游 SSE 解析为中性帧。
type StreamProvider interface {
	// PushLine 消费一行上游 SSE（原始字节，含/不含 "data:" 前缀均可），产出零或多帧。
	PushLine(line []byte) ([]StreamFrame, error)
	// Finish 在上游流自然结束（无显式 finish 帧）时补一个收尾帧。
	Finish() ([]StreamFrame, error)
}

// StreamEmitter 把中性帧重放为某格式的客户端 SSE 字节。
type StreamEmitter interface {
	Emit(frame StreamFrame) ([]byte, error)
	// Finish 在转换结束时补齐收尾 SSE（如未显式收尾）。
	Finish() ([]byte, error)
}

// ----------------------------------------------------------------------------
// SSE 编解码
// ----------------------------------------------------------------------------

// decodeSSEData 解析一行 SSE 的 data 负载：跳过空行/注释行/event 行，剥离 "data:"，
// 跳过 [DONE]，返回内层 JSON。非数据行返回 ok=false。
func decodeSSEData(line []byte) (json.RawMessage, bool) {
	text := strings.TrimSpace(strings.Trim(string(line), "\r"))
	if text == "" || strings.HasPrefix(text, ":") || strings.HasPrefix(text, "event:") {
		return nil, false
	}
	data, ok := strings.CutPrefix(text, "data:")
	if !ok {
		return nil, false
	}
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return nil, false
	}
	if !json.Valid([]byte(data)) {
		return nil, false
	}
	return json.RawMessage(data), true
}

// encodeSSE 编码一帧 SSE：event 非空则带 "event:" 行，data 为紧凑 JSON。
func encodeSSE(event string, v any) []byte {
	var b bytes.Buffer
	if strings.TrimSpace(event) != "" {
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteByte('\n')
	}
	b.WriteString("data: ")
	b.Write(mustRaw(v))
	b.WriteString("\n\n")
	return b.Bytes()
}

// encodeDoneSSE 是 OpenAI 风格的流终止标记。
func encodeDoneSSE() []byte { return []byte("data: [DONE]\n\n") }

// ----------------------------------------------------------------------------
// 共享：finish reason / usage / 身份解析
// ----------------------------------------------------------------------------

// resolveStreamIdentity 解析帧身份：优先用流内观测到的 id/model，回退到默认值。
func resolveStreamIdentity(seenID, seenModel, fallbackModel, defaultID string) (string, string) {
	id := seenID
	if id == "" {
		id = defaultID
	}
	model := seenModel
	if model == "" {
		model = fallbackModel
	}
	if model == "" {
		model = "unknown"
	}
	return id, model
}

// normalizeOpenAIFinish 规范化 OpenAI finish_reason（function_call→tool_calls，空→""）。
func normalizeOpenAIFinish(value string) string {
	switch value {
	case "function_call":
		return "tool_calls"
	default:
		return strings.TrimSpace(value)
	}
}

// claudeStopToOpenAIFinish 把 Claude stop_reason 映射为 OpenAI finish 口径。
func claudeStopToOpenAIFinish(stop string, hasToolCalls bool) string {
	var mapped string
	switch stop {
	case "end_turn", "stop_sequence", "pause_turn":
		mapped = "stop"
	case "max_tokens":
		mapped = "length"
	case "tool_use":
		mapped = "tool_calls"
	default:
		mapped = ""
	}
	if hasToolCalls && (mapped == "" || mapped == "stop") {
		return "tool_calls"
	}
	return mapped
}

// openaiFinishToClaudeStop 把 OpenAI finish_reason 映射为 Claude stop_reason。
func openaiFinishToClaudeStop(value string) string {
	switch value {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "content_filter":
		return "content_filtered"
	default:
		return "end_turn"
	}
}

// generatedToolCallID 为缺失 id 的工具调用生成稳定占位 id。
func generatedToolCallID(index int) string { return "call_" + strconv.Itoa(index) }

// ----------------------------------------------------------------------------
// OpenAI chat chunk 构造（emit 侧共用）
// ----------------------------------------------------------------------------

func openaiChatRoleChunk(id, model string) map[string]any {
	return map[string]any{
		"id": id, "object": "chat.completion.chunk", "model": model,
		"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
	}
}

func openaiChatTextChunk(id, model, text string) map[string]any {
	return map[string]any{
		"id": id, "object": "chat.completion.chunk", "model": model,
		"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": text}, "finish_reason": nil}},
	}
}

func openaiChatFinishChunk(id, model, finishReason string) map[string]any {
	var fr any
	if finishReason != "" {
		fr = finishReason
	}
	return map[string]any{
		"id": id, "object": "chat.completion.chunk", "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": fr}},
	}
}

func openaiChatUsageChunk(id, model string, u *Usage) map[string]any {
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens
	}
	usage := map[string]any{
		"prompt_tokens": u.InputTokens, "completion_tokens": u.OutputTokens, "total_tokens": total}
	if u.ReasoningTokens > 0 {
		usage["completion_tokens_details"] = map[string]any{"reasoning_tokens": u.ReasoningTokens}
	}
	return map[string]any{
		"id": id, "object": "chat.completion.chunk", "model": model,
		"choices": []any{}, "usage": usage,
	}
}
