package formats

import (
	"encoding/json"
	"sort"
	"strings"
)

// OpenAI Responses（/v1/responses）流式两侧：Provider 把上游事件流（response.created
// / response.output_text.delta / response.completed 等）解析成中性帧；Emitter 把中性帧
// 重放为同款 event-based SSE。对照 Aether formats/openai/chat/stream.rs 的
// OpenAIResponsesProviderState / OpenAIResponsesClientEmitter（responses/stream.rs 仅
// re-export 它们）。
//
// 差分增量：Provider 对文本/思考/工具参数/工具结果均维护累积缓冲，仅产出后缀增量，
// 既容忍上游发累积量，也避免 *.delta 与 *.done/response.completed 携全量时重复发帧。

const responsesStreamDefaultID = "resp-local-stream"

// ----------------------------------------------------------------------------
// Provider（上游 Responses 事件 SSE → 帧）
// ----------------------------------------------------------------------------

type responsesProviderToolState struct {
	callID      string
	name        string
	arguments   string
	startedEmit bool
}

type responsesProviderToolResultState struct {
	content string
	emitted bool
}

type responsesProvider struct {
	fallbackModel    string
	seenID           string
	seenModel        string
	started          bool
	finished         bool
	text             string
	reasoning        string
	tools            map[int]*responsesProviderToolState
	toolResults      map[int]*responsesProviderToolResultState
	toolIndexByKey   map[string]int
	lastToolIndex    int
	hasLastToolIndex bool
}

func newResponsesProvider(fallbackModel string) *responsesProvider {
	return &responsesProvider{
		fallbackModel:  fallbackModel,
		tools:          map[int]*responsesProviderToolState{},
		toolResults:    map[int]*responsesProviderToolResultState{},
		toolIndexByKey: map[string]int{},
	}
}

func (p *responsesProvider) identity() (string, string) {
	return resolveStreamIdentity(p.seenID, p.seenModel, p.fallbackModel, responsesStreamDefaultID)
}

func (p *responsesProvider) ensureStarted(out *[]StreamFrame) {
	if p.started {
		return
	}
	p.started = true
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvStart})
}

func (p *responsesProvider) tool(index int) *responsesProviderToolState {
	st := p.tools[index]
	if st == nil {
		st = &responsesProviderToolState{}
		p.tools[index] = st
	}
	return st
}

func (p *responsesProvider) toolIndexForKey(key string, hasKey bool, outputIndex int, hasOut bool) int {
	if hasOut {
		if hasKey {
			if _, exists := p.toolIndexByKey[key]; !exists {
				p.toolIndexByKey[key] = outputIndex
			}
		}
		p.lastToolIndex, p.hasLastToolIndex = outputIndex, true
		return outputIndex
	}
	if hasKey {
		if idx, ok := p.toolIndexByKey[key]; ok {
			p.lastToolIndex, p.hasLastToolIndex = idx, true
			return idx
		}
	}
	index := len(p.tools)
	if p.hasLastToolIndex {
		index = p.lastToolIndex
	}
	if hasKey {
		p.toolIndexByKey[key] = index
	}
	p.lastToolIndex, p.hasLastToolIndex = index, true
	return index
}

func (p *responsesProvider) PushLine(line []byte) ([]StreamFrame, error) {
	raw, ok := decodeSSEData(line)
	if !ok {
		return nil, nil
	}
	obj, ok := decodeObject(raw)
	if !ok {
		return nil, nil
	}
	if resp, ok := decodeObject(field(obj, "response")); ok {
		if s := getStr(resp, "id"); s != "" {
			p.seenID = s
		}
		if s := getStr(resp, "model"); s != "" {
			p.seenModel = s
		}
	}

	var out []StreamFrame
	switch getStr(obj, "type") {
	case "response.created", "response.in_progress":
		p.ensureStarted(&out)
	case "response.output_text.delta", "response.outtext.delta":
		if piece := responsesDeltaText(field(obj, "delta")); piece != "" {
			p.ensureStarted(&out)
			p.text += piece
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvTextDelta, Text: piece})
		}
	case "response.content_part.added", "response.content_part.done":
		if part, ok := decodeObject(field(obj, "part")); ok && getStr(part, "type") == "output_text" {
			if text := getStr(part, "text"); text != "" {
				p.emitMissingText(text, &out)
			}
		}
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		if part, ok := decodeObject(field(obj, "part")); ok && getStr(part, "type") == "summary_text" {
			if text := getStr(part, "text"); text != "" {
				p.emitMissingReasoning(text, &out)
			}
		}
	case "response.output_text.done":
		if text := responsesDoneText(obj); text != "" {
			p.emitMissingText(text, &out)
		}
	case "response.reasoning_summary_text.delta":
		if piece := getStr(obj, "delta"); piece != "" {
			p.ensureStarted(&out)
			p.reasoning += piece
			id, model := p.identity()
			out = append(out, StreamFrame{ID: id, Model: model, Event: EvReasoningDelta, Text: piece})
		}
	case "response.reasoning_summary_text.done":
		if text := responsesDoneText(obj); text != "" {
			p.emitMissingReasoning(text, &out)
		}
	case "response.output_item.added", "response.output_item.done":
		if item, ok := decodeObject(field(obj, "item")); ok {
			outputIndex, hasOut := responsesOutputIndex(obj)
			p.dispatchOutputItem(item, outputIndex, hasOut, &out)
		}
	case "response.function_call_arguments.delta":
		p.handleArgsDelta(obj, &out)
	case "response.function_call_arguments.done":
		p.handleArgsDone(obj, &out)
	case "response.function_call_output.delta", "response.function_call_output.done":
		p.handleToolOutput(obj, &out)
	case "response.completed":
		p.handleCompleted(obj, &out)
	default:
		id, model := p.identity()
		out = append(out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: raw})
	}
	return out, nil
}

func (p *responsesProvider) dispatchOutputItem(item map[string]json.RawMessage, outputIndex int, hasOut bool, out *[]StreamFrame) {
	switch getStr(item, "type") {
	case "function_call":
		p.emitToolCallItem(item, outputIndex, hasOut, out)
	case "function_call_output":
		p.emitToolResultItem(item, outputIndex, hasOut, out)
	case "message":
		p.emitMessageItem(item, out)
	case "reasoning":
		p.emitReasoningItem(item, out)
	default:
		id, model := p.identity()
		*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: mustRaw(item)})
	}
}

func (p *responsesProvider) emitToolCallItem(item map[string]json.RawMessage, outputIndex int, hasOut bool, out *[]StreamFrame) {
	p.ensureStarted(out)
	key, hasKey := firstStr(item, "call_id", "id")
	index := p.toolIndexForKey(key, hasKey, outputIndex, hasOut)
	id, model := p.identity()
	st := p.tool(index)
	if v, ok := firstStr(item, "call_id", "id"); ok {
		st.callID = v
	}
	if v := getStr(item, "name"); v != "" {
		st.name = v
	}
	if !st.startedEmit {
		*out = append(*out, responsesToolStartFrame(id, model, index, st))
		st.startedEmit = true
	}
	completed := getStr(item, "arguments")
	missing := streamTextDelta(completed, st.arguments)
	if missing == "" {
		return
	}
	st.arguments += missing
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
		ToolIndex: index, Arguments: missing})
}

func (p *responsesProvider) emitToolResultItem(item map[string]json.RawMessage, outputIndex int, hasOut bool, out *[]StreamFrame) {
	toolUseID, ok := firstStr(item, "call_id", "tool_call_id", "id")
	if !ok {
		toolUseID = "call_auto_0"
	}
	index := p.toolIndexForKey("function_call_output:"+toolUseID, true, outputIndex, hasOut)
	content := openaiToolResultContentFromValue(fieldOr(item, "output", "content", "delta"))
	name, hasName := firstStr(item, "name")
	p.emitMissingToolResult(index, toolUseID, name, hasName, content, out)
}

func (p *responsesProvider) emitMessageItem(item map[string]json.RawMessage, out *[]StreamFrame) {
	if getStr(item, "type") != "message" {
		return
	}
	var b strings.Builder
	if content, ok := decodeArray(field(item, "content")); ok {
		for _, c := range content {
			co, ok := decodeObject(c)
			if !ok {
				continue
			}
			if getStr(co, "type") == "output_text" {
				b.WriteString(getStr(co, "text"))
			}
		}
	}
	if b.Len() > 0 {
		p.emitMissingText(b.String(), out)
	}
}

func (p *responsesProvider) emitReasoningItem(item map[string]json.RawMessage, out *[]StreamFrame) {
	if getStr(item, "type") != "reasoning" {
		return
	}
	var b strings.Builder
	if summary, ok := decodeArray(field(item, "summary")); ok {
		for _, s := range summary {
			so, ok := decodeObject(s)
			if !ok {
				continue
			}
			if getStr(so, "type") == "summary_text" {
				b.WriteString(getStr(so, "text"))
			}
		}
	}
	if b.Len() > 0 {
		p.emitMissingReasoning(b.String(), out)
	}
}

func (p *responsesProvider) handleArgsDelta(obj map[string]json.RawMessage, out *[]StreamFrame) {
	delta := getStr(obj, "delta")
	if delta == "" {
		return
	}
	p.ensureStarted(out)
	key, hasKey := firstStr(obj, "item_id", "call_id", "id")
	outputIndex, hasOut := responsesOutputIndex(obj)
	index := p.toolIndexForKey(key, hasKey, outputIndex, hasOut)
	id, model := p.identity()
	st := p.tool(index)
	if v, ok := firstStr(obj, "item_id", "call_id", "id"); ok {
		st.callID = v
	}
	if !st.startedEmit {
		*out = append(*out, responsesToolStartFrame(id, model, index, st))
		st.startedEmit = true
	}
	st.arguments += delta
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
		ToolIndex: index, Arguments: delta})
}

func (p *responsesProvider) handleArgsDone(obj map[string]json.RawMessage, out *[]StreamFrame) {
	arguments := getStr(obj, "arguments")
	if arguments == "" {
		if item, ok := decodeObject(field(obj, "item")); ok {
			arguments = getStr(item, "arguments")
		}
	}
	if arguments == "" {
		return
	}
	p.ensureStarted(out)
	key, hasKey := firstStr(obj, "item_id", "call_id", "id")
	if !hasKey {
		if item, ok := decodeObject(field(obj, "item")); ok {
			key, hasKey = firstStr(item, "call_id", "id")
		}
	}
	outputIndex, hasOut := responsesOutputIndex(obj)
	index := p.toolIndexForKey(key, hasKey, outputIndex, hasOut)
	id, model := p.identity()
	st := p.tool(index)
	if v, ok := firstStr(obj, "item_id", "call_id", "id"); ok {
		st.callID = v
	} else if item, ok := decodeObject(field(obj, "item")); ok {
		if v, ok := firstStr(item, "call_id", "id"); ok {
			st.callID = v
		}
	}
	if !st.startedEmit {
		*out = append(*out, responsesToolStartFrame(id, model, index, st))
		st.startedEmit = true
	}
	missing := streamTextDelta(arguments, st.arguments)
	if missing == "" {
		return
	}
	st.arguments += missing
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvToolCallArgsDelta,
		ToolIndex: index, Arguments: missing})
}

func (p *responsesProvider) handleToolOutput(obj map[string]json.RawMessage, out *[]StreamFrame) {
	toolUseID, ok := firstStr(obj, "call_id", "tool_call_id", "item_id", "id")
	if !ok {
		toolUseID = "call_auto_0"
	}
	outputIndex, hasOut := responsesOutputIndex(obj)
	index := p.toolIndexForKey("function_call_output:"+toolUseID, true, outputIndex, hasOut)
	content := openaiToolResultContentFromValue(fieldOr(obj, "delta", "output", "content"))
	name, hasName := firstStr(obj, "name")
	p.emitMissingToolResult(index, toolUseID, name, hasName, content, out)
}

func (p *responsesProvider) handleCompleted(obj map[string]json.RawMessage, out *[]StreamFrame) {
	resp, ok := decodeObject(field(obj, "response"))
	if !ok {
		return
	}
	p.ensureStarted(out)
	if items, ok := decodeArray(field(resp, "output")); ok {
		for outputIndex, it := range items {
			item, ok := decodeObject(it)
			if !ok {
				continue
			}
			switch getStr(item, "type") {
			case "message":
				p.emitMessageItem(item, out)
			case "function_call":
				p.emitToolCallItem(item, outputIndex, true, out)
			case "function_call_output":
				p.emitToolResultItem(item, outputIndex, true, out)
			case "reasoning":
				p.emitReasoningItem(item, out)
			default:
				id, model := p.identity()
				*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvUnknown, Unknown: it})
			}
		}
	}
	finish := "stop"
	if len(p.tools) > 0 {
		finish = "tool_calls"
	}
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvFinish,
		FinishReason: finish, HasFinish: true, Usage: openaiUsageToCanonical(field(resp, "usage"))})
	p.finished = true
}

func (p *responsesProvider) emitMissingText(text string, out *[]StreamFrame) {
	missing := streamTextDelta(text, p.text)
	if missing == "" {
		return
	}
	p.ensureStarted(out)
	p.text += missing
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvTextDelta, Text: missing})
}

func (p *responsesProvider) emitMissingReasoning(text string, out *[]StreamFrame) {
	missing := streamTextDelta(text, p.reasoning)
	if missing == "" {
		return
	}
	p.ensureStarted(out)
	p.reasoning += missing
	id, model := p.identity()
	*out = append(*out, StreamFrame{ID: id, Model: model, Event: EvReasoningDelta, Text: missing})
}

func (p *responsesProvider) emitMissingToolResult(index int, toolUseID, name string, hasName bool, content string, out *[]StreamFrame) {
	st := p.toolResults[index]
	if st == nil {
		st = &responsesProviderToolResultState{}
		p.toolResults[index] = st
	}
	missing := content
	if st.emitted {
		missing = streamTextDelta(content, st.content)
	}
	if missing == "" && st.emitted {
		return
	}
	st.emitted = true
	st.content += missing
	p.ensureStarted(out)
	id, model := p.identity()
	frame := StreamFrame{ID: id, Model: model, Event: EvToolResultDelta,
		ToolIndex: index, ToolUseID: toolUseID, Content: missing}
	if hasName {
		frame.Name = name
	}
	*out = append(*out, frame)
}

func (p *responsesProvider) Finish() ([]StreamFrame, error) {
	if !p.started || p.finished {
		return nil, nil
	}
	p.finished = true
	finish := "stop"
	if len(p.tools) > 0 {
		finish = "tool_calls"
	}
	id, model := p.identity()
	return []StreamFrame{{ID: id, Model: model, Event: EvFinish, FinishReason: finish, HasFinish: true}}, nil
}

func responsesToolStartFrame(id, model string, index int, st *responsesProviderToolState) StreamFrame {
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

// responsesDeltaText 取 delta（字符串或 {text}）的文本。
func responsesDeltaText(raw json.RawMessage) string {
	if s, ok := asString(raw); ok {
		return s
	}
	if o, ok := decodeObject(raw); ok {
		return getStr(o, "text")
	}
	return ""
}

// responsesDoneText 取 *.done 事件的全量文本：优先 text，回退 part.text。
func responsesDoneText(obj map[string]json.RawMessage) string {
	if s, ok := asString(field(obj, "text")); ok {
		return s
	}
	if part, ok := decodeObject(field(obj, "part")); ok {
		return getStr(part, "text")
	}
	return ""
}

func responsesOutputIndex(obj map[string]json.RawMessage) (int, bool) {
	if v, ok := asUint(field(obj, "output_index")); ok {
		return int(v), true
	}
	return 0, false
}

// openaiToolResultContentFromValue 把工具结果值折叠成字符串（字符串直出，空→空，
// 其余→紧凑 JSON）。
func openaiToolResultContentFromValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if s, ok := asString(raw); ok {
		return s
	}
	return compactJSON(raw)
}

// firstStr 返回 obj 中首个存在且非空的字符串字段。
func firstStr(obj map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, k := range keys {
		if s, ok := asString(field(obj, k)); ok && trimmed(s) != "" {
			return s, true
		}
	}
	return "", false
}

// sortedIntKeys 返回 map 的整型键升序切片（保证 finish/completed 输出顺序确定）。
func sortedIntKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// ----------------------------------------------------------------------------
// Emitter（帧 → Responses 事件 SSE）
// ----------------------------------------------------------------------------

type responsesEmitterToolState struct {
	callID         string
	name           string
	arguments      string
	outputIndex    int
	hasOutputIndex bool
}

type responsesEmitterToolResultState struct {
	toolUseID      string
	name           string
	hasName        bool
	content        string
	outputIndex    int
	hasOutputIndex bool
	itemStarted    bool
}

type responsesEmitter struct {
	seenID          string
	seenModel       string
	messageItemID   string
	reasoningItemID string
	started         bool
	finished        bool
	sequenceNumber  uint64
	nextOutputIndex int

	reasoningItemStarted    bool
	reasoningPartStarted    bool
	reasoningOutputIndex    int
	hasReasoningOutputIndex bool

	textItemStarted       bool
	textPartStarted       bool
	messageOutputIndex    int
	hasMessageOutputIndex bool

	text      string
	reasoning string
	tools     map[int]*responsesEmitterToolState
	results   map[int]*responsesEmitterToolResultState
}

func newResponsesEmitter() *responsesEmitter {
	return &responsesEmitter{
		tools:   map[int]*responsesEmitterToolState{},
		results: map[int]*responsesEmitterToolResultState{},
	}
}

func (e *responsesEmitter) responseID() string {
	if e.seenID != "" {
		return e.seenID
	}
	return responsesStreamDefaultID
}

func (e *responsesEmitter) model() string {
	if e.seenModel != "" {
		return e.seenModel
	}
	return "unknown"
}

func (e *responsesEmitter) messageID() string {
	if e.messageItemID != "" {
		return e.messageItemID
	}
	return e.responseID() + "_msg"
}

func (e *responsesEmitter) reasoningID() string {
	if e.reasoningItemID != "" {
		return e.reasoningItemID
	}
	return e.responseID() + "_rs_0"
}

func (e *responsesEmitter) ensureMessageID() string {
	if e.messageItemID == "" {
		e.messageItemID = e.responseID() + "_msg"
	}
	return e.messageItemID
}

func (e *responsesEmitter) ensureReasoningID() string {
	if e.reasoningItemID == "" {
		e.reasoningItemID = e.responseID() + "_rs_0"
	}
	return e.reasoningItemID
}

func (e *responsesEmitter) nextSeq() uint64 {
	e.sequenceNumber++
	return e.sequenceNumber
}

func (e *responsesEmitter) allocOutputIndex() int {
	idx := e.nextOutputIndex
	e.nextOutputIndex++
	return idx
}

func (e *responsesEmitter) event(eventType string, payload map[string]any) []byte {
	payload["sequence_number"] = e.nextSeq()
	return encodeSSE(eventType, payload)
}

func (e *responsesEmitter) inProgressResponse() map[string]any {
	return map[string]any{
		"id": e.responseID(), "object": "response", "model": e.model(),
		"status": "in_progress", "output": []any{}}
}

func (e *responsesEmitter) ensureStarted() []byte {
	if e.started {
		return nil
	}
	e.started = true
	out := e.event("response.created", map[string]any{
		"type": "response.created", "response": e.inProgressResponse()})
	out = append(out, e.event("response.in_progress", map[string]any{
		"type": "response.in_progress", "response": e.inProgressResponse()})...)
	return out
}

func (e *responsesEmitter) ensureReasoningOutputIndex() int {
	if !e.hasReasoningOutputIndex {
		e.reasoningOutputIndex, e.hasReasoningOutputIndex = e.allocOutputIndex(), true
	}
	return e.reasoningOutputIndex
}

func (e *responsesEmitter) ensureMessageOutputIndex() int {
	if !e.hasMessageOutputIndex {
		e.messageOutputIndex, e.hasMessageOutputIndex = e.allocOutputIndex(), true
	}
	return e.messageOutputIndex
}

func (e *responsesEmitter) ensureToolOutputIndex(index int) int {
	st := e.tool(index)
	if !st.hasOutputIndex {
		st.outputIndex, st.hasOutputIndex = e.allocOutputIndex(), true
	}
	return st.outputIndex
}

func (e *responsesEmitter) ensureToolResultOutputIndex(index int) int {
	st := e.result(index)
	if !st.hasOutputIndex {
		st.outputIndex, st.hasOutputIndex = e.allocOutputIndex(), true
	}
	return st.outputIndex
}

func (e *responsesEmitter) tool(index int) *responsesEmitterToolState {
	st := e.tools[index]
	if st == nil {
		st = &responsesEmitterToolState{}
		e.tools[index] = st
	}
	return st
}

func (e *responsesEmitter) result(index int) *responsesEmitterToolResultState {
	st := e.results[index]
	if st == nil {
		st = &responsesEmitterToolResultState{}
		e.results[index] = st
	}
	return st
}

func (e *responsesEmitter) ensureReasoningItemStarted() []byte {
	out := e.ensureStarted()
	outputIndex := e.ensureReasoningOutputIndex()
	itemID := e.ensureReasoningID()
	if !e.reasoningItemStarted {
		out = append(out, e.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": e.responseID(),
			"output_index": outputIndex,
			"item":         map[string]any{"type": "reasoning", "id": itemID, "summary": []any{}}})...)
		e.reasoningItemStarted = true
	}
	if !e.reasoningPartStarted {
		out = append(out, e.event("response.reasoning_summary_part.added", map[string]any{
			"type": "response.reasoning_summary_part.added", "response_id": e.responseID(),
			"item_id": itemID, "output_index": outputIndex, "summary_index": 0,
			"part": map[string]any{"type": "summary_text", "text": ""}})...)
		e.reasoningPartStarted = true
	}
	return out
}

func (e *responsesEmitter) ensureTextItemStarted() []byte {
	out := e.ensureStarted()
	outputIndex := e.ensureMessageOutputIndex()
	itemID := e.ensureMessageID()
	if !e.textItemStarted {
		out = append(out, e.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": e.responseID(),
			"output_index": outputIndex,
			"item": map[string]any{"type": "message", "id": itemID, "status": "in_progress",
				"role": "assistant", "content": []any{}}})...)
		e.textItemStarted = true
	}
	if !e.textPartStarted {
		out = append(out, e.event("response.content_part.added", map[string]any{
			"type": "response.content_part.added", "response_id": e.responseID(),
			"output_index": outputIndex, "item_id": itemID, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})...)
		e.textPartStarted = true
	}
	return out
}

func (e *responsesEmitter) Emit(frame StreamFrame) ([]byte, error) {
	if frame.ID != "" {
		e.seenID = strings.ReplaceAll(frame.ID, "chatcmpl", "resp")
	}
	if frame.Model != "" {
		e.seenModel = frame.Model
	}
	switch frame.Event {
	case EvStart:
		return e.ensureStarted(), nil
	case EvTextDelta:
		out := e.ensureTextItemStarted()
		e.text += frame.Text
		out = append(out, e.event("response.output_text.delta", map[string]any{
			"type": "response.output_text.delta", "response_id": e.responseID(),
			"output_index": e.messageOutputIndex, "item_id": e.messageID(),
			"content_index": 0, "delta": frame.Text})...)
		return out, nil
	case EvReasoningDelta:
		out := e.ensureReasoningItemStarted()
		e.reasoning += frame.Text
		out = append(out, e.event("response.reasoning_summary_text.delta", map[string]any{
			"type": "response.reasoning_summary_text.delta", "response_id": e.responseID(),
			"item_id": e.reasoningID(), "output_index": e.reasoningOutputIndex,
			"summary_index": 0, "delta": frame.Text})...)
		return out, nil
	case EvReasoningSignature:
		return nil, nil
	case EvToolCallStart:
		out := e.ensureStarted()
		outputIndex := e.ensureToolOutputIndex(frame.ToolIndex)
		st := e.tool(frame.ToolIndex)
		st.callID = frame.CallID
		st.name = frame.Name
		out = append(out, e.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": e.responseID(),
			"output_index": outputIndex,
			"item": map[string]any{"type": "function_call", "id": frame.CallID,
				"call_id": st.callID, "name": st.name, "arguments": "", "status": "in_progress"}})...)
		return out, nil
	case EvToolCallArgsDelta:
		out := e.ensureStarted()
		outputIndex := e.ensureToolOutputIndex(frame.ToolIndex)
		st := e.tool(frame.ToolIndex)
		st.arguments += frame.Arguments
		itemID := st.callID
		if itemID == "" {
			itemID = generatedToolCallID(frame.ToolIndex)
		}
		out = append(out, e.event("response.function_call_arguments.delta", map[string]any{
			"type": "response.function_call_arguments.delta", "response_id": e.responseID(),
			"output_index": outputIndex, "item_id": itemID, "call_id": itemID,
			"delta": frame.Arguments})...)
		return out, nil
	case EvToolResultDelta:
		return e.emitToolResult(frame), nil
	case EvFinish:
		if e.finished {
			return nil, nil
		}
		out := e.ensureStarted()
		out = append(out, e.finishReasoningItem()...)
		out = append(out, e.finishTextItem()...)
		out = append(out, e.finishToolItems()...)
		out = append(out, e.finishToolResultItems()...)
		out = append(out, e.event("response.completed", map[string]any{
			"type": "response.completed", "response": e.completedResponse(frame.Usage)})...)
		e.finished = true
		return out, nil
	default:
		return nil, nil
	}
}

func (e *responsesEmitter) emitToolResult(frame StreamFrame) []byte {
	out := e.ensureStarted()
	outputIndex := e.ensureToolResultOutputIndex(frame.ToolIndex)
	st := e.result(frame.ToolIndex)
	if st.toolUseID == "" {
		st.toolUseID = frame.ToolUseID
	}
	if frame.Name != "" {
		st.name, st.hasName = frame.Name, true
	}
	toolUseID := st.toolUseID
	if toolUseID == "" {
		toolUseID = frame.ToolUseID
	}
	if !st.itemStarted {
		item := map[string]any{"type": "function_call_output",
			"id": toolUseID + "_output", "call_id": toolUseID, "output": ""}
		if st.hasName && trimmed(st.name) != "" {
			item["name"] = st.name
		}
		out = append(out, e.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": e.responseID(),
			"output_index": outputIndex, "item": item})...)
		st.itemStarted = true
	}
	st.content += frame.Content
	if frame.Content != "" {
		out = append(out, e.event("response.function_call_output.delta", map[string]any{
			"type": "response.function_call_output.delta", "response_id": e.responseID(),
			"output_index": outputIndex, "item_id": toolUseID + "_output",
			"call_id": toolUseID, "delta": frame.Content})...)
	}
	return out
}

func (e *responsesEmitter) finishTextItem() []byte {
	if !e.textItemStarted {
		return nil
	}
	itemID := e.messageID()
	outputIndex := e.messageOutputIndex
	var out []byte
	if e.textPartStarted {
		out = append(out, e.event("response.output_text.done", map[string]any{
			"type": "response.output_text.done", "response_id": e.responseID(),
			"output_index": outputIndex, "item_id": itemID, "content_index": 0,
			"text": e.text})...)
		out = append(out, e.event("response.content_part.done", map[string]any{
			"type": "response.content_part.done", "response_id": e.responseID(),
			"output_index": outputIndex, "item_id": itemID, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": e.text, "annotations": []any{}}})...)
	}
	out = append(out, e.event("response.output_item.done", map[string]any{
		"type": "response.output_item.done", "response_id": e.responseID(),
		"output_index": outputIndex,
		"item": map[string]any{"type": "message", "id": itemID, "status": "completed",
			"role": "assistant", "content": []any{map[string]any{
				"type": "output_text", "text": e.text, "annotations": []any{}}}}})...)
	return out
}

func (e *responsesEmitter) finishReasoningItem() []byte {
	if !e.reasoningItemStarted {
		return nil
	}
	itemID := e.reasoningID()
	outputIndex := e.reasoningOutputIndex
	var out []byte
	if e.reasoningPartStarted {
		out = append(out, e.event("response.reasoning_summary_text.done", map[string]any{
			"type": "response.reasoning_summary_text.done", "response_id": e.responseID(),
			"item_id": itemID, "output_index": outputIndex, "summary_index": 0,
			"text": e.reasoning})...)
		out = append(out, e.event("response.reasoning_summary_part.done", map[string]any{
			"type": "response.reasoning_summary_part.done", "response_id": e.responseID(),
			"item_id": itemID, "output_index": outputIndex, "summary_index": 0,
			"part": map[string]any{"type": "summary_text", "text": e.reasoning}})...)
	}
	out = append(out, e.event("response.output_item.done", map[string]any{
		"type": "response.output_item.done", "response_id": e.responseID(),
		"output_index": outputIndex,
		"item": map[string]any{"type": "reasoning", "id": itemID,
			"summary": []any{map[string]any{"type": "summary_text", "text": e.reasoning}}}})...)
	return out
}

func (e *responsesEmitter) finishToolItems() []byte {
	var out []byte
	for _, index := range sortedIntKeys(e.tools) {
		outputIndex := e.ensureToolOutputIndex(index)
		st := e.tools[index]
		itemID := st.callID
		if itemID == "" {
			itemID = generatedToolCallID(index)
		}
		name := st.name
		if name == "" {
			name = "unknown"
		}
		out = append(out, e.event("response.function_call_arguments.done", map[string]any{
			"type": "response.function_call_arguments.done", "response_id": e.responseID(),
			"output_index": outputIndex, "item_id": itemID, "call_id": itemID,
			"arguments": st.arguments})...)
		out = append(out, e.event("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "response_id": e.responseID(),
			"output_index": outputIndex,
			"item": map[string]any{"type": "function_call", "id": itemID, "call_id": itemID,
				"name": name, "arguments": st.arguments, "status": "completed"}})...)
	}
	return out
}

func (e *responsesEmitter) finishToolResultItems() []byte {
	var out []byte
	for _, index := range sortedIntKeys(e.results) {
		outputIndex := e.ensureToolResultOutputIndex(index)
		st := e.results[index]
		itemID := st.toolUseID
		if itemID == "" {
			itemID = generatedToolCallID(index)
		}
		item := map[string]any{"type": "function_call_output",
			"id": itemID + "_output", "call_id": itemID, "output": st.content}
		if st.hasName && trimmed(st.name) != "" {
			item["name"] = st.name
		}
		out = append(out, e.event("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "response_id": e.responseID(),
			"output_index": outputIndex, "item": item})...)
	}
	return out
}

func (e *responsesEmitter) completedResponse(usage *Usage) map[string]any {
	type ordered struct {
		idx  int
		item map[string]any
	}
	var items []ordered
	if trimmed(e.reasoning) != "" {
		items = append(items, ordered{e.reasoningOutputIndex, map[string]any{
			"type": "reasoning", "id": e.reasoningID(), "status": "completed",
			"summary": []any{map[string]any{"type": "summary_text", "text": e.reasoning}}}})
	}
	if e.textItemStarted || e.text != "" {
		items = append(items, ordered{e.messageOutputIndex, map[string]any{
			"type": "message", "id": e.messageID(), "role": "assistant", "status": "completed",
			"content": []any{map[string]any{"type": "output_text", "text": e.text, "annotations": []any{}}}}})
	}
	for _, index := range sortedIntKeys(e.tools) {
		st := e.tools[index]
		if !st.hasOutputIndex {
			continue
		}
		callID := st.callID
		if callID == "" {
			callID = generatedToolCallID(index)
		}
		name := st.name
		if name == "" {
			name = "unknown"
		}
		items = append(items, ordered{st.outputIndex, map[string]any{
			"type": "function_call", "id": callID, "call_id": callID, "name": name,
			"arguments": st.arguments, "status": "completed"}})
	}
	for _, index := range sortedIntKeys(e.results) {
		st := e.results[index]
		if !st.hasOutputIndex {
			continue
		}
		itemID := st.toolUseID
		if itemID == "" {
			itemID = generatedToolCallID(index)
		}
		item := map[string]any{"type": "function_call_output",
			"id": itemID + "_output", "call_id": itemID, "output": st.content}
		if st.hasName && trimmed(st.name) != "" {
			item["name"] = st.name
		}
		items = append(items, ordered{st.outputIndex, item})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].idx < items[j].idx })
	output := make([]any, len(items))
	for i, it := range items {
		output[i] = it.item
	}
	u := usage
	if u == nil {
		u = &Usage{}
	}
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens
	}
	usagePayload := map[string]any{
		"input_tokens": u.InputTokens, "output_tokens": u.OutputTokens, "total_tokens": total}
	if u.ReasoningTokens > 0 {
		usagePayload["output_tokens_details"] = map[string]any{"reasoning_tokens": u.ReasoningTokens}
	}
	return map[string]any{
		"id": e.responseID(), "object": "response", "status": "completed",
		"model": e.model(), "output": output, "usage": usagePayload}
}

func (e *responsesEmitter) Finish() ([]byte, error) {
	if !e.started || e.finished {
		return nil, nil
	}
	return e.Emit(StreamFrame{ID: e.responseID(), Model: e.model(), Event: EvFinish, HasFinish: true})
}
