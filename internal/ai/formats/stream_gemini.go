package formats

import (
	"encoding/json"
	"strings"
)

// Gemini generateContent 流式两侧：Provider 把上游 streamGenerateContent SSE
// （candidates[].content.parts[]）解析成中性帧；Emitter 把中性帧重放为同款 SSE。
// 对照 Aether formats/gemini/generate_content/stream.rs 的 GeminiProviderState /
// GeminiClientEmitter。
//
// 简化（沿用 P-B 帧模型）：流式多模态（inline image/file part）不在中性帧集内，
// Provider 直接跳过这类 part（与 openai:chat / claude 流式一致），不发占位文本。

const geminiStreamDefaultID = "resp-local-stream"

// ----------------------------------------------------------------------------
// Provider（上游 Gemini SSE → 帧）
// ----------------------------------------------------------------------------

type geminiStreamToolState struct {
	callID      string
	name        string
	arguments   string
	startedEmit bool
}

type geminiStreamToolResultState struct {
	content string
	emitted bool
}

type geminiProvider struct {
	fallbackModel  string
	seenID         string
	seenModel      string
	started        bool
	finished       bool
	textParts      map[int]string
	reasoningParts map[int]string
	reasoningSigs  map[int]string
	tools          map[int]*geminiStreamToolState
	toolResults    map[int]*geminiStreamToolResultState
}

func newGeminiProvider(fallbackModel string) *geminiProvider {
	return &geminiProvider{
		fallbackModel:  fallbackModel,
		textParts:      map[int]string{},
		reasoningParts: map[int]string{},
		reasoningSigs:  map[int]string{},
		tools:          map[int]*geminiStreamToolState{},
		toolResults:    map[int]*geminiStreamToolResultState{},
	}
}

func (p *geminiProvider) identity() (string, string) {
	return resolveStreamIdentity(p.seenID, p.seenModel, p.fallbackModel, geminiStreamDefaultID)
}

func (p *geminiProvider) ensureStarted(out *[]StreamFrame) {
	if p.started {
		return
	}
	p.started = true
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvStart})
}

func (p *geminiProvider) PushLine(line []byte) ([]StreamFrame, error) {
	raw, ok := decodeSSEData(line)
	if !ok {
		return nil, nil
	}
	rawObj, ok := decodeObject(raw)
	if !ok {
		return nil, nil
	}
	if s := getStr(rawObj, "responseId"); s != "" {
		p.seenID = s
	}
	// CodeAssist 封套：真正的事件可能裹在 response 字段里（带 candidates）。
	eventObj := rawObj
	if inner, ok := decodeObject(field(rawObj, "response")); ok && objHasKey(inner, "candidates") {
		eventObj = inner
	}
	if s := getStr(eventObj, "responseId"); s != "" {
		p.seenID = s
	}
	if s := getStr(eventObj, "modelVersion"); s != "" {
		p.seenModel = s
	}

	var out []StreamFrame
	candidates, ok := decodeArray(field(eventObj, "candidates"))
	if !ok {
		id, model := p.identity()
		out = append(out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: raw})
		return out, nil
	}

	for _, c := range candidates {
		cand, ok := decodeObject(c)
		if !ok {
			continue
		}
		content, ok := decodeObject(field(cand, "content"))
		if !ok {
			p.maybeFinish(cand, eventObj, &out)
			continue
		}
		parts, ok := decodeArray(field(content, "parts"))
		if !ok {
			p.maybeFinish(cand, eventObj, &out)
			continue
		}
		if len(parts) > 0 {
			p.ensureStarted(&out)
		}
		for index, raw := range parts {
			p.handlePart(index, raw, &out)
		}
		p.maybeFinish(cand, eventObj, &out)
	}
	return out, nil
}

func (p *geminiProvider) handlePart(index int, raw json.RawMessage, out *[]StreamFrame) {
	part, ok := decodeObject(raw)
	if !ok {
		return
	}
	id, model := p.identity()

	if text, ok := geminiStreamPartText(part); ok {
		isReasoning := false
		if b, _ := asBool(field(part, "thought")); b {
			isReasoning = true
		}
		bucket := p.textParts
		if isReasoning {
			bucket = p.reasoningParts
		}
		delta := streamTextDelta(text, bucket[index])
		bucket[index] = text
		if delta != "" {
			ev := EvTextDelta
			if isReasoning {
				ev = EvReasoningDelta
			}
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: ev, Text: delta})
		}
		if isReasoning {
			sig := trimmed(strFirst(part, "thoughtSignature", "thought_signature"))
			if sig != "" && p.reasoningSigs[index] != sig {
				p.reasoningSigs[index] = sig
				*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvReasoningSignature, Text: sig})
			}
		}
		return
	}

	if fr, ok := decodeObject(fieldOr(part, "functionResponse", "function_response")); ok {
		toolUseID := trimmed(getStr(fr, "id"))
		if toolUseID == "" {
			toolUseID = generatedToolCallID(index)
		}
		name := trimmed(getStr(fr, "name"))
		content := geminiStreamFunctionResponseContent(field(fr, "response"))
		st := p.toolResults[index]
		if st == nil {
			st = &geminiStreamToolResultState{}
			p.toolResults[index] = st
		}
		delta := content
		if st.emitted {
			delta = streamTextDelta(content, st.content)
		}
		if delta != "" || !st.emitted {
			st.emitted = true
			st.content += delta
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolResultDelta,
				ToolIndex: index, ToolUseID: toolUseID, Name: name, Content: delta})
		}
		return
	}

	fc, ok := decodeObject(fieldOr(part, "functionCall", "function_call"))
	if !ok {
		// 流式多模态（inlineData/fileData）暂不入帧集，直接跳过；其余未知 part 透传。
		if objHasKey(part, "inlineData") || objHasKey(part, "inline_data") ||
			objHasKey(part, "fileData") || objHasKey(part, "file_data") {
			return
		}
		*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: raw})
		return
	}
	st := p.tools[index]
	if st == nil {
		st = &geminiStreamToolState{}
		p.tools[index] = st
	}
	if v := getStr(fc, "id"); v != "" {
		st.callID = v
	}
	if v := getStr(fc, "name"); v != "" {
		st.name = v
	}
	if !st.startedEmit {
		*out = append(*out, geminiToolStartFrame(id, model, index, st))
		st.startedEmit = true
	}
	args := canonicalizeToolArgs(field(fc, "args"))
	delta := streamTextDelta(args, st.arguments)
	st.arguments = args
	if delta != "" {
		*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
			ToolIndex: index, Arguments: delta})
	}
}

func (p *geminiProvider) maybeFinish(cand, eventObj map[string]json.RawMessage, out *[]StreamFrame) {
	reason := strFirst(cand, "finishReason", "finish_reason")
	if reason == "" {
		return
	}
	finish := normalizeOpenAIFinish(geminiStreamFinishReason(reason))
	if len(p.tools) > 0 && (finish == "" || finish == "stop") {
		finish = "tool_calls"
	}
	p.ensureStarted(out)
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvFinish,
		FinishReason: finish, HasFinish: true, Usage: geminiUsageToCanonical(field(eventObj, "usageMetadata"))})
	p.finished = true
}

func geminiToolStartFrame(id, model string, index int, st *geminiStreamToolState) StreamFrame {
	callID := st.callID
	if callID == "" {
		callID = generatedToolCallID(index)
	}
	name := st.name
	if name == "" {
		name = "unknown"
	}
	return StreamFrame{ID: id, Model: model, Event: EvToolCallStart,
		ToolIndex: index, CallID: callID, Name: name}
}

func (p *geminiProvider) Finish() ([]StreamFrame, error) {
	if !p.started || p.finished {
		return nil, nil
	}
	p.finished = true
	id, model := p.identity()
	return []StreamFrame{{ID: id, Model: model, Event: EvFinish, HasFinish: true}}, nil
}

// geminiStreamPartText 把可渲染为文本的 part 转成文本（text / executableCode /
// codeExecutionResult），对照 Aether render_gemini_part_as_text。
func geminiStreamPartText(part map[string]json.RawMessage) (string, bool) {
	if s, ok := asString(field(part, "text")); ok {
		return s, true
	}
	if code, ok := decodeObject(fieldOr(part, "executableCode", "executable_code")); ok {
		lang := getStr(code, "language")
		src := getStr(code, "code")
		return "```" + lang + "\n" + src + "\n```", true
	}
	if res, ok := decodeObject(fieldOr(part, "codeExecutionResult", "code_execution_result")); ok {
		return "```output\n" + getStr(res, "output") + "\n```", true
	}
	return "", false
}

// geminiStreamFunctionResponseContent 把 functionResponse.response 折叠成内容字符串
// （对象取 result 否则整体；其余值直出），对照 Aether gemini_function_response_content。
func geminiStreamFunctionResponseContent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if obj, ok := decodeObject(raw); ok {
		if r := field(obj, "result"); r != nil {
			return compactJSON(r)
		}
		return compactJSON(raw)
	}
	return compactJSON(raw)
}

// geminiStreamFinishReason 把 Gemini finishReason 映射为 OpenAI 口径。
func geminiStreamFinishReason(value string) string {
	switch strings.ToUpper(trimmed(value)) {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "OTHER":
		return "content_filter"
	default:
		return strings.ToLower(trimmed(value))
	}
}

// streamTextDelta 计算 full 相对 previous 的增量：若 full 以 previous 为前缀则取后缀
// （容忍累积式上游），相等则空，否则整体（增量式上游直出当前片段）。
func streamTextDelta(full, previous string) string {
	if strings.HasPrefix(full, previous) {
		return full[len(previous):]
	}
	if full == previous {
		return ""
	}
	return full
}

// ----------------------------------------------------------------------------
// Emitter（帧 → Gemini SSE）
// ----------------------------------------------------------------------------

type geminiEmitterToolState struct {
	callID    string
	name      string
	arguments string
	emitted   bool
}

type geminiEmitter struct {
	seenID    string
	seenModel string
	finished  bool
	tools     map[int]*geminiEmitterToolState
}

func newGeminiEmitter() *geminiEmitter {
	return &geminiEmitter{tools: map[int]*geminiEmitterToolState{}}
}

func (e *geminiEmitter) id() string {
	if e.seenID != "" {
		return e.seenID
	}
	return geminiStreamDefaultID
}

func (e *geminiEmitter) model() string {
	if e.seenModel != "" {
		return e.seenModel
	}
	return "unknown"
}

func (e *geminiEmitter) emitCandidate(parts []any, finishReason string, hasFinish bool, usage *Usage) []byte {
	candidate := map[string]any{
		"content": map[string]any{"role": "model", "parts": parts},
		"index":   0,
	}
	if hasFinish {
		candidate["finishReason"] = openaiFinishToGemini(finishReason)
	}
	response := map[string]any{
		"responseId":   e.id(),
		"modelVersion": e.model(),
		"candidates":   []any{candidate},
	}
	if usage != nil {
		response["usageMetadata"] = canonicalUsageToGeminiMetadata(usage)
	}
	return encodeSSE("", response)
}

func (e *geminiEmitter) flushPendingToolCalls() []byte {
	var out []byte
	for _, index := range sortedIntKeys(e.tools) {
		st := e.tools[index]
		if st.emitted {
			continue
		}
		st.emitted = true
		out = append(out, e.emitCandidate([]any{geminiToolCallPart(index, st)}, "", false, nil)...)
	}
	return out
}

func (e *geminiEmitter) Emit(frame StreamFrame) ([]byte, error) {
	if frame.ID != "" {
		e.seenID = frame.ID
	}
	if frame.Model != "" {
		e.seenModel = frame.Model
	}
	switch frame.Event {
	case EvStart:
		return nil, nil
	case EvTextDelta:
		return e.emitCandidate([]any{map[string]any{"text": frame.Text}}, "", false, nil), nil
	case EvReasoningDelta:
		return e.emitCandidate([]any{map[string]any{"text": frame.Text, "thought": true}}, "", false, nil), nil
	case EvReasoningSignature:
		return e.emitCandidate([]any{map[string]any{
			"text": "", "thought": true, "thoughtSignature": frame.Text}}, "", false, nil), nil
	case EvToolCallStart:
		st := e.tools[frame.ToolIndex]
		if st == nil {
			st = &geminiEmitterToolState{}
			e.tools[frame.ToolIndex] = st
		}
		st.callID = frame.CallID
		st.name = frame.Name
		return nil, nil
	case EvToolCallArgsDelta:
		st := e.tools[frame.ToolIndex]
		if st == nil {
			st = &geminiEmitterToolState{}
			e.tools[frame.ToolIndex] = st
		}
		st.arguments += frame.Arguments
		if st.emitted {
			return nil, nil
		}
		if parseStreamToolArgs(st.arguments) == nil {
			return nil, nil // 参数尚不完整，等后续片段或 flush
		}
		st.emitted = true
		return e.emitCandidate([]any{geminiToolCallPart(frame.ToolIndex, st)}, "", false, nil), nil
	case EvToolResultDelta:
		return e.emitCandidate([]any{geminiFunctionResponsePart(frame.ToolUseID, frame.Name, frame.Content)}, "", false, nil), nil
	case EvFinish:
		if e.finished {
			return nil, nil
		}
		out := e.flushPendingToolCalls()
		out = append(out, e.emitCandidate(nil, frame.FinishReason, true, frame.Usage)...)
		e.finished = true
		return out, nil
	default:
		return nil, nil
	}
}

func (e *geminiEmitter) Finish() ([]byte, error) {
	if e.finished {
		return nil, nil
	}
	out := e.flushPendingToolCalls()
	e.finished = true
	return out, nil
}

func geminiToolCallPart(index int, st *geminiEmitterToolState) map[string]any {
	callID := st.callID
	if callID == "" {
		callID = generatedToolCallID(index)
	}
	name := st.name
	if name == "" {
		name = "unknown"
	}
	args := parseStreamToolArgs(st.arguments)
	if args == nil {
		args = json.RawMessage("{}")
	}
	return map[string]any{"functionCall": map[string]any{"id": callID, "name": name, "args": args}}
}

func geminiFunctionResponsePart(toolUseID, name, content string) map[string]any {
	n := trimmed(name)
	if n == "" {
		n = "unknown"
	}
	return map[string]any{"functionResponse": map[string]any{
		"id": toolUseID, "name": n, "response": geminiStreamFunctionResponseValue(content)}}
}

func geminiStreamFunctionResponseValue(content string) any {
	if trimmed(content) == "" {
		return map[string]any{}
	}
	if obj, ok := decodeObject(json.RawMessage(content)); ok {
		return rawObjToAny(obj)
	}
	if json.Valid([]byte(content)) {
		return map[string]any{"output": json.RawMessage(content)}
	}
	return map[string]any{"output": content}
}

// openaiFinishToGemini 把 OpenAI finish_reason 映射为 Gemini finishReason。
func openaiFinishToGemini(value string) string {
	switch value {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	default:
		return "STOP"
	}
}

// parseStreamToolArgs 解析累积工具参数：空→{}，合法 JSON→原样，否则 nil（未完整）。
func parseStreamToolArgs(arguments string) json.RawMessage {
	t := strings.TrimSpace(arguments)
	if t == "" {
		return json.RawMessage("{}")
	}
	if json.Valid([]byte(t)) {
		return json.RawMessage(t)
	}
	return nil
}
