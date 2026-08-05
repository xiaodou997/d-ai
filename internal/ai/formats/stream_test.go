package formats

import (
	"encoding/json"
	"strings"
	"testing"
)

// 流式两侧翻译测试：把上游 SSE 序列喂给 Provider→Emitter，收集客户端 SSE，
// 断言跨协议（claude↔openai:chat）语义保持。

// pumpStream 驱动一次完整流式转换：provider 解析每行上游 SSE，emitter 重放成客户端
// SSE，最后补 provider/emitter 收尾。
func pumpStream(t *testing.T, provider StreamProvider, emitter StreamEmitter, lines []string) []byte {
	t.Helper()
	var out []byte
	for _, l := range lines {
		frames, err := provider.PushLine([]byte(l))
		if err != nil {
			t.Fatalf("PushLine(%q): %v", l, err)
		}
		for _, f := range frames {
			b, err := emitter.Emit(f)
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			out = append(out, b...)
		}
	}
	tail, _ := provider.Finish()
	for _, f := range tail {
		b, _ := emitter.Emit(f)
		out = append(out, b...)
	}
	fin, _ := emitter.Finish()
	out = append(out, fin...)
	return out
}

// collectSSE 抽取客户端 SSE 输出里的全部 data JSON 负载（含是否出现 [DONE]）。
func collectSSE(t *testing.T, out []byte) ([]map[string]json.RawMessage, bool) {
	t.Helper()
	var datas []map[string]json.RawMessage
	done := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		d, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		d = strings.TrimSpace(d)
		if d == "[DONE]" {
			done = true
			continue
		}
		obj, ok := decodeObject(json.RawMessage(d))
		if !ok {
			t.Fatalf("bad SSE data payload: %s", d)
		}
		datas = append(datas, obj)
	}
	return datas, done
}

func TestStreamOpenAIChatToClaude(t *testing.T) {
	provider, _ := NewStreamProvider(FormatOpenAIChat, "gpt-4o")
	emitter, _ := NewStreamEmitter(FormatClaudeMessages)
	lines := []string{
		`data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		`data: [DONE]`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	var text strings.Builder
	sawMessageStart, stopReason := false, ""
	var outputTokens uint64
	for _, d := range datas {
		switch getStr(d, "type") {
		case "message_start":
			sawMessageStart = true
		case "content_block_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok && getStr(delta, "type") == "text_delta" {
				text.WriteString(getStr(delta, "text"))
			}
		case "message_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok {
				stopReason = getStr(delta, "stop_reason")
			}
			if usage, ok := decodeObject(field(d, "usage")); ok {
				outputTokens, _ = asUint(field(usage, "output_tokens"))
			}
		}
	}
	if !sawMessageStart {
		t.Error("missing message_start")
	}
	if text.String() != "Hello world" {
		t.Errorf("text = %q, want %q", text.String(), "Hello world")
	}
	if stopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", stopReason)
	}
	if outputTokens != 2 {
		t.Errorf("output_tokens = %d, want 2", outputTokens)
	}
}

func TestStreamClaudeToOpenAIChat(t *testing.T) {
	provider, _ := NewStreamProvider(FormatClaudeMessages, "claude-sonnet-4")
	emitter, _ := NewStreamEmitter(FormatOpenAIChat)
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":5,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`data: {"type":"message_stop"}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, done := collectSSE(t, out)
	if !done {
		t.Error("missing [DONE]")
	}

	var content strings.Builder
	finishReason := ""
	var promptTokens, completionTokens uint64
	for _, d := range datas {
		if choices, ok := decodeArray(field(d, "choices")); ok {
			for _, c := range choices {
				choice, _ := decodeObject(c)
				if delta, ok := decodeObject(field(choice, "delta")); ok {
					content.WriteString(getStr(delta, "content"))
				}
				if fr := getStr(choice, "finish_reason"); fr != "" {
					finishReason = fr
				}
			}
		}
		if usage, ok := decodeObject(field(d, "usage")); ok {
			promptTokens, _ = asUint(field(usage, "prompt_tokens"))
			completionTokens, _ = asUint(field(usage, "completion_tokens"))
		}
	}
	if content.String() != "Hi there" {
		t.Errorf("content = %q, want %q", content.String(), "Hi there")
	}
	if finishReason != "stop" {
		t.Errorf("finish_reason = %q, want stop", finishReason)
	}
	if promptTokens != 5 || completionTokens != 3 {
		t.Errorf("usage = %d/%d, want 5/3", promptTokens, completionTokens)
	}
}

func TestStreamToolCallOpenAIChatToClaude(t *testing.T) {
	provider, _ := NewStreamProvider(FormatOpenAIChat, "gpt-4o")
	emitter, _ := NewStreamEmitter(FormatClaudeMessages)
	lines := []string{
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"SF\"}"}}]}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	toolName, partialJSON, stopReason := "", "", ""
	for _, d := range datas {
		switch getStr(d, "type") {
		case "content_block_start":
			if cb, ok := decodeObject(field(d, "content_block")); ok && getStr(cb, "type") == "tool_use" {
				toolName = getStr(cb, "name")
			}
		case "content_block_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok && getStr(delta, "type") == "input_json_delta" {
				partialJSON += getStr(delta, "partial_json")
			}
		case "message_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok {
				stopReason = getStr(delta, "stop_reason")
			}
		}
	}
	if toolName != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", toolName)
	}
	if !strings.Contains(partialJSON, "SF") {
		t.Errorf("partial_json = %q, want to contain SF", partialJSON)
	}
	if stopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", stopReason)
	}
}

func TestStreamRegistry4x4Convertible(t *testing.T) {
	gen := []FormatID{FormatOpenAIChat, FormatOpenAIResponses, FormatClaudeMessages, FormatGeminiGenerate}
	for _, src := range gen {
		if _, err := NewStreamProvider(src, ""); err != nil {
			t.Errorf("provider %q should be implemented: %v", src, err)
		}
		if _, err := NewStreamEmitter(src); err != nil {
			t.Errorf("emitter %q should be implemented: %v", src, err)
		}
	}
	for _, src := range gen {
		for _, dst := range gen {
			if !StreamConvertible(src, dst) {
				t.Errorf("%q -> %q should be stream-convertible", src, dst)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// gemini 流式
// ----------------------------------------------------------------------------

func TestStreamGeminiToOpenAIChat(t *testing.T) {
	provider, _ := NewStreamProvider(FormatGeminiGenerate, "gemini-2.5-pro")
	emitter, _ := NewStreamEmitter(FormatOpenAIChat)
	lines := []string{
		`data: {"responseId":"r1","modelVersion":"gemini-2.5-pro","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":" world"}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, done := collectSSE(t, out)
	if !done {
		t.Error("missing [DONE]")
	}

	var content strings.Builder
	finishReason := ""
	var promptTokens, completionTokens uint64
	for _, d := range datas {
		if choices, ok := decodeArray(field(d, "choices")); ok {
			for _, c := range choices {
				choice, _ := decodeObject(c)
				if delta, ok := decodeObject(field(choice, "delta")); ok {
					content.WriteString(getStr(delta, "content"))
				}
				if fr := getStr(choice, "finish_reason"); fr != "" {
					finishReason = fr
				}
			}
		}
		if usage, ok := decodeObject(field(d, "usage")); ok {
			promptTokens, _ = asUint(field(usage, "prompt_tokens"))
			completionTokens, _ = asUint(field(usage, "completion_tokens"))
		}
	}
	if content.String() != "Hello world" {
		t.Errorf("content = %q, want %q", content.String(), "Hello world")
	}
	if finishReason != "stop" {
		t.Errorf("finish_reason = %q, want stop", finishReason)
	}
	if promptTokens != 5 || completionTokens != 2 {
		t.Errorf("usage = %d/%d, want 5/2", promptTokens, completionTokens)
	}
}

func TestStreamOpenAIChatToGemini(t *testing.T) {
	provider, _ := NewStreamProvider(FormatOpenAIChat, "gpt-4o")
	emitter, _ := NewStreamEmitter(FormatGeminiGenerate)
	lines := []string{
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	var text strings.Builder
	finishReason := ""
	var promptTokens uint64
	for _, d := range datas {
		candidates, ok := decodeArray(field(d, "candidates"))
		if !ok {
			continue
		}
		for _, c := range candidates {
			cand, _ := decodeObject(c)
			if fr := getStr(cand, "finishReason"); fr != "" {
				finishReason = fr
			}
			content, _ := decodeObject(field(cand, "content"))
			parts, _ := decodeArray(field(content, "parts"))
			for _, p := range parts {
				po, _ := decodeObject(p)
				text.WriteString(getStr(po, "text"))
			}
		}
		if usage, ok := decodeObject(field(d, "usageMetadata")); ok {
			promptTokens, _ = asUint(field(usage, "promptTokenCount"))
		}
	}
	if text.String() != "Hello world" {
		t.Errorf("text = %q, want %q", text.String(), "Hello world")
	}
	if finishReason != "STOP" {
		t.Errorf("finishReason = %q, want STOP", finishReason)
	}
	if promptTokens != 5 {
		t.Errorf("promptTokenCount = %d, want 5", promptTokens)
	}
}

func TestStreamGeminiToClaudeToolCall(t *testing.T) {
	provider, _ := NewStreamProvider(FormatGeminiGenerate, "gemini-2.5-pro")
	emitter, _ := NewStreamEmitter(FormatClaudeMessages)
	lines := []string{
		`data: {"responseId":"r1","modelVersion":"gemini-2.5-pro","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"get_weather","args":{"city":"SF"}}}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	toolName, partialJSON, stopReason := "", "", ""
	for _, d := range datas {
		switch getStr(d, "type") {
		case "content_block_start":
			if cb, ok := decodeObject(field(d, "content_block")); ok && getStr(cb, "type") == "tool_use" {
				toolName = getStr(cb, "name")
			}
		case "content_block_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok && getStr(delta, "type") == "input_json_delta" {
				partialJSON += getStr(delta, "partial_json")
			}
		case "message_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok {
				stopReason = getStr(delta, "stop_reason")
			}
		}
	}
	if toolName != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", toolName)
	}
	if !strings.Contains(partialJSON, "SF") {
		t.Errorf("partial_json = %q, want to contain SF", partialJSON)
	}
	if stopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", stopReason)
	}
}

// ----------------------------------------------------------------------------
// openai:responses 流式
// ----------------------------------------------------------------------------

func TestStreamResponsesToClaude(t *testing.T) {
	provider, _ := NewStreamProvider(FormatOpenAIResponses, "gpt-5")
	emitter, _ := NewStreamEmitter(FormatClaudeMessages)
	lines := []string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5"}}`,
		`data: {"type":"response.output_text.delta","delta":"Hello","item_id":"resp_1_msg","output_index":0}`,
		`data: {"type":"response.output_text.delta","delta":" world","item_id":"resp_1_msg","output_index":0}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5","output":[{"type":"message","id":"resp_1_msg","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	var text strings.Builder
	sawMessageStart, stopReason := false, ""
	var outputTokens uint64
	for _, d := range datas {
		switch getStr(d, "type") {
		case "message_start":
			sawMessageStart = true
		case "content_block_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok && getStr(delta, "type") == "text_delta" {
				text.WriteString(getStr(delta, "text"))
			}
		case "message_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok {
				stopReason = getStr(delta, "stop_reason")
			}
			if usage, ok := decodeObject(field(d, "usage")); ok {
				outputTokens, _ = asUint(field(usage, "output_tokens"))
			}
		}
	}
	if !sawMessageStart {
		t.Error("missing message_start")
	}
	if text.String() != "Hello world" {
		t.Errorf("text = %q, want %q", text.String(), "Hello world")
	}
	if stopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", stopReason)
	}
	if outputTokens != 2 {
		t.Errorf("output_tokens = %d, want 2", outputTokens)
	}
}

func TestStreamClaudeToResponses(t *testing.T) {
	provider, _ := NewStreamProvider(FormatClaudeMessages, "claude-sonnet-4")
	emitter, _ := NewStreamEmitter(FormatOpenAIResponses)
	lines := []string{
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":5,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`data: {"type":"message_stop"}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	var delta strings.Builder
	sawCreated, sawCompleted := false, false
	var completedText string
	var outputTokens uint64
	for _, d := range datas {
		switch getStr(d, "type") {
		case "response.created":
			sawCreated = true
		case "response.output_text.delta":
			delta.WriteString(getStr(d, "delta"))
		case "response.completed":
			sawCompleted = true
			resp, _ := decodeObject(field(d, "response"))
			if output, ok := decodeArray(field(resp, "output")); ok {
				for _, it := range output {
					item, _ := decodeObject(it)
					if getStr(item, "type") != "message" {
						continue
					}
					content, _ := decodeArray(field(item, "content"))
					for _, c := range content {
						co, _ := decodeObject(c)
						completedText += getStr(co, "text")
					}
				}
			}
			if usage, ok := decodeObject(field(resp, "usage")); ok {
				outputTokens, _ = asUint(field(usage, "output_tokens"))
			}
		}
	}
	if !sawCreated || !sawCompleted {
		t.Errorf("missing lifecycle events: created=%v completed=%v", sawCreated, sawCompleted)
	}
	if delta.String() != "Hi there" {
		t.Errorf("delta text = %q, want %q", delta.String(), "Hi there")
	}
	if completedText != "Hi there" {
		t.Errorf("completed text = %q, want %q", completedText, "Hi there")
	}
	if outputTokens != 3 {
		t.Errorf("output_tokens = %d, want 3", outputTokens)
	}
}

func TestStreamResponsesToClaudeToolCall(t *testing.T) {
	provider, _ := NewStreamProvider(FormatOpenAIResponses, "gpt-5")
	emitter, _ := NewStreamEmitter(FormatClaudeMessages)
	lines := []string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5"}}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"call_1","call_id":"call_1","name":"get_weather","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"call_1","output_index":0,"delta":"{\"city\":\"SF\"}"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5","output":[{"type":"function_call","id":"call_1","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"SF\"}"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	toolName, partialJSON, stopReason := "", "", ""
	for _, d := range datas {
		switch getStr(d, "type") {
		case "content_block_start":
			if cb, ok := decodeObject(field(d, "content_block")); ok && getStr(cb, "type") == "tool_use" {
				toolName = getStr(cb, "name")
			}
		case "content_block_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok && getStr(delta, "type") == "input_json_delta" {
				partialJSON += getStr(delta, "partial_json")
			}
		case "message_delta":
			if delta, ok := decodeObject(field(d, "delta")); ok {
				stopReason = getStr(delta, "stop_reason")
			}
		}
	}
	if toolName != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", toolName)
	}
	if !strings.Contains(partialJSON, "SF") {
		t.Errorf("partial_json = %q, want to contain SF", partialJSON)
	}
	if stopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", stopReason)
	}
}

// 跨家族冒烟：openai:chat ↔ openai:responses 文本往返。
func TestStreamOpenAIChatToResponses(t *testing.T) {
	provider, _ := NewStreamProvider(FormatOpenAIChat, "gpt-4o")
	emitter, _ := NewStreamEmitter(FormatOpenAIResponses)
	lines := []string{
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi there"}}]}`,
		`data: {"id":"c1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
	}
	out := pumpStream(t, provider, emitter, lines)
	datas, _ := collectSSE(t, out)

	var delta strings.Builder
	sawCompleted := false
	for _, d := range datas {
		switch getStr(d, "type") {
		case "response.output_text.delta":
			delta.WriteString(getStr(d, "delta"))
		case "response.completed":
			sawCompleted = true
		}
	}
	if delta.String() != "Hi there" {
		t.Errorf("delta text = %q, want %q", delta.String(), "Hi there")
	}
	if !sawCompleted {
		t.Error("missing response.completed")
	}
}
