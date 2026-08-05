package formats

import (
	"encoding/json"
	"sort"
	"strings"
)

// OpenAI Chat Completions（/v1/chat/completions）的 parse/emit。
// 字段映射对照 Aether formats/openai/chat/*.rs + protocol/canonical.rs 的 openai_* helper。

// ----------------------------------------------------------------------------
// request
// ----------------------------------------------------------------------------

func openaiChatRequestFrom(body json.RawMessage) (*Request, bool) {
	obj, ok := decodeObject(body)
	if !ok {
		return nil, false
	}
	req := &Request{Model: getStr(obj, "model")}

	if messages, ok := decodeArray(field(obj, "messages")); ok {
		for _, m := range messages {
			msgObj, ok := decodeObject(m)
			if !ok {
				return nil, false
			}
			role := roleFromString(getStr(msgObj, "role"))
			if role == RoleSystem || role == RoleDeveloper {
				text := openaiContentText(field(msgObj, "content"))
				req.Instructions = append(req.Instructions, Instruction{
					Role:       role,
					Text:       text,
					Extensions: collectExtensions(msgObj, "openai", "role", "content"),
				})
				if trimmed(text) != "" {
					if trimmed(req.System) != "" {
						req.System += "\n\n" + text
					} else {
						req.System = text
					}
					req.HasSystem = true
				}
				continue
			}
			blocks, ok := openaiMessageContentBlocks(msgObj)
			if !ok {
				return nil, false
			}
			req.Messages = append(req.Messages, Message{
				Role:       role,
				Content:    blocks,
				Extensions: collectExtensions(msgObj, "openai", "role", "content", "tool_calls", "tool_call_id"),
			})
		}
	}

	req.Generation = openaiGenerationConfig(obj)
	tools, ok := openaiToolsToCanonical(field(obj, "tools"))
	if !ok {
		return nil, false
	}
	req.Tools = tools
	req.ToolChoice = openaiToolChoiceToCanonical(field(obj, "tool_choice"))
	if b, ok := asBool(field(obj, "parallel_tool_calls")); ok {
		req.ParallelToolCall = &b
	}
	req.Metadata = field(obj, "metadata")
	req.ResponseFormat = openaiResponseFormatToCanonical(field(obj, "response_format"))
	if effort, ok := asString(field(obj, "reasoning_effort")); ok {
		req.Thinking = &ThinkingConfig{
			Enabled:    true,
			Extensions: Extensions{"openai": {"reasoning_effort": rawString(effort)}},
		}
	}
	req.Extensions = collectExtensions(obj, "openai",
		"model", "messages", "max_tokens", "max_completion_tokens", "temperature",
		"top_p", "top_k", "stop", "stream", "tools", "tool_choice",
		"parallel_tool_calls", "metadata", "response_format", "reasoning_effort",
		"n", "presence_penalty", "frequency_penalty", "seed", "logprobs", "top_logprobs")
	return req, true
}

func openaiChatRequestTo(req *Request, upstreamStream bool) json.RawMessage {
	out := map[string]any{}
	if trimmed(req.Model) != "" {
		out["model"] = req.Model
	}

	var messages []json.RawMessage
	for _, inst := range req.Instructions {
		role := "system"
		if inst.Role == RoleDeveloper {
			role = "developer"
		}
		if trimmed(inst.Text) != "" {
			messages = append(messages, mustRaw(map[string]any{"role": role, "content": inst.Text}))
		}
	}
	for i := range req.Messages {
		messages = append(messages, canonicalMessageToOpenAIChatMessages(&req.Messages[i])...)
	}
	out["messages"] = messages

	writeOpenAIGenerationConfig(out, &req.Generation)
	if len(req.Tools) > 0 {
		tools := make([]json.RawMessage, 0, len(req.Tools))
		for i := range req.Tools {
			tools = append(tools, canonicalToolToOpenAI(&req.Tools[i]))
		}
		out["tools"] = tools
	}
	if req.ToolChoice != nil {
		out["tool_choice"] = canonicalToolChoiceToOpenAI(req.ToolChoice)
	}
	if req.ParallelToolCall != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCall
	}
	if len(req.Metadata) > 0 {
		out["metadata"] = req.Metadata
	}
	if req.ResponseFormat != nil {
		out["response_format"] = canonicalResponseFormatToOpenAI(req.ResponseFormat)
	}
	if req.Thinking != nil {
		if effort := openaiReasoningEffort(req.Thinking); effort != "" {
			out["reasoning_effort"] = effort
		}
	}
	emitExtensions(out, req.Extensions, "openai")
	if upstreamStream {
		out["stream"] = true
		out["stream_options"] = map[string]any{"include_usage": true}
	}
	return mustRaw(out)
}

// ----------------------------------------------------------------------------
// response
// ----------------------------------------------------------------------------

func openaiChatResponseFrom(body json.RawMessage) (*Response, bool) {
	obj, ok := decodeObject(body)
	if !ok || objErrorFieldIsPresent(obj) {
		return nil, false
	}
	choices, ok := decodeArray(field(obj, "choices"))
	if !ok {
		return nil, false
	}
	var outputs []ResponseOutput
	for i, c := range choices {
		choice, ok := decodeObject(c)
		if !ok {
			return nil, false
		}
		msg, ok := decodeObject(field(choice, "message"))
		if !ok {
			return nil, false
		}
		content, ok := openaiMessageContentBlocks(msg)
		if !ok {
			return nil, false
		}
		if !hasThinking(content) {
			if rc, ok := asString(field(msg, "reasoning_content")); ok && trimmed(rc) != "" {
				content = append([]ContentBlock{{Kind: BlockThinking, Text: rc}}, content...)
			}
		}
		idx := i
		if v, ok := asUint(field(choice, "index")); ok {
			idx = int(v)
		}
		outputs = append(outputs, ResponseOutput{
			Index:      idx,
			Role:       RoleAssistant,
			Content:    content,
			StopReason: openaiFinishReasonToCanonical(getStr(choice, "finish_reason")),
		})
	}
	if len(outputs) == 0 {
		return nil, false
	}
	resp := &Response{
		ID:         strOr(obj, "id", "chatcmpl-unknown"),
		Model:      strOr(obj, "model", "unknown"),
		Outputs:    outputs,
		Content:    outputs[0].Content,
		StopReason: outputs[0].StopReason,
		Usage:      openaiUsageToCanonical(field(obj, "usage")),
		Extensions: collectExtensions(obj, "openai", "id", "object", "model", "choices", "usage", "created"),
	}
	return resp, true
}

func openaiChatResponseTo(resp *Response) json.RawMessage {
	outputs := resp.Outputs
	if len(outputs) == 0 {
		outputs = []ResponseOutput{{Index: 0, Role: RoleAssistant, Content: resp.Content, StopReason: resp.StopReason}}
	}
	choices := make([]json.RawMessage, 0, len(outputs))
	for fallback, o := range outputs {
		idx := o.Index
		if o.Index == 0 && fallback != 0 {
			idx = fallback
		}
		choices = append(choices, mustRaw(map[string]any{
			"index":         idx,
			"message":       canonicalBlocksToOpenAIChatMessage(o.Content),
			"finish_reason": canonicalStopReasonToOpenAI(o.StopReason),
		}))
	}
	var usage json.RawMessage
	if resp.Usage != nil {
		usage = canonicalUsageToOpenAI(resp.Usage)
	} else {
		usage = mustRaw(map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0})
	}
	return mustRaw(map[string]any{
		"id":      resp.ID,
		"object":  "chat.completion",
		"model":   resp.Model,
		"choices": choices,
		"usage":   usage,
	})
}

// ----------------------------------------------------------------------------
// 内容块 parse（OpenAI message → 规范化块）
// ----------------------------------------------------------------------------

func openaiMessageContentBlocks(msg map[string]json.RawMessage) ([]ContentBlock, bool) {
	role := roleFromString(getStr(msg, "role"))
	var blocks []ContentBlock
	if role != RoleTool {
		b, ok := openaiContentToBlocks(field(msg, "content"))
		if !ok {
			return nil, false
		}
		blocks = b
	}
	if tcs, ok := decodeArray(field(msg, "tool_calls")); ok {
		for _, tc := range tcs {
			tcObj, ok := decodeObject(tc)
			if !ok {
				return nil, false
			}
			fn, ok := decodeObject(field(tcObj, "function"))
			if !ok {
				return nil, false
			}
			blocks = append(blocks, ContentBlock{
				Kind:       BlockToolUse,
				ID:         getStr(tcObj, "id"),
				Name:       getStr(fn, "name"),
				Input:      parseJSONish(field(fn, "arguments")),
				Extensions: collectExtensions(tcObj, "openai", "id", "type", "function"),
			})
		}
	}
	if role == RoleTool {
		text := openaiContentText(field(msg, "content"))
		content := field(msg, "content")
		var output json.RawMessage
		var hasOutput bool
		if s, ok := asString(content); ok {
			if json.Valid([]byte(s)) {
				output, hasOutput = json.RawMessage(s), true
			} else {
				output, hasOutput = content, true
			}
		} else if content != nil {
			output, hasOutput = content, true
		}
		ctext := text
		if ctext == "" && content != nil {
			ctext = toolOutputText(content)
		}
		blocks = append(blocks, ContentBlock{
			Kind:           BlockToolResult,
			ToolUseID:      getStr(msg, "tool_call_id"),
			Output:         output,
			HasOutput:      hasOutput,
			ContentText:    ctext,
			HasContentText: true,
			Extensions:     collectExtensions(msg, "openai", "role", "content", "tool_call_id"),
		})
	}
	return blocks, true
}

func openaiContentToBlocks(content json.RawMessage) ([]ContentBlock, bool) {
	if len(content) == 0 || string(content) == "null" {
		return nil, true
	}
	if s, ok := asString(content); ok {
		return []ContentBlock{{Kind: BlockText, Text: s}}, true
	}
	parts, ok := decodeArray(content)
	if !ok {
		return nil, false
	}
	var blocks []ContentBlock
	for _, p := range parts {
		b, ok := openaiPartToBlock(p)
		if !ok {
			return nil, false
		}
		blocks = append(blocks, b)
	}
	return blocks, true
}

func openaiPartToBlock(part json.RawMessage) (ContentBlock, bool) {
	obj, ok := decodeObject(part)
	if !ok {
		return ContentBlock{}, false
	}
	rawType := normLower(getStr(obj, "type"))
	switch rawType {
	case "text", "input_text", "output_text":
		return ContentBlock{Kind: BlockText, Text: getStr(obj, "text"),
			Extensions: collectExtensions(obj, "openai", "type", "text")}, true
	case "image_url", "input_image", "output_image":
		var url, detail string
		if img, ok := decodeObject(field(obj, "image_url")); ok {
			url = getStr(img, "url")
			detail = getStr(img, "detail")
		} else if s, ok := asString(field(obj, "image_url")); ok {
			url = s
		}
		if url == "" {
			url = getStr(obj, "url")
		}
		if detail == "" {
			detail = getStr(obj, "detail")
		}
		mt, data, u := splitDataURL(url, "")
		return ContentBlock{Kind: BlockImage, Data: data, URL: u, MediaType: mt, Detail: detail,
			Extensions: collectExtensions(obj, "openai", "type", "image_url", "url", "detail")}, true
	case "file", "input_file":
		fileObj := obj
		if f, ok := decodeObject(field(obj, "file")); ok {
			fileObj = f
		}
		src := getStr(fileObj, "file_data")
		if src == "" {
			src = getStr(fileObj, "file_url")
		}
		mt, data, fileURL := splitDataURL(src, getStr(fileObj, "mime_type"))
		return ContentBlock{Kind: BlockFile, Data: data, FileURL: fileURL, MediaType: mt,
			FileID: getStr(fileObj, "file_id"), Filename: getStr(fileObj, "filename"),
			Extensions: collectExtensions(obj, "openai", "type", "file")}, true
	case "input_audio":
		audio, ok := decodeObject(field(obj, "input_audio"))
		if !ok {
			return ContentBlock{}, false
		}
		format := getStr(audio, "format")
		mt := ""
		if format != "" {
			mt = "audio/" + format
		}
		return ContentBlock{Kind: BlockAudio, Data: getStr(audio, "data"), Format: format, MediaType: mt,
			Extensions: collectExtensions(obj, "openai", "type", "input_audio")}, true
	default:
		return ContentBlock{Kind: BlockUnknown, RawType: rawType, Payload: part}, true
	}
}

// ----------------------------------------------------------------------------
// 内容块 emit（规范化块 → OpenAI message）
// ----------------------------------------------------------------------------

// canonicalMessageToOpenAIChatMessages 把一条规范化消息 emit 成一条或多条 OpenAI
// chat 消息：tool_result 块各自拆成独立的 role=tool 消息，其余块合并。
func canonicalMessageToOpenAIChatMessages(msg *Message) []json.RawMessage {
	var out []json.RawMessage
	pendingStart := 0
	sawToolResult := false
	for i, b := range msg.Content {
		if b.Kind == BlockToolResult {
			sawToolResult = true
			if pendingStart < i {
				if v, ok := canonicalMessageBlocksToOpenAIChat(msg, msg.Content[pendingStart:i], false); ok {
					out = append(out, v)
				}
			}
			out = append(out, canonicalToolResultToOpenAIChat(b))
			pendingStart = i + 1
		}
	}
	if !sawToolResult {
		v, _ := canonicalMessageBlocksToOpenAIChat(msg, msg.Content, true)
		return []json.RawMessage{v}
	}
	if pendingStart < len(msg.Content) {
		if v, ok := canonicalMessageBlocksToOpenAIChat(msg, msg.Content[pendingStart:], false); ok {
			out = append(out, v)
		}
	}
	return out
}

func canonicalMessageBlocksToOpenAIChat(msg *Message, content []ContentBlock, includeEmpty bool) (json.RawMessage, bool) {
	roleStr := "user"
	switch msg.Role {
	case RoleAssistant:
		roleStr = "assistant"
	case RoleSystem:
		roleStr = "system"
	case RoleDeveloper:
		roleStr = "developer"
	case RoleTool:
		roleStr = "tool"
	}
	out := map[string]any{"role": roleStr}
	var contentParts []json.RawMessage
	var toolCalls []json.RawMessage
	var reasoningSegments []string
	var reasoningParts []json.RawMessage
	for _, b := range content {
		switch {
		case b.Kind == BlockThinking && msg.Role == RoleAssistant:
			if b.EncryptedContent != "" {
				reasoningParts = append(reasoningParts, mustRaw(map[string]any{
					"type": "redacted_thinking", "data": b.EncryptedContent}))
				continue
			}
			if trimmed(b.Text) != "" {
				omitContent := extBool(b.Extensions, "openai", "omit_reasoning_content")
				omitParts := extBool(b.Extensions, "openai", "omit_reasoning_parts")
				if !omitContent {
					reasoningSegments = append(reasoningSegments, b.Text)
				}
				if !omitParts {
					rp := map[string]any{"type": "thinking", "thinking": b.Text}
					if b.Signature != "" {
						rp["signature"] = b.Signature
					}
					reasoningParts = append(reasoningParts, mustRaw(rp))
				}
			}
		case b.Kind == BlockToolUse:
			toolCalls = append(toolCalls, mustRaw(map[string]any{
				"id":   b.ID,
				"type": "function",
				"function": map[string]any{
					"name":      b.Name,
					"arguments": canonicalizeToolArgs(b.Input),
				},
			}))
		case b.Kind == BlockToolResult:
			// 在上层拆分处理
		default:
			if part, ok := canonicalBlockToOpenAIPart(b); ok {
				contentParts = append(contentParts, part)
			}
		}
	}
	if !includeEmpty && len(contentParts) == 0 && len(toolCalls) == 0 &&
		len(reasoningSegments) == 0 && len(reasoningParts) == 0 {
		return nil, false
	}
	if len(toolCalls) > 0 && len(contentParts) == 0 {
		if len(reasoningParts) == 0 {
			out["content"] = []any{}
		} else {
			out["content"] = nil
		}
	} else {
		out["content"] = openaiContentValueFromParts(contentParts, false)
	}
	if len(toolCalls) > 0 {
		out["tool_calls"] = toolCalls
	}
	if len(reasoningSegments) > 0 {
		out["reasoning_content"] = strings.Join(reasoningSegments, "")
	}
	if len(reasoningParts) > 0 {
		out["reasoning_parts"] = reasoningParts
	}
	return mustRaw(out), true
}

// canonicalBlocksToOpenAIChatMessage 把响应输出块 emit 成单条 assistant 消息。
func canonicalBlocksToOpenAIChatMessage(content []ContentBlock) json.RawMessage {
	msg := &Message{Role: RoleAssistant, Content: content}
	v, _ := canonicalMessageBlocksToOpenAIChat(msg, content, true)
	return v
}

func canonicalToolResultToOpenAIChat(b ContentBlock) json.RawMessage {
	out := map[string]any{"role": "tool", "tool_call_id": b.ToolUseID}
	var content json.RawMessage
	if isClaudeToolResult(b.Extensions) {
		content = openaiChatToolResultContent(b)
		if b.IsError {
			content = openaiChatToolErrorContent(content)
		}
	} else if b.HasOutput && len(b.Output) > 0 {
		content = b.Output
	} else {
		content = rawString(b.ContentText)
	}
	out["content"] = content
	return mustRaw(out)
}

func openaiChatToolResultContent(b ContentBlock) json.RawMessage {
	if !b.HasOutput || len(b.Output) == 0 {
		return rawString(b.ContentText)
	}
	if s, ok := asString(b.Output); ok {
		return rawString(s)
	}
	if parts, ok := decodeArray(b.Output); ok {
		return anthropicToolResultBlocksToOpenAIChatContent(parts)
	}
	return rawString(compactJSON(b.Output))
}

func openaiChatToolErrorContent(content json.RawMessage) json.RawMessage {
	const prefix = "[tool error]"
	if s, ok := asString(content); ok {
		if s == "" {
			return rawString(prefix)
		}
		return rawString(prefix + " " + s)
	}
	return content
}

// anthropicToolResultBlocksToOpenAIChatContent 把 Anthropic 风格的 tool_result
// 内容块（text/image 数组）转成 OpenAI chat 可接受的内容；纯文本则合并为字符串。
func anthropicToolResultBlocksToOpenAIChatContent(parts []json.RawMessage) json.RawMessage {
	var texts []string
	allText := true
	for _, p := range parts {
		obj, ok := decodeObject(p)
		if !ok {
			allText = false
			break
		}
		if normLower(getStr(obj, "type")) == "text" {
			texts = append(texts, getStr(obj, "text"))
		} else {
			allText = false
			break
		}
	}
	if allText {
		return rawString(strings.Join(texts, "\n\n"))
	}
	return mustRaw(parts)
}

// ----------------------------------------------------------------------------
// generation / tools / tool_choice / response_format / usage / stop（OpenAI）
// ----------------------------------------------------------------------------

func openaiGenerationConfig(req map[string]json.RawMessage) GenerationConfig {
	var g GenerationConfig
	if v, ok := asUint(fieldOr(req, "max_completion_tokens", "max_tokens")); ok {
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
	if ss := openaiStopToVec(field(req, "stop")); ss != nil {
		g.StopSequences = &ss
	}
	if v, ok := asUint(field(req, "n")); ok {
		g.N = &v
	}
	if v, ok := asFloat(field(req, "presence_penalty")); ok {
		g.PresencePenalty = &v
	}
	if v, ok := asFloat(field(req, "frequency_penalty")); ok {
		g.FrequencyPenalty = &v
	}
	if v, ok := asInt(field(req, "seed")); ok {
		g.Seed = &v
	}
	if v, ok := asBool(field(req, "logprobs")); ok {
		g.Logprobs = &v
	}
	if v, ok := asUint(field(req, "top_logprobs")); ok {
		g.TopLogprobs = &v
	}
	return g
}

func writeOpenAIGenerationConfig(out map[string]any, g *GenerationConfig) {
	if g.MaxTokens != nil {
		out["max_completion_tokens"] = *g.MaxTokens
	}
	if g.Temperature != nil {
		out["temperature"] = *g.Temperature
	}
	if g.TopP != nil {
		out["top_p"] = *g.TopP
	}
	if g.TopK != nil {
		out["top_k"] = *g.TopK
	}
	if g.StopSequences != nil {
		ss := *g.StopSequences
		if len(ss) == 1 {
			out["stop"] = ss[0]
		} else {
			out["stop"] = ss
		}
	}
	if g.N != nil {
		out["n"] = *g.N
	}
	if g.PresencePenalty != nil {
		out["presence_penalty"] = *g.PresencePenalty
	}
	if g.FrequencyPenalty != nil {
		out["frequency_penalty"] = *g.FrequencyPenalty
	}
	if g.Seed != nil {
		out["seed"] = *g.Seed
	}
	if g.Logprobs != nil {
		out["logprobs"] = *g.Logprobs
	}
	if g.TopLogprobs != nil {
		out["top_logprobs"] = *g.TopLogprobs
	}
}

func openaiStopToVec(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	if s, ok := asString(raw); ok {
		return []string{s}
	}
	if arr, ok := decodeArray(raw); ok {
		var out []string
		for _, v := range arr {
			if s, ok := asString(v); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func openaiToolsToCanonical(raw json.RawMessage) ([]ToolDefinition, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	arr, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var out []ToolDefinition
	for _, t := range arr {
		tObj, ok := decodeObject(t)
		if !ok {
			return nil, false
		}
		fn, ok := decodeObject(field(tObj, "function"))
		if !ok {
			return nil, false
		}
		name, ok := asString(field(fn, "name"))
		if !ok {
			return nil, false
		}
		out = append(out, ToolDefinition{
			Name:        name,
			Description: getStr(fn, "description"),
			Parameters:  field(fn, "parameters"),
			Extensions:  collectExtensions(tObj, "openai", "type", "function"),
		})
	}
	return out, true
}

func canonicalToolToOpenAI(t *ToolDefinition) json.RawMessage {
	fn := map[string]any{"name": t.Name}
	if t.Description != "" {
		fn["description"] = t.Description
	}
	if len(t.Parameters) > 0 {
		fn["parameters"] = t.Parameters
	}
	return mustRaw(map[string]any{"type": "function", "function": fn})
}

func openaiToolChoiceToCanonical(raw json.RawMessage) *ToolChoice {
	if len(raw) == 0 {
		return nil
	}
	if s, ok := asString(raw); ok {
		switch s {
		case "auto":
			return &ToolChoice{Kind: ToolChoiceAuto}
		case "none":
			return &ToolChoice{Kind: ToolChoiceNone}
		case "required":
			return &ToolChoice{Kind: ToolChoiceRequired}
		}
		return nil
	}
	if obj, ok := decodeObject(raw); ok {
		if fn, ok := decodeObject(field(obj, "function")); ok {
			if name, ok := asString(field(fn, "name")); ok {
				return &ToolChoice{Kind: ToolChoiceTool, Name: name}
			}
		}
	}
	return nil
}

func canonicalToolChoiceToOpenAI(tc *ToolChoice) any {
	switch tc.Kind {
	case ToolChoiceAuto:
		return "auto"
	case ToolChoiceNone:
		return "none"
	case ToolChoiceRequired:
		return "required"
	case ToolChoiceTool:
		return map[string]any{"type": "function", "function": map[string]any{"name": tc.Name}}
	}
	return "auto"
}

func openaiResponseFormatToCanonical(raw json.RawMessage) *ResponseFormat {
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	ft := getStr(obj, "type")
	if ft == "" {
		ft = "text"
	}
	return &ResponseFormat{
		FormatType: ft,
		JSONSchema: field(obj, "json_schema"),
		Extensions: collectExtensions(obj, "openai", "type", "json_schema"),
	}
}

func canonicalResponseFormatToOpenAI(rf *ResponseFormat) json.RawMessage {
	out := map[string]any{"type": rf.FormatType}
	if len(rf.JSONSchema) > 0 {
		out["json_schema"] = rf.JSONSchema
	}
	return mustRaw(out)
}

func openaiUsageToCanonical(raw json.RawMessage) *Usage {
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	input, _ := asUint(fieldOr(obj, "prompt_tokens", "input_tokens"))
	output, _ := asUint(fieldOr(obj, "completion_tokens", "output_tokens"))
	u := &Usage{InputTokens: input, OutputTokens: output}
	if details, ok := decodeObject(fieldOr(obj, "completion_tokens_details", "output_tokens_details")); ok {
		u.ReasoningTokens, _ = asUint(field(details, "reasoning_tokens"))
	}
	if details, ok := decodeObject(fieldOr(obj, "prompt_tokens_details", "input_tokens_details")); ok {
		u.CacheReadTokens, _ = asUint(field(details, "cached_tokens"))
		u.CacheWriteTokens, _ = asUint(fieldOr(details, "cached_creation_tokens", "cache_creation_tokens"))
	}
	if total, ok := asUint(field(obj, "total_tokens")); ok {
		u.TotalTokens = total
	} else {
		u.TotalTokens = input + output
	}
	return u
}

func canonicalUsageToOpenAI(u *Usage) json.RawMessage {
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens
	}
	out := map[string]any{
		"prompt_tokens":     u.InputTokens,
		"completion_tokens": u.OutputTokens,
		"total_tokens":      total,
	}
	if u.ReasoningTokens > 0 {
		out["completion_tokens_details"] = map[string]any{"reasoning_tokens": u.ReasoningTokens}
	}
	promptDetails := map[string]any{}
	if u.CacheReadTokens > 0 {
		promptDetails["cached_tokens"] = u.CacheReadTokens
	}
	if u.CacheWriteTokens > 0 {
		promptDetails["cached_creation_tokens"] = u.CacheWriteTokens
	}
	if len(promptDetails) > 0 {
		out["prompt_tokens_details"] = promptDetails
	}
	return mustRaw(out)
}

func openaiFinishReasonToCanonical(value string) *StopReason {
	if value == "" {
		return nil
	}
	var sr StopReason
	switch value {
	case "stop":
		sr = StopEndTurn
	case "length":
		sr = StopMaxTokens
	case "tool_calls", "function_call":
		sr = StopToolUse
	case "content_filter":
		sr = StopContentFiltered
	default:
		sr = StopUnknown
	}
	return &sr
}

func canonicalStopReasonToOpenAI(sr *StopReason) string {
	if sr == nil {
		return "stop"
	}
	switch *sr {
	case StopMaxTokens:
		return "length"
	case StopToolUse:
		return "tool_calls"
	case StopContentFiltered, StopRefusal:
		return "content_filter"
	default:
		return "stop"
	}
}

// ----------------------------------------------------------------------------
// 小工具
// ----------------------------------------------------------------------------

func trimmed(s string) string { return strings.TrimSpace(s) }

func strOr(obj map[string]json.RawMessage, key, def string) string {
	if s, ok := asString(field(obj, key)); ok {
		return s
	}
	return def
}

func hasThinking(blocks []ContentBlock) bool {
	for _, b := range blocks {
		if b.Kind == BlockThinking {
			return true
		}
	}
	return false
}

func isClaudeToolResult(ext Extensions) bool {
	bucket := ext.get("aether")
	if bucket == nil {
		return false
	}
	s, _ := asString(bucket["source"])
	return s == "claude_tool_result"
}

func extBool(ext Extensions, ns, key string) bool {
	bucket := ext.get(ns)
	if bucket == nil {
		return false
	}
	b, _ := asBool(bucket[key])
	return b
}

func extString(ext Extensions, ns, key string) string {
	bucket := ext.get(ns)
	if bucket == nil {
		return ""
	}
	s, _ := asString(bucket[key])
	return s
}

// openaiReasoningEffort 从 thinking 扩展里取 OpenAI reasoning_effort（chat 与
// responses 命名空间都看）。
func openaiReasoningEffort(t *ThinkingConfig) string {
	if s := extString(t.Extensions, "openai", "reasoning_effort"); s != "" {
		return s
	}
	if s := extString(t.Extensions, "openai_responses", "effort"); s != "" {
		return s
	}
	return ""
}

// sortedKeys 返回 map 的有序键（确定性遍历）。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
