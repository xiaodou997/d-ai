package formats

import "encoding/json"

// OpenAI Chat 流式两侧：Provider 把上游 chat.completion.chunk SSE 解析成中性帧；
// Emitter 把中性帧重放为 chat.completion.chunk SSE。对照 Aether
// formats/openai/chat/stream.rs 的 OpenAIChatProviderState / OpenAIChatClientEmitter。

const openaiStreamDefaultID = "chatcmpl-local-stream"

// ----------------------------------------------------------------------------
// Provider（上游 SSE → 帧）
// ----------------------------------------------------------------------------

type openaiChatToolState struct {
	id            string
	name          string
	startedEmit   bool
	hasID, hasNam bool
}

type openaiChatProvider struct {
	fallbackModel string
	seenID        string
	seenModel     string
	started       bool
	finished      bool
	pendingFinish string
	tools         map[int]*openaiChatToolState
}

func newOpenAIChatProvider(fallbackModel string) *openaiChatProvider {
	return &openaiChatProvider{fallbackModel: fallbackModel, tools: map[int]*openaiChatToolState{}}
}

func (p *openaiChatProvider) identity() (string, string) {
	return resolveStreamIdentity(p.seenID, p.seenModel, p.fallbackModel, openaiStreamDefaultID)
}

func (p *openaiChatProvider) ensureStarted(out *[]StreamFrame) {
	if p.started {
		return
	}
	p.started = true
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvStart})
}

func (p *openaiChatProvider) PushLine(line []byte) ([]StreamFrame, error) {
	raw, ok := decodeSSEData(line)
	if !ok {
		return nil, nil
	}
	obj, ok := decodeObject(raw)
	if !ok {
		return nil, nil
	}
	if s := getStr(obj, "id"); s != "" {
		p.seenID = s
	}
	if s := getStr(obj, "model"); s != "" {
		p.seenModel = s
	}

	var out []StreamFrame
	choices, hasChoices := decodeArray(field(obj, "choices"))
	if !hasChoices || len(choices) == 0 {
		if u := streamUsageFromOpenAI(obj); u != nil {
			p.ensureStarted(&out)
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvFinish,
				FinishReason: p.pendingFinish, HasFinish: true, Usage: u})
			p.pendingFinish = ""
			p.finished = true
		} else if !hasChoices && objHasKey(obj, "choices") {
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: raw})
		}
		return out, nil
	}

	for _, c := range choices {
		choice, ok := decodeObject(c)
		if !ok {
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: c})
			continue
		}
		delta, hasDelta := decodeObject(field(choice, "delta"))
		finish := normalizeOpenAIFinish(getStr(choice, "finish_reason"))
		if !hasDelta {
			if finish != "" {
				p.emitFinish(finish, obj, &out)
			}
			continue
		}
		if text, ok := asString(field(delta, "content")); ok && text != "" {
			p.ensureStarted(&out)
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvTextDelta, Text: text})
		}
		if rc, ok := asString(field(delta, "reasoning_content")); ok && rc != "" {
			p.ensureStarted(&out)
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvReasoningDelta, Text: rc})
		}
		if tcs, ok := decodeArray(field(delta, "tool_calls")); ok {
			p.ensureStarted(&out)
			p.handleToolCalls(tcs, &out)
		}
		if finish != "" {
			p.emitFinish(finish, obj, &out)
		}
	}
	return out, nil
}

func (p *openaiChatProvider) handleToolCalls(tcs []json.RawMessage, out *[]StreamFrame) {
	id, model := p.identity()
	for _, tc := range tcs {
		tco, ok := decodeObject(tc)
		if !ok {
			continue
		}
		index := 0
		if v, ok := asUint(field(tco, "index")); ok {
			index = int(v)
		}
		st := p.tools[index]
		if st == nil {
			st = &openaiChatToolState{}
			p.tools[index] = st
		}
		if cid, ok := asString(field(tco, "id")); ok {
			st.id, st.hasID = cid, true
		}
		fn, _ := decodeObject(field(tco, "function"))
		if fn != nil {
			if name, ok := asString(field(fn, "name")); ok {
				st.name, st.hasNam = name, true
			}
		}
		if !st.startedEmit && (st.hasID || st.hasNam) {
			out = appendToolStart(out, id, model, index, st)
			st.startedEmit = true
		}
		if fn != nil {
			if args, ok := asString(field(fn, "arguments")); ok && args != "" {
				if !st.startedEmit {
					out = appendToolStart(out, id, model, index, st)
					st.startedEmit = true
				}
				*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
					ToolIndex: index, Arguments: args})
			}
		}
	}
}

func appendToolStart(out *[]StreamFrame, id, model string, index int, st *openaiChatToolState) *[]StreamFrame {
	callID := st.id
	if callID == "" {
		callID = generatedToolCallID(index)
	}
	name := st.name
	if name == "" {
		name = "unknown"
	}
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallStart,
		ToolIndex: index, CallID: callID, Name: name})
	return out
}

func (p *openaiChatProvider) emitFinish(finish string, obj map[string]json.RawMessage, out *[]StreamFrame) {
	if u := streamUsageFromOpenAI(obj); u != nil {
		p.ensureStarted(out)
		id, model := p.identity()
		*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvFinish,
			FinishReason: finish, HasFinish: true, Usage: u})
		p.finished = true
	} else {
		p.pendingFinish = finish
	}
}

func (p *openaiChatProvider) Finish() ([]StreamFrame, error) {
	if !p.started || p.finished {
		return nil, nil
	}
	p.finished = true
	id, model := p.identity()
	return []StreamFrame{{ID: id, Model: model, Event: EvFinish,
		FinishReason: p.pendingFinish, HasFinish: true}}, nil
}

// streamUsageFromOpenAI 仅当 usage 含 token 字段时返回规范化用量。
func streamUsageFromOpenAI(obj map[string]json.RawMessage) *Usage {
	usage, ok := decodeObject(field(obj, "usage"))
	if !ok {
		return nil
	}
	hasTokens := false
	for _, k := range []string{"input_tokens", "prompt_tokens", "output_tokens", "completion_tokens", "total_tokens"} {
		if objHasKey(usage, k) {
			hasTokens = true
			break
		}
	}
	if !hasTokens {
		return nil
	}
	return openaiUsageToCanonical(field(obj, "usage"))
}

// ----------------------------------------------------------------------------
// Emitter（帧 → 客户端 SSE）
// ----------------------------------------------------------------------------

type openaiChatEmitter struct {
	seenID    string
	seenModel string
	started   bool
	finished  bool
}

func newOpenAIChatEmitter() *openaiChatEmitter { return &openaiChatEmitter{} }

func (e *openaiChatEmitter) id() string {
	if e.seenID != "" {
		return e.seenID
	}
	return openaiStreamDefaultID
}

func (e *openaiChatEmitter) model() string {
	if e.seenModel != "" {
		return e.seenModel
	}
	return "unknown"
}

func (e *openaiChatEmitter) ensureStarted() []byte {
	if e.started {
		return nil
	}
	e.started = true
	return encodeSSE("", openaiChatRoleChunk(e.id(), e.model()))
}

func (e *openaiChatEmitter) Emit(frame StreamFrame) ([]byte, error) {
	if frame.ID != "" {
		e.seenID = frame.ID
	}
	if frame.Model != "" {
		e.seenModel = frame.Model
	}
	switch frame.Event {
	case EvStart:
		return e.ensureStarted(), nil
	case EvTextDelta:
		out := e.ensureStarted()
		return append(out, encodeSSE("", openaiChatTextChunk(e.id(), e.model(), frame.Text))...), nil
	case EvReasoningDelta:
		out := e.ensureStarted()
		chunk := map[string]any{
			"id": e.id(), "object": "chat.completion.chunk", "model": e.model(),
			"choices": []any{map[string]any{"index": 0,
				"delta": map[string]any{"reasoning_content": frame.Text}, "finish_reason": nil}}}
		return append(out, encodeSSE("", chunk)...), nil
	case EvReasoningSignature:
		return nil, nil
	case EvToolCallStart:
		out := e.ensureStarted()
		chunk := map[string]any{
			"id": e.id(), "object": "chat.completion.chunk", "model": e.model(),
			"choices": []any{map[string]any{"index": 0, "finish_reason": nil,
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index": frame.ToolIndex, "id": frame.CallID, "type": "function",
					"function": map[string]any{"name": frame.Name, "arguments": ""}}}}}}}
		return append(out, encodeSSE("", chunk)...), nil
	case EvToolCallArgsDelta:
		out := e.ensureStarted()
		chunk := map[string]any{
			"id": e.id(), "object": "chat.completion.chunk", "model": e.model(),
			"choices": []any{map[string]any{"index": 0, "finish_reason": nil,
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index":    frame.ToolIndex,
					"function": map[string]any{"arguments": frame.Arguments}}}}}}}
		return append(out, encodeSSE("", chunk)...), nil
	case EvToolResultDelta:
		out := e.ensureStarted()
		delta := map[string]any{"role": "tool", "tool_call_id": frame.ToolUseID, "content": frame.Content}
		if frame.Name != "" {
			delta["name"] = frame.Name
		}
		chunk := map[string]any{
			"id": e.id(), "object": "chat.completion.chunk", "model": e.model(),
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}}
		return append(out, encodeSSE("", chunk)...), nil
	case EvFinish:
		if e.finished {
			return nil, nil
		}
		out := e.ensureStarted()
		out = append(out, encodeSSE("", openaiChatFinishChunk(e.id(), e.model(), frame.FinishReason))...)
		if frame.Usage != nil {
			out = append(out, encodeSSE("", openaiChatUsageChunk(e.id(), e.model(), frame.Usage))...)
		}
		out = append(out, encodeDoneSSE()...)
		e.finished = true
		return out, nil
	default:
		return nil, nil
	}
}

func (e *openaiChatEmitter) Finish() ([]byte, error) {
	if !e.started || e.finished {
		return nil, nil
	}
	e.finished = true
	out := encodeSSE("", openaiChatFinishChunk(e.id(), e.model(), ""))
	return append(out, encodeDoneSSE()...), nil
}
