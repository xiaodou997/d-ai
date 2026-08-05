package formats

import "encoding/json"

// Claude Messages 流式两侧：Provider 把上游 Anthropic SSE（message_start /
// content_block_* / message_delta）解析成中性帧；Emitter 把中性帧重放为 Anthropic
// SSE。对照 Aether formats/claude/messages/stream.rs。
//
// P-B 简化：ToolCallArgsDelta 不做 Claude 特有的 remove_empty_pages 清洗；
// ContentPart（流式多模态）未纳入。

const claudeStreamDefaultID = "msg-local-stream"

// ----------------------------------------------------------------------------
// Provider（上游 Anthropic SSE → 帧）
// ----------------------------------------------------------------------------

type claudeToolStreamState struct {
	callID      string
	name        string
	startedEmit bool
}

type claudeProvider struct {
	fallbackModel string
	seenID        string
	seenModel     string
	started       bool
	finished      bool
	usage         *Usage
	tools         map[int]*claudeToolStreamState
}

func newClaudeProvider(fallbackModel string) *claudeProvider {
	return &claudeProvider{fallbackModel: fallbackModel, tools: map[int]*claudeToolStreamState{}}
}

func (p *claudeProvider) identity() (string, string) {
	return resolveStreamIdentity(p.seenID, p.seenModel, p.fallbackModel, claudeStreamDefaultID)
}

func (p *claudeProvider) ensureStarted(out *[]StreamFrame) {
	if p.started {
		return
	}
	p.started = true
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvStart})
}

func (p *claudeProvider) mergeUsage(u *Usage) {
	if u == nil {
		return
	}
	if p.usage == nil {
		p.usage = u
		return
	}
	if u.InputTokens > 0 {
		p.usage.InputTokens = u.InputTokens
	}
	if u.OutputTokens > 0 {
		p.usage.OutputTokens = u.OutputTokens
	}
	if u.CacheReadTokens > 0 {
		p.usage.CacheReadTokens = u.CacheReadTokens
	}
	if u.CacheWriteTokens > 0 {
		p.usage.CacheWriteTokens = u.CacheWriteTokens
	}
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens
}

func (p *claudeProvider) PushLine(line []byte) ([]StreamFrame, error) {
	raw, ok := decodeSSEData(line)
	if !ok {
		return nil, nil
	}
	obj, ok := decodeObject(raw)
	if !ok {
		return nil, nil
	}
	var out []StreamFrame
	switch getStr(obj, "type") {
	case "message_start":
		if msg, ok := decodeObject(field(obj, "message")); ok {
			if s := getStr(msg, "id"); s != "" {
				p.seenID = s
			}
			if s := getStr(msg, "model"); s != "" {
				p.seenModel = s
			}
			p.mergeUsage(claudeUsageToCanonical(field(msg, "usage")))
		}
		p.ensureStarted(&out)
	case "content_block_start":
		p.handleBlockStart(obj, &out)
	case "content_block_delta":
		p.handleBlockDelta(obj, &out)
	case "message_delta":
		p.handleMessageDelta(obj, &out)
	case "content_block_stop", "message_stop", "ping":
		// 无对应中性帧
	default:
		id, model := p.identity()
		out = append(out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: raw})
	}
	return out, nil
}

func (p *claudeProvider) handleBlockDelta(obj map[string]json.RawMessage, out *[]StreamFrame) {
	index := uintField(obj, "index")
	delta, ok := decodeObject(field(obj, "delta"))
	if !ok {
		return
	}
	id, model := p.identity()
	switch getStr(delta, "type") {
	case "text_delta":
		if piece := getStr(delta, "text"); piece != "" {
			p.ensureStarted(out)
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvTextDelta, Text: piece})
		}
	case "input_json_delta":
		if pj := getStr(delta, "partial_json"); pj != "" {
			p.ensureStarted(out)
			st := p.tool(index)
			if !st.startedEmit {
				*out = append(*out, p.toolStartFrame(id, model, index, st))
				st.startedEmit = true
			}
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
				ToolIndex: index, Arguments: pj})
		}
	case "thinking_delta":
		piece := getStr(delta, "thinking")
		if piece == "" {
			piece = getStr(delta, "text")
		}
		if piece != "" {
			p.ensureStarted(out)
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvReasoningDelta, Text: piece})
		}
	case "signature_delta":
		if sig := getStr(delta, "signature"); sig != "" {
			p.ensureStarted(out)
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvReasoningSignature, Text: sig})
		}
	default:
		*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: mustRaw(obj)})
	}
}

func (p *claudeProvider) handleBlockStart(obj map[string]json.RawMessage, out *[]StreamFrame) {
	index := uintField(obj, "index")
	block, ok := decodeObject(field(obj, "content_block"))
	if !ok {
		return
	}
	id, model := p.identity()
	switch getStr(block, "type") {
	case "thinking":
		piece := getStr(block, "thinking")
		if piece == "" {
			piece = getStr(block, "text")
		}
		if piece != "" {
			p.ensureStarted(out)
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvReasoningDelta, Text: piece})
		}
	case "text":
		if text := getStr(block, "text"); text != "" {
			p.ensureStarted(out)
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvTextDelta, Text: text})
		}
	case "tool_use":
		p.ensureStarted(out)
		st := p.tool(index)
		if cid := getStr(block, "id"); cid != "" {
			st.callID = cid
		}
		if name := getStr(block, "name"); name != "" {
			st.name = name
		}
		if !st.startedEmit {
			*out = append(*out, p.toolStartFrame(id, model, index, st))
			st.startedEmit = true
		}
		args := canonicalizeToolArgs(field(block, "input"))
		if args != "" && args != "{}" {
			*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
				ToolIndex: index, Arguments: args})
		}
	default:
		*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: mustRaw(obj)})
	}
}

func (p *claudeProvider) handleMessageDelta(obj map[string]json.RawMessage, out *[]StreamFrame) {
	p.ensureStarted(out)
	delta, ok := decodeObject(field(obj, "delta"))
	if !ok {
		return
	}
	stop := getStr(delta, "stop_reason")
	finish := claudeStopToOpenAIFinish(stop, stop == "tool_use")
	p.mergeUsage(claudeUsageToCanonical(field(obj, "usage")))
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvFinish,
		FinishReason: finish, HasFinish: true, Usage: p.usage})
	p.finished = true
}

func (p *claudeProvider) tool(index int) *claudeToolStreamState {
	st := p.tools[index]
	if st == nil {
		st = &claudeToolStreamState{}
		p.tools[index] = st
	}
	return st
}

func (p *claudeProvider) toolStartFrame(id, model string, index int, st *claudeToolStreamState) StreamFrame {
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

func (p *claudeProvider) Finish() ([]StreamFrame, error) {
	if !p.started || p.finished {
		return nil, nil
	}
	p.finished = true
	id, model := p.identity()
	return []StreamFrame{{ID: id, Model: model, Event: EvFinish, HasFinish: true}}, nil
}

func uintField(obj map[string]json.RawMessage, key string) int {
	if v, ok := asUint(field(obj, key)); ok {
		return int(v)
	}
	return 0
}

// ----------------------------------------------------------------------------
// Emitter（帧 → Anthropic SSE）
// ----------------------------------------------------------------------------

type claudeBlockKind int

const (
	claudeBlockNone claudeBlockKind = iota
	claudeBlockText
	claudeBlockThinking
	claudeBlockTool
)

type claudeEmitter struct {
	seenID         string
	seenModel      string
	started        bool
	finished       bool
	nextBlockIndex int
	openKind       claudeBlockKind
	openIndex      int
	openToolIndex  int
	toolBlockIndex map[int]int
	toolStates     map[int]*claudeToolStreamState
}

func newClaudeEmitter() *claudeEmitter {
	return &claudeEmitter{openKind: claudeBlockNone,
		toolBlockIndex: map[int]int{}, toolStates: map[int]*claudeToolStreamState{}}
}

func (e *claudeEmitter) id() string {
	if e.seenID != "" {
		return e.seenID
	}
	return claudeStreamDefaultID
}

func (e *claudeEmitter) model() string {
	if e.seenModel != "" {
		return e.seenModel
	}
	return "unknown"
}

func (e *claudeEmitter) ensureStarted() []byte {
	if e.started {
		return nil
	}
	e.started = true
	return encodeSSE("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": e.id(), "type": "message", "role": "assistant", "model": e.model(),
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}})
}

func (e *claudeEmitter) closeOpenBlock() []byte {
	if e.openKind == claudeBlockNone {
		return nil
	}
	idx := e.openIndex
	e.openKind = claudeBlockNone
	return encodeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
}

func (e *claudeEmitter) ensureTextBlock() []byte {
	if e.openKind == claudeBlockText {
		return nil
	}
	out := e.closeOpenBlock()
	idx := e.nextBlockIndex
	e.nextBlockIndex++
	e.openKind, e.openIndex = claudeBlockText, idx
	return append(out, encodeSSE("content_block_start", map[string]any{
		"type": "content_block_start", "index": idx,
		"content_block": map[string]any{"type": "text", "text": ""}})...)
}

func (e *claudeEmitter) ensureThinkingBlock() []byte {
	if e.openKind == claudeBlockThinking {
		return nil
	}
	out := e.closeOpenBlock()
	idx := e.nextBlockIndex
	e.nextBlockIndex++
	e.openKind, e.openIndex = claudeBlockThinking, idx
	return append(out, encodeSSE("content_block_start", map[string]any{
		"type": "content_block_start", "index": idx,
		"content_block": map[string]any{"type": "thinking", "thinking": ""}})...)
}

func (e *claudeEmitter) ensureToolBlock(toolIndex int, callID, name string) []byte {
	if e.openKind == claudeBlockTool && e.openToolIndex == toolIndex {
		return nil
	}
	out := e.closeOpenBlock()
	idx, ok := e.toolBlockIndex[toolIndex]
	if !ok {
		idx = e.nextBlockIndex
		e.nextBlockIndex++
		e.toolBlockIndex[toolIndex] = idx
	}
	e.openKind, e.openIndex, e.openToolIndex = claudeBlockTool, idx, toolIndex
	return append(out, encodeSSE("content_block_start", map[string]any{
		"type": "content_block_start", "index": idx,
		"content_block": map[string]any{"type": "tool_use", "id": callID, "name": name, "input": map[string]any{}}})...)
}

func (e *claudeEmitter) Emit(frame StreamFrame) ([]byte, error) {
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
		out = append(out, e.ensureTextBlock()...)
		return append(out, encodeSSE("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": e.openIndex,
			"delta": map[string]any{"type": "text_delta", "text": frame.Text}})...), nil
	case EvReasoningDelta:
		out := e.ensureStarted()
		out = append(out, e.ensureThinkingBlock()...)
		return append(out, encodeSSE("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": e.openIndex,
			"delta": map[string]any{"type": "thinking_delta", "thinking": frame.Text}})...), nil
	case EvReasoningSignature:
		out := e.ensureStarted()
		out = append(out, e.ensureThinkingBlock()...)
		return append(out, encodeSSE("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": e.openIndex,
			"delta": map[string]any{"type": "signature_delta", "signature": frame.Text}})...), nil
	case EvToolCallStart:
		out := e.ensureStarted()
		e.toolStates[frame.ToolIndex] = &claudeToolStreamState{callID: frame.CallID, name: frame.Name}
		return append(out, e.ensureToolBlock(frame.ToolIndex, frame.CallID, frame.Name)...), nil
	case EvToolCallArgsDelta:
		if frame.Arguments == "" {
			return nil, nil
		}
		out := e.ensureStarted()
		st := e.toolStates[frame.ToolIndex]
		callID, name := generatedToolCallID(frame.ToolIndex), "unknown"
		if st != nil {
			if st.callID != "" {
				callID = st.callID
			}
			if st.name != "" {
				name = st.name
			}
		}
		out = append(out, e.ensureToolBlock(frame.ToolIndex, callID, name)...)
		return append(out, encodeSSE("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": e.openIndex,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": frame.Arguments}})...), nil
	case EvToolResultDelta:
		return e.emitToolResult(frame), nil
	case EvFinish:
		if e.finished {
			return nil, nil
		}
		out := e.ensureStarted()
		out = append(out, e.closeOpenBlock()...)
		usage := frame.Usage
		if usage == nil {
			usage = &Usage{}
		}
		out = append(out, encodeSSE("message_delta", map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   openaiFinishToClaudeStop(frame.FinishReason),
				"stop_sequence": nil},
			"usage": map[string]any{"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens}})...)
		out = append(out, encodeSSE("message_stop", map[string]any{"type": "message_stop"})...)
		e.finished = true
		return out, nil
	default:
		return nil, nil
	}
}

func (e *claudeEmitter) emitToolResult(frame StreamFrame) []byte {
	out := e.ensureStarted()
	out = append(out, e.closeOpenBlock()...)
	idx := e.nextBlockIndex
	e.nextBlockIndex++
	cb := map[string]any{"type": "tool_result", "tool_use_id": frame.ToolUseID, "content": frame.Content}
	if frame.Name != "" {
		cb["name"] = frame.Name
	}
	out = append(out, encodeSSE("content_block_start", map[string]any{
		"type": "content_block_start", "index": idx, "content_block": cb})...)
	return append(out, encodeSSE("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": idx})...)
}

func (e *claudeEmitter) Finish() ([]byte, error) {
	if !e.started || e.finished {
		return nil, nil
	}
	return e.Emit(StreamFrame{ID: e.id(), Model: e.model(), Event: EvFinish, HasFinish: true})
}
