package formats

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Native Anthropic Messages requests require max_tokens. When OpenAI callers
// omit it, use a code- and long-form-friendly default while preserving an
// explicitly supplied client value.
const defaultClaudeMaxTokens uint64 = 64 * 1024

// Claude Messages（/v1/messages）的 parse/emit。
// 字段映射对照 Aether formats/claude/messages/*.rs + protocol/canonical.rs 的 claude_* helper。

// ----------------------------------------------------------------------------
// request
// ----------------------------------------------------------------------------

func claudeRequestFrom(body json.RawMessage) (*Request, bool) {
	obj, ok := decodeObject(body)
	if !ok {
		return nil, false
	}
	req := &Request{Model: getStr(obj, "model")}

	insts, ok := claudeSystemToInstructions(field(obj, "system"))
	if !ok {
		return nil, false
	}
	req.Instructions = insts
	var sys []string
	for _, in := range insts {
		if trimmed(in.Text) != "" {
			sys = append(sys, in.Text)
		}
	}
	if len(sys) > 0 {
		req.System = strings.Join(sys, "\n\n")
		req.HasSystem = true
	}

	msgs, ok := claudeMessagesToCanonical(field(obj, "messages"))
	if !ok {
		return nil, false
	}
	req.Messages = msgs
	req.Generation = claudeGenerationConfig(obj)

	tools, builtin, ok := claudeToolsToCanonical(field(obj, "tools"))
	if !ok {
		return nil, false
	}
	req.Tools = tools
	req.ToolChoice = claudeToolChoiceToCanonical(field(obj, "tool_choice"))
	req.ParallelToolCall = claudeParallelToolCalls(field(obj, "tool_choice"))
	req.Metadata = field(obj, "metadata")
	req.Thinking = claudeThinkingToCanonical(obj)
	req.Extensions = collectExtensions(obj, "claude",
		"model", "system", "messages", "max_tokens", "temperature", "top_p", "top_k",
		"stop", "stop_sequences", "stream", "tools", "tool_choice", "metadata",
		"thinking", "output_config")
	if len(builtin) > 0 {
		req.Extensions.setField("claude", "builtin_tools", mustRaw(builtin))
	}
	return req, true
}

func claudeRequestTo(req *Request, upstreamStream bool) json.RawMessage {
	out := map[string]any{"model": req.Model}
	out["messages"] = compactCanonicalClaudeMessages(canonicalMessagesToClaude(req))
	maxTok := defaultClaudeMaxTokens
	if req.Generation.MaxTokens != nil {
		maxTok = *req.Generation.MaxTokens
	}
	out["max_tokens"] = maxTok
	if sys := canonicalInstructionsToClaudeSystem(req.Instructions); sys != nil {
		out["system"] = sys
	} else if trimmed(req.System) != "" {
		out["system"] = req.System
	}
	if upstreamStream {
		out["stream"] = true
	}
	if req.Generation.Temperature != nil {
		out["temperature"] = *req.Generation.Temperature
	}
	if req.Generation.TopP != nil {
		out["top_p"] = *req.Generation.TopP
	}
	if req.Generation.TopK != nil {
		out["top_k"] = *req.Generation.TopK
	}
	if req.Generation.StopSequences != nil {
		out["stop_sequences"] = *req.Generation.StopSequences
	}
	if tools := canonicalToolsToClaude(req); len(tools) > 0 {
		out["tools"] = tools
	}
	if tc := canonicalToolChoiceToClaude(req.ToolChoice, req.ParallelToolCall); tc != nil {
		out["tool_choice"] = tc
	}
	if len(req.Metadata) > 0 {
		out["metadata"] = req.Metadata
	}
	if req.Thinking != nil {
		writeClaudeThinking(out, req)
	}
	emitExtensions(out, req.Extensions, "claude")
	return mustRaw(out)
}

// ----------------------------------------------------------------------------
// response
// ----------------------------------------------------------------------------

func claudeResponseFrom(body json.RawMessage) (*Response, bool) {
	obj, ok := decodeObject(body)
	if !ok || objErrorFieldIsPresent(obj) || getStr(obj, "type") == "error" {
		return nil, false
	}
	content, ok := claudeContentToCanonicalBlocks(field(obj, "content"))
	if !ok {
		return nil, false
	}
	sr := claudeStopReasonToCanonical(getStr(obj, "stop_reason"))
	resp := &Response{
		ID:    strOr(obj, "id", "msg-unknown"),
		Model: strOr(obj, "model", "unknown"),
		Outputs: []ResponseOutput{
			{Index: 0, Role: RoleAssistant, Content: content, StopReason: sr},
		},
		Content:    content,
		StopReason: sr,
		Usage:      claudeUsageToCanonical(field(obj, "usage")),
		Extensions: collectExtensions(obj, "claude",
			"id", "type", "role", "model", "content", "stop_reason", "stop_sequence", "usage"),
	}
	return resp, true
}

func claudeResponseTo(resp *Response) json.RawMessage {
	content := canonicalBlocksToClaude(resp.Content, RoleAssistant)
	if len(content) == 0 {
		content = []json.RawMessage{mustRaw(map[string]any{"type": "text", "text": ""})}
	}
	var usage json.RawMessage
	if resp.Usage != nil {
		usage = canonicalUsageToClaude(resp.Usage)
	} else {
		usage = mustRaw(map[string]any{"input_tokens": 0, "output_tokens": 0})
	}
	out := map[string]any{
		"id":          resp.ID,
		"type":        "message",
		"role":        "assistant",
		"model":       resp.Model,
		"content":     content,
		"stop_reason": canonicalStopReasonToClaude(resp.StopReason),
		"usage":       usage,
	}
	emitExtensions(out, resp.Extensions, "claude")
	return mustRaw(out)
}

// ----------------------------------------------------------------------------
// 内容块 parse（Claude → 规范化块）
// ----------------------------------------------------------------------------

func claudeSystemToInstructions(raw json.RawMessage) ([]Instruction, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if s, ok := asString(raw); ok {
		if trimmed(s) == "" {
			return nil, true
		}
		return []Instruction{{Role: RoleSystem, Text: s}}, true
	}
	blocks, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var insts []Instruction
	for _, b := range blocks {
		obj, ok := decodeObject(b)
		if !ok {
			return nil, false
		}
		bt := getStr(obj, "type")
		if bt != "" && bt != "text" {
			continue
		}
		text := getStr(obj, "text")
		if trimmed(text) != "" {
			insts = append(insts, Instruction{
				Role: RoleSystem, Text: text,
				Extensions: collectExtensions(obj, "claude", "type", "text"),
			})
		}
	}
	return insts, true
}

func claudeMessagesToCanonical(raw json.RawMessage) ([]Message, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	arr, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var out []Message
	for _, m := range arr {
		obj, ok := decodeObject(m)
		if !ok {
			return nil, false
		}
		blocks, ok := claudeContentToCanonicalBlocks(field(obj, "content"))
		if !ok {
			return nil, false
		}
		out = append(out, Message{
			Role:       roleFromString(getStr(obj, "role")),
			Content:    blocks,
			Extensions: collectExtensions(obj, "claude", "role", "content"),
		})
	}
	return out, true
}

func claudeContentToCanonicalBlocks(raw json.RawMessage) ([]ContentBlock, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	if s, ok := asString(raw); ok {
		if s == "" {
			return nil, true
		}
		return []ContentBlock{{Kind: BlockText, Text: s}}, true
	}
	arr, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var out []ContentBlock
	autoIdx := 0
	for _, b := range arr {
		blk, ok := claudeBlockToCanonical(b)
		if !ok {
			return nil, false
		}
		if blk.Kind == BlockToolUse && trimmed(blk.ID) == "" {
			blk.ID = "toolu_auto_" + strconv.Itoa(autoIdx)
			autoIdx++
		}
		out = append(out, blk)
	}
	return out, true
}

func claudeBlockToCanonical(raw json.RawMessage) (ContentBlock, bool) {
	obj, ok := decodeObject(raw)
	if !ok {
		return ContentBlock{}, false
	}
	switch normLower(getStr(obj, "type")) {
	case "text":
		return ContentBlock{Kind: BlockText, Text: getStr(obj, "text"),
			Extensions: collectExtensions(obj, "claude", "type", "text")}, true
	case "thinking":
		text := getStr(obj, "thinking")
		if text == "" {
			text = getStr(obj, "text")
		}
		return ContentBlock{Kind: BlockThinking, Text: text, Signature: getStr(obj, "signature"),
			Extensions: collectExtensions(obj, "claude", "type", "thinking", "text", "signature")}, true
	case "redacted_thinking":
		return ContentBlock{Kind: BlockThinking, EncryptedContent: getStr(obj, "data"),
			Extensions: collectExtensions(obj, "claude", "type", "data")}, true
	case "image":
		return claudeMediaBlockToCanonical(obj, true)
	case "document":
		return claudeMediaBlockToCanonical(obj, false)
	case "tool_use":
		input := field(obj, "input")
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		return ContentBlock{Kind: BlockToolUse, ID: getStr(obj, "id"), Name: getStr(obj, "name"),
			Input:      input,
			Extensions: collectExtensions(obj, "claude", "type", "id", "name", "input")}, true
	case "tool_result":
		content := field(obj, "content")
		ext := collectExtensions(obj, "claude", "type", "tool_use_id", "content", "is_error")
		ext.setField("aether", "source", rawString("claude_tool_result"))
		isErr, _ := asBool(field(obj, "is_error"))
		blk := ContentBlock{
			Kind:       BlockToolResult,
			ToolUseID:  getStr(obj, "tool_use_id"),
			IsError:    isErr,
			Extensions: ext,
		}
		if content != nil {
			blk.Output, blk.HasOutput = content, true
			blk.ContentText, blk.HasContentText = toolOutputText(content), true
		}
		return blk, true
	default:
		return ContentBlock{Kind: BlockUnknown, RawType: normLower(getStr(obj, "type")), Payload: raw}, true
	}
}

func claudeMediaBlockToCanonical(obj map[string]json.RawMessage, image bool) (ContentBlock, bool) {
	src, ok := decodeObject(field(obj, "source"))
	if !ok {
		return ContentBlock{}, false
	}
	srcType := getStr(src, "type")
	mediaType := getStr(src, "media_type")
	ext := collectExtensions(obj, "claude", "type", "source")
	switch {
	case image && srcType == "base64":
		return ContentBlock{Kind: BlockImage, Data: getStr(src, "data"), MediaType: mediaType, Extensions: ext}, true
	case image && srcType == "url":
		return ContentBlock{Kind: BlockImage, URL: getStr(src, "url"), Extensions: ext}, true
	case !image && srcType == "base64" && strings.HasPrefix(mediaType, "audio/"):
		return ContentBlock{Kind: BlockAudio, Data: getStr(src, "data"), MediaType: mediaType,
			Format: strings.TrimPrefix(mediaType, "audio/"), Extensions: ext}, true
	case !image && srcType == "base64":
		return ContentBlock{Kind: BlockFile, Data: getStr(src, "data"), MediaType: mediaType, Extensions: ext}, true
	case !image && srcType == "url":
		return ContentBlock{Kind: BlockFile, FileURL: getStr(src, "url"), Extensions: ext}, true
	default:
		return ContentBlock{Kind: BlockUnknown, RawType: getStr(obj, "type"), Payload: mustRaw(obj)}, true
	}
}

// ----------------------------------------------------------------------------
// 内容块 emit（规范化 → Claude）
// ----------------------------------------------------------------------------

func canonicalInstructionsToClaudeSystem(insts []Instruction) any {
	var filtered []Instruction
	for _, in := range insts {
		if trimmed(in.Text) != "" {
			filtered = append(filtered, in)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	var blocks []json.RawMessage
	structured := false
	var texts []string
	for _, in := range filtered {
		block := map[string]any{"type": "text", "text": in.Text}
		before := len(block)
		emitExtensions(block, in.Extensions, "claude")
		if len(block) > before {
			structured = true
		}
		blocks = append(blocks, mustRaw(block))
		texts = append(texts, in.Text)
	}
	if structured {
		return blocks
	}
	return strings.Join(texts, "\n\n")
}

func canonicalMessagesToClaude(req *Request) []json.RawMessage {
	var out []json.RawMessage
	for i := range req.Messages {
		msg := &req.Messages[i]
		var role string
		switch msg.Role {
		case RoleAssistant:
			role = "assistant"
		case RoleTool, RoleUser, RoleUnknown:
			role = "user"
		default: // system / developer
			continue
		}
		blocks := canonicalBlocksToClaude(msg.Content, msg.Role)
		if len(blocks) == 0 {
			continue
		}
		out = append(out, mustRaw(map[string]any{
			"role":    role,
			"content": simplifyCanonicalClaudeContent(blocks),
		}))
	}
	return out
}

func canonicalBlocksToClaude(blocks []ContentBlock, role Role) []json.RawMessage {
	var out []json.RawMessage
	for _, b := range blocks {
		if v, ok := canonicalBlockToClaude(b, role); ok {
			out = append(out, v)
		}
	}
	return out
}

func canonicalBlockToClaude(b ContentBlock, role Role) (json.RawMessage, bool) {
	switch b.Kind {
	case BlockText:
		if trimmed(b.Text) == "" {
			return nil, false
		}
		out := map[string]any{"type": "text", "text": b.Text}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	case BlockThinking:
		if b.EncryptedContent != "" {
			out := map[string]any{"type": "redacted_thinking", "data": b.EncryptedContent}
			emitExtensions(out, b.Extensions, "claude")
			return mustRaw(out), true
		}
		if role != RoleAssistant {
			if trimmed(b.Text) == "" {
				return nil, false
			}
			return mustRaw(map[string]any{"type": "text", "text": b.Text}), true
		}
		if trimmed(b.Text) == "" {
			return nil, false
		}
		out := map[string]any{"type": "thinking", "thinking": b.Text}
		if b.Signature != "" {
			out["signature"] = b.Signature
		}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	case BlockImage:
		if role == RoleAssistant {
			return mustRaw(map[string]any{"type": "text", "text": "[image]"}), true
		}
		src := claudeSourceValue(b.MediaType, b.Data, b.URL)
		if src == nil {
			return nil, false
		}
		out := map[string]any{"type": "image", "source": src}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	case BlockFile:
		if b.FileID != "" {
			return mustRaw(map[string]any{"type": "text", "text": "[File: " + b.FileID + "]"}), true
		}
		src := claudeSourceValue(b.MediaType, b.Data, b.FileURL)
		if src == nil {
			return nil, false
		}
		out := map[string]any{"type": "document", "source": src}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	case BlockAudio:
		mt := b.MediaType
		if mt == "" && b.Format != "" {
			mt = "audio/" + b.Format
		}
		src := claudeSourceValue(mt, b.Data, "")
		if src == nil {
			return nil, false
		}
		out := map[string]any{"type": "document", "source": src}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	case BlockToolUse:
		input := b.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		out := map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": input}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	case BlockToolResult:
		var content any
		if b.HasOutput && len(b.Output) > 0 {
			content = b.Output
		} else {
			content = b.ContentText
		}
		out := map[string]any{"type": "tool_result", "tool_use_id": b.ToolUseID,
			"content": content, "is_error": b.IsError}
		emitExtensions(out, b.Extensions, "claude")
		return mustRaw(out), true
	default:
		return nil, false
	}
}

func claudeSourceValue(mediaType, data, url string) map[string]any {
	if data != "" {
		mt := mediaType
		if mt == "" {
			mt = "application/octet-stream"
		}
		return map[string]any{"type": "base64", "media_type": mt, "data": data}
	}
	if url != "" {
		return map[string]any{"type": "url", "url": url}
	}
	return nil
}

// simplifyCanonicalClaudeContent 把仅含纯文本块（恰 type+text 两键）的内容折叠为字符串。
func simplifyCanonicalClaudeContent(blocks []json.RawMessage) any {
	if len(blocks) == 0 {
		return ""
	}
	var texts []string
	for _, b := range blocks {
		obj, ok := decodeObject(b)
		if !ok {
			return blocks
		}
		if len(obj) == 2 && getStr(obj, "type") == "text" {
			if t, ok := asString(field(obj, "text")); ok {
				texts = append(texts, t)
				continue
			}
		}
		return blocks
	}
	return strings.Join(texts, "\n")
}

// compactCanonicalClaudeMessages 合并相邻同角色消息；若首条是 assistant 则前置空 user。
func compactCanonicalClaudeMessages(messages []json.RawMessage) []json.RawMessage {
	var compact []json.RawMessage
	for _, m := range messages {
		obj, _ := decodeObject(m)
		role := getStr(obj, "role")
		if len(compact) > 0 {
			lastObj, _ := decodeObject(compact[len(compact)-1])
			if getStr(lastObj, "role") == role {
				compact[len(compact)-1] = mergeClaudeMessageContent(lastObj, obj)
				continue
			}
		}
		compact = append(compact, m)
	}
	if len(compact) > 0 {
		first, _ := decodeObject(compact[0])
		if getStr(first, "role") == "assistant" {
			compact = append([]json.RawMessage{mustRaw(map[string]any{"role": "user", "content": ""})}, compact...)
		}
	}
	return compact
}

func mergeClaudeMessageContent(target, incoming map[string]json.RawMessage) json.RawMessage {
	blocks := append(extractClaudeContentBlocks(field(target, "content")),
		extractClaudeContentBlocks(field(incoming, "content"))...)
	return mustRaw(map[string]any{
		"role":    getStr(target, "role"),
		"content": simplifyCanonicalClaudeContent(blocks),
	})
}

func extractClaudeContentBlocks(content json.RawMessage) []json.RawMessage {
	if s, ok := asString(content); ok {
		if s == "" {
			return nil
		}
		return []json.RawMessage{mustRaw(map[string]any{"type": "text", "text": s})}
	}
	if arr, ok := decodeArray(content); ok {
		return arr
	}
	return nil
}

// ----------------------------------------------------------------------------
// generation / tools / tool_choice / thinking / usage / stop（Claude）
// ----------------------------------------------------------------------------

func claudeGenerationConfig(req map[string]json.RawMessage) GenerationConfig {
	var g GenerationConfig
	if v, ok := asUint(field(req, "max_tokens")); ok {
		g.MaxTokens = &v
	}
	if v, ok := asFloat(field(req, "temperature")); ok {
		g.Temperature = &v
	}
	if v, ok := asFloat(field(req, "top_p")); ok {
		g.TopP = &v
	}
	if v, ok := asUint(field(req, "top_k")); ok {
		g.TopK = &v
	}
	if ss := openaiStopToVec(fieldOr(req, "stop", "stop_sequences")); ss != nil {
		g.StopSequences = &ss
	}
	return g
}

func claudeToolsToCanonical(raw json.RawMessage) (tools []ToolDefinition, builtin []json.RawMessage, ok bool) {
	if len(raw) == 0 {
		return nil, nil, true
	}
	arr, ok := decodeArray(raw)
	if !ok {
		return nil, nil, false
	}
	for _, t := range arr {
		obj, ok := decodeObject(t)
		if !ok {
			return nil, nil, false
		}
		tt := normLower(getStr(obj, "type"))
		if strings.HasPrefix(tt, "web_search") {
			builtin = append(builtin, t)
			continue
		}
		name := trimmed(getStr(obj, "name"))
		if name == "" {
			return nil, nil, false
		}
		tools = append(tools, ToolDefinition{
			Name:        name,
			Description: getStr(obj, "description"),
			Parameters:  field(obj, "input_schema"),
			Extensions:  collectExtensions(obj, "claude", "type", "name", "description", "input_schema"),
		})
	}
	return tools, builtin, true
}

func canonicalToolsToClaude(req *Request) []json.RawMessage {
	var tools []json.RawMessage
	for i := range req.Tools {
		t := &req.Tools[i]
		out := map[string]any{"name": t.Name}
		if t.Description != "" {
			out["description"] = t.Description
		}
		if len(t.Parameters) > 0 {
			out["input_schema"] = t.Parameters
		} else {
			out["input_schema"] = map[string]any{}
		}
		emitExtensions(out, t.Extensions, "claude")
		tools = append(tools, mustRaw(out))
	}
	if bucket := req.Extensions.get("claude"); bucket != nil {
		if arr, ok := decodeArray(bucket["builtin_tools"]); ok {
			tools = append(tools, arr...)
		}
	}
	return tools
}

func claudeToolChoiceToCanonical(raw json.RawMessage) *ToolChoice {
	if len(raw) == 0 {
		return nil
	}
	if s, ok := asString(raw); ok {
		switch normLower(s) {
		case "auto":
			return &ToolChoice{Kind: ToolChoiceAuto}
		case "any":
			return &ToolChoice{Kind: ToolChoiceRequired}
		case "none":
			return &ToolChoice{Kind: ToolChoiceNone}
		}
		return nil
	}
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	if name := getStr(obj, "name"); name != "" {
		return &ToolChoice{Kind: ToolChoiceTool, Name: name}
	}
	switch normLower(getStr(obj, "type")) {
	case "auto":
		return &ToolChoice{Kind: ToolChoiceAuto}
	case "any":
		return &ToolChoice{Kind: ToolChoiceRequired}
	case "none":
		return &ToolChoice{Kind: ToolChoiceNone}
	default:
		return nil
	}
}

func claudeParallelToolCalls(raw json.RawMessage) *bool {
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	if normLower(getStr(obj, "type")) == "none" {
		return nil
	}
	disable, ok := asBool(field(obj, "disable_parallel_tool_use"))
	if !ok {
		return nil
	}
	v := !disable
	return &v
}

func canonicalToolChoiceToClaude(tc *ToolChoice, parallel *bool) map[string]any {
	var out map[string]any
	switch {
	case tc == nil:
		if parallel == nil {
			return nil
		}
		out = map[string]any{"type": "auto"}
	case tc.Kind == ToolChoiceNone:
		out = map[string]any{"type": "none"}
	case tc.Kind == ToolChoiceRequired:
		out = map[string]any{"type": "any"}
	case tc.Kind == ToolChoiceAuto:
		out = map[string]any{"type": "auto"}
	case tc.Kind == ToolChoiceTool:
		out = map[string]any{"type": "tool", "name": tc.Name}
	}
	if parallel != nil && out["type"] != "none" {
		out["disable_parallel_tool_use"] = !*parallel
	}
	return out
}

func claudeThinkingToCanonical(req map[string]json.RawMessage) *ThinkingConfig {
	thinking, hasThinking := decodeObject(field(req, "thinking"))
	oc, hasOC := decodeObject(field(req, "output_config"))
	if !hasThinking && !hasOC {
		return nil
	}
	ext := Extensions{}
	if hasThinking {
		for k, v := range thinking {
			ext.setField("claude", k, v)
		}
	}
	if hasOC {
		ext.setField("claude", "output_config", field(req, "output_config"))
	}
	var effort string
	if hasOC {
		effort = claudeOutputEffortToOpenAI(getStr(oc, "effort"))
	}
	if effort == "" && hasThinking {
		if b, ok := asUint(field(thinking, "budget_tokens")); ok {
			effort = budgetToReasoningEffort(b)
		}
	}
	if effort != "" {
		ext.setField("openai", "reasoning_effort", rawString(effort))
	}
	enabled := true
	if hasThinking {
		if t, ok := asString(field(thinking, "type")); ok && t != "enabled" {
			enabled = false
		}
	}
	cfg := &ThinkingConfig{Enabled: enabled, Extensions: ext}
	if hasThinking {
		if b, ok := asUint(field(thinking, "budget_tokens")); ok {
			cfg.BudgetTokens = &b
		}
	}
	return cfg
}

func writeClaudeThinking(out map[string]any, req *Request) {
	t := req.Thinking
	effort := openaiReasoningEffort(t)
	var budget *uint64
	if t.BudgetTokens != nil {
		budget = t.BudgetTokens
	} else if effort != "" {
		if b, ok := reasoningEffortToBudget(effort); ok {
			budget = &b
		}
	}
	if t.Enabled || budget != nil {
		b := uint64(1024)
		if budget != nil {
			b = *budget
		}
		out["thinking"] = map[string]any{"type": "enabled", "budget_tokens": b}
	}
	if effort != "" {
		if oe := openaiEffortToClaudeOutput(effort); oe != "" {
			out["output_config"] = map[string]any{"effort": oe}
		}
	}
}

func claudeUsageToCanonical(raw json.RawMessage) *Usage {
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	input, _ := asUint(field(obj, "input_tokens"))
	output, _ := asUint(field(obj, "output_tokens"))
	u := &Usage{InputTokens: input, OutputTokens: output, TotalTokens: input + output}
	u.CacheReadTokens, _ = asUint(field(obj, "cache_read_input_tokens"))
	u.CacheWriteTokens, _ = asUint(field(obj, "cache_creation_input_tokens"))
	if cc, ok := decodeObject(field(obj, "cache_creation")); ok {
		u.CacheCreationEphemeral5mTok, _ = asUint(field(cc, "ephemeral_5m_input_tokens"))
		u.CacheCreationEphemeral1hTok, _ = asUint(field(cc, "ephemeral_1h_input_tokens"))
	}
	return u
}

func canonicalUsageToClaude(u *Usage) json.RawMessage {
	out := map[string]any{"input_tokens": u.InputTokens, "output_tokens": u.OutputTokens}
	if u.CacheReadTokens > 0 {
		out["cache_read_input_tokens"] = u.CacheReadTokens
	}
	if u.CacheWriteTokens > 0 {
		out["cache_creation_input_tokens"] = u.CacheWriteTokens
	}
	if u.CacheCreationEphemeral5mTok > 0 || u.CacheCreationEphemeral1hTok > 0 {
		out["cache_creation"] = map[string]any{
			"ephemeral_5m_input_tokens": u.CacheCreationEphemeral5mTok,
			"ephemeral_1h_input_tokens": u.CacheCreationEphemeral1hTok,
		}
	}
	return mustRaw(out)
}

func claudeStopReasonToCanonical(value string) *StopReason {
	if value == "" {
		return nil
	}
	var sr StopReason
	switch value {
	case "end_turn":
		sr = StopEndTurn
	case "max_tokens":
		sr = StopMaxTokens
	case "stop_sequence":
		sr = StopStopSequence
	case "tool_use":
		sr = StopToolUse
	case "pause_turn":
		sr = StopPauseTurn
	case "refusal":
		sr = StopRefusal
	case "content_filtered":
		sr = StopContentFiltered
	default:
		sr = StopUnknown
	}
	return &sr
}

func canonicalStopReasonToClaude(sr *StopReason) string {
	if sr == nil {
		return "end_turn"
	}
	switch *sr {
	case StopMaxTokens:
		return "max_tokens"
	case StopStopSequence:
		return "stop_sequence"
	case StopToolUse:
		return "tool_use"
	case StopPauseTurn:
		return "pause_turn"
	case StopRefusal:
		return "refusal"
	case StopContentFiltered:
		return "content_filtered"
	default:
		return "end_turn"
	}
}
