package formats

import (
	"encoding/json"
	"strconv"
	"strings"
)

// OpenAI Responses（/v1/responses）的 parse/emit。
// 字段映射对照 Aether formats/openai/responses/*.rs 及相关 openai_responses_* helper。
// 私有字段命名空间为 "openai_responses"（与 chat 的 "openai" 区分）。

const nsResponses = "openai_responses"

// ----------------------------------------------------------------------------
// request
// ----------------------------------------------------------------------------

func openaiResponsesRequestFrom(body json.RawMessage) (*Request, bool) {
	obj, ok := decodeObject(body)
	if !ok {
		return nil, false
	}
	req := &Request{Model: getStr(obj, "model")}

	if instr := field(obj, "instructions"); instr != nil {
		text := openaiContentText(instr)
		if trimmed(text) != "" {
			req.System, req.HasSystem = text, true
			req.Instructions = append(req.Instructions, Instruction{Role: RoleSystem, Text: text})
		}
	}
	msgs, ok := responsesInputToMessages(field(obj, "input"))
	if !ok {
		return nil, false
	}
	req.Messages = msgs

	req.Generation = openaiGenerationConfig(obj)
	req.Generation.MaxTokens = nil
	if v, ok := asUint(field(obj, "max_output_tokens")); ok {
		req.Generation.MaxTokens = &v
	}

	tools, ok := responsesToolsToCanonical(field(obj, "tools"))
	if !ok {
		return nil, false
	}
	req.Tools = tools
	req.ToolChoice = responsesToolChoiceToCanonical(field(obj, "tool_choice"))
	if b, ok := asBool(field(obj, "parallel_tool_calls")); ok {
		req.ParallelToolCall = &b
	}
	req.Metadata = field(obj, "metadata")
	if textObj, ok := decodeObject(field(obj, "text")); ok {
		req.ResponseFormat = openaiResponseFormatToCanonical(field(textObj, "format"))
	}
	if reasoning, ok := decodeObject(field(obj, "reasoning")); ok {
		ext := Extensions{}
		for k, v := range reasoning {
			ext.setField(nsResponses, k, v)
		}
		tc := &ThinkingConfig{Enabled: true, Extensions: ext}
		if b, ok := asUint(field(reasoning, "budget_tokens")); ok {
			tc.BudgetTokens = &b
		}
		req.Thinking = tc
	}
	// 顶层未知字段归档到 openai_responses 命名空间（对应 Aether 的 openai→openai_responses 重命名）
	req.Extensions = collectExtensions(obj, nsResponses,
		"model", "instructions", "input", "max_output_tokens", "temperature", "top_p",
		"metadata", "tools", "tool_choice", "parallel_tool_calls", "text", "reasoning")
	if textObj, ok := decodeObject(field(obj, "text")); ok {
		if v := field(textObj, "verbosity"); v != nil {
			req.Extensions.setField(nsResponses, "verbosity", v)
		}
	}
	return req, true
}

func openaiResponsesRequestTo(req *Request, upstreamStream bool) json.RawMessage {
	out := map[string]any{"model": req.Model}
	if instr := canonicalInstructionsToResponses(req); instr != "" {
		out["instructions"] = instr
	}
	out["input"] = canonicalMessagesToResponsesInput(req)
	if upstreamStream {
		out["stream"] = true
	}
	if req.Generation.MaxTokens != nil {
		out["max_output_tokens"] = *req.Generation.MaxTokens
	}
	if req.Generation.Temperature != nil {
		out["temperature"] = *req.Generation.Temperature
	}
	if req.Generation.TopP != nil {
		out["top_p"] = *req.Generation.TopP
	}
	if req.Generation.TopLogprobs != nil {
		out["top_logprobs"] = *req.Generation.TopLogprobs
	}
	if req.ParallelToolCall != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCall
	}
	if len(req.Metadata) > 0 {
		out["metadata"] = req.Metadata
	}
	if tc := canonicalTextConfigToResponses(req); tc != nil {
		out["text"] = tc
	}
	if len(req.Tools) > 0 {
		out["tools"] = canonicalToolsToResponses(req)
	}
	if req.ToolChoice != nil {
		out["tool_choice"] = canonicalToolChoiceToResponses(req.ToolChoice)
	}
	if req.Thinking != nil {
		if r := reasoningConfigToResponses(req.Thinking); r != nil {
			out["reasoning"] = r
		}
	}
	emitExtensions(out, req.Extensions, nsResponses)
	delete(out, "verbosity")
	return mustRaw(out)
}

// ----------------------------------------------------------------------------
// response
// ----------------------------------------------------------------------------

func openaiResponsesResponseFrom(body json.RawMessage) (*Response, bool) {
	obj, ok := decodeObject(body)
	if !ok {
		return nil, false
	}
	if objErrorFieldIsPresent(obj) {
		return nil, false
	}
	if getStr(obj, "status") == "failed" {
		return nil, false
	}
	content, ok := responsesOutputToBlocks(field(obj, "output"))
	if !ok {
		return nil, false
	}
	var sr StopReason
	if hasToolUse(content) {
		sr = StopToolUse
	} else {
		switch getStr(obj, "status") {
		case "incomplete":
			sr = StopMaxTokens
		case "failed":
			sr = StopUnknown
		default:
			sr = StopEndTurn
		}
	}
	resp := &Response{
		ID:         strOr(obj, "id", "resp-unknown"),
		Model:      strOr(obj, "model", "unknown"),
		Outputs:    []ResponseOutput{{Index: 0, Role: RoleAssistant, Content: content, StopReason: &sr}},
		Content:    content,
		StopReason: &sr,
		Usage:      openaiUsageToCanonical(field(obj, "usage")),
		Extensions: collectExtensions(obj, nsResponses, "id", "object", "model", "output", "usage", "status"),
	}
	return resp, true
}

func openaiResponsesResponseTo(resp *Response) json.RawMessage {
	responseID := strings.ReplaceAll(resp.ID, "chatcmpl", "resp")
	out := map[string]any{
		"id":     responseID,
		"object": "response",
		"status": "completed",
		"model":  resp.Model,
	}
	var output []json.RawMessage
	var messageContent []json.RawMessage
	msgIndex := 0
	flush := func() {
		if len(messageContent) == 0 {
			return
		}
		id := responseID + "_msg"
		if msgIndex != 0 {
			id = responseID + "_msg_" + strconv.Itoa(msgIndex)
		}
		output = append(output, mustRaw(map[string]any{
			"type": "message", "id": id, "role": "assistant", "status": "completed",
			"content": coalesceResponsesText(messageContent),
		}))
		messageContent = nil
		msgIndex++
	}
	for _, b := range resp.Content {
		switch b.Kind {
		case BlockText, BlockImage, BlockFile, BlockAudio:
			if p, ok := canonicalBlockToResponsesPart(b); ok {
				messageContent = append(messageContent, p)
			}
		case BlockThinking:
			flush()
			item := map[string]any{
				"type": "reasoning", "id": responseID + "_rs_" + strconv.Itoa(len(output)),
				"status": "completed",
			}
			if b.EncryptedContent != "" {
				item["encrypted_content"] = b.EncryptedContent
			}
			if trimmed(b.Text) != "" {
				item["summary"] = []any{map[string]any{"type": "summary_text", "text": b.Text}}
			}
			output = append(output, mustRaw(item))
		case BlockToolUse:
			flush()
			output = append(output, mustRaw(map[string]any{
				"type": "function_call", "id": b.ID, "call_id": b.ID,
				"name": b.Name, "arguments": canonicalizeToolArgs(b.Input),
			}))
		case BlockToolResult:
			flush()
			item := map[string]any{"type": "function_call_output", "call_id": b.ToolUseID}
			if b.HasOutput && len(b.Output) > 0 {
				item["output"] = b.Output
			} else {
				item["output"] = b.ContentText
			}
			if b.IsError {
				item["is_error"] = true
			}
			output = append(output, mustRaw(item))
		}
	}
	flush()
	out["output"] = output
	if resp.Usage != nil {
		out["usage"] = canonicalUsageToResponsesUsage(resp.Usage)
	}
	emitExtensions(out, resp.Extensions, nsResponses)
	return mustRaw(out)
}

// ----------------------------------------------------------------------------
// input parse（Responses input items → 规范化消息）
// ----------------------------------------------------------------------------

func responsesInputToMessages(raw json.RawMessage) ([]Message, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	if s, ok := asString(raw); ok {
		if trimmed(s) == "" {
			return nil, true
		}
		return []Message{{Role: RoleUser, Content: []ContentBlock{{Kind: BlockText, Text: s}}}}, true
	}
	items, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var msgs []Message
	autoIdx := 0
	for _, it := range items {
		if s, ok := asString(it); ok {
			if trimmed(s) != "" {
				msgs = append(msgs, Message{Role: RoleUser, Content: []ContentBlock{{Kind: BlockText, Text: s}}})
			}
			continue
		}
		obj, ok := decodeObject(it)
		if !ok {
			return nil, false
		}
		switch normLower(strOr(obj, "type", "message")) {
		case "message":
			role := roleFromString(strOr(obj, "role", "user"))
			if role == RoleSystem || role == RoleDeveloper {
				text := openaiContentText(field(obj, "content"))
				if trimmed(text) != "" {
					msgs = append(msgs, Message{Role: role,
						Content:    []ContentBlock{{Kind: BlockText, Text: text}},
						Extensions: collectExtensions(obj, nsResponses, "type", "role", "content")})
				}
				continue
			}
			blocks, ok := responsesContentToBlocks(field(obj, "content"))
			if !ok {
				return nil, false
			}
			msgs = append(msgs, Message{Role: role, Content: blocks,
				Extensions: collectExtensions(obj, nsResponses, "type", "role", "content")})
		case "function_call":
			name := trimmed(getStr(obj, "name"))
			if name == "" {
				return nil, false
			}
			id := trimmed(strFirst(obj, "call_id", "id"))
			if id == "" {
				id = "call_auto_" + strconv.Itoa(autoIdx)
				autoIdx++
			}
			msgs = append(msgs, Message{Role: RoleAssistant, Content: []ContentBlock{{
				Kind: BlockToolUse, ID: id, Name: name, Input: parseJSONish(field(obj, "arguments")),
				Extensions: collectExtensions(obj, nsResponses, "type", "call_id", "id", "name", "arguments"),
			}}})
		case "function_call_output":
			id := trimmed(strFirst(obj, "call_id", "tool_call_id", "id"))
			if id == "" {
				id = "call_auto_" + strconv.Itoa(autoIdx)
				autoIdx++
			}
			rawOut := field(obj, "output")
			isErr, _ := asBool(field(obj, "is_error"))
			blk := ContentBlock{Kind: BlockToolResult, ToolUseID: id, IsError: isErr,
				Output: parseJSONish(rawOut), HasOutput: true,
				Extensions: collectExtensions(obj, nsResponses, "type", "call_id", "tool_call_id", "id", "output", "is_error")}
			if rawOut != nil {
				blk.ContentText, blk.HasContentText = toolOutputText(rawOut), true
			}
			msgs = append(msgs, Message{Role: RoleTool, Content: []ContentBlock{blk}})
		default:
			msgs = append(msgs, Message{Role: RoleUnknown, Content: []ContentBlock{{
				Kind: BlockUnknown, RawType: normLower(getStr(obj, "type")), Payload: it}}})
		}
	}
	return msgs, true
}

func responsesContentToBlocks(raw json.RawMessage) ([]ContentBlock, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	if s, ok := asString(raw); ok {
		return []ContentBlock{{Kind: BlockText, Text: s}}, true
	}
	parts, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var blocks []ContentBlock
	for _, p := range parts {
		b, ok := responsesPartToBlock(p)
		if !ok {
			return nil, false
		}
		blocks = append(blocks, b)
	}
	return blocks, true
}

func responsesPartToBlock(part json.RawMessage) (ContentBlock, bool) {
	obj, ok := decodeObject(part)
	if !ok {
		return ContentBlock{}, false
	}
	switch normLower(getStr(obj, "type")) {
	case "input_text", "output_text", "text":
		return ContentBlock{Kind: BlockText, Text: getStr(obj, "text"),
			Extensions: collectExtensions(obj, nsResponses, "type", "text")}, true
	case "reasoning", "thinking":
		ext := collectExtensions(obj, nsResponses, "type", "text", "summary", "signature", "encrypted_content")
		ext.setField("openai", "omit_reasoning_parts", rawString("true")) // 占位，emit 时按需读取
		ext.ensure("openai")["omit_reasoning_parts"] = mustRaw(true)
		text := getStr(obj, "text")
		if text == "" {
			text = getStr(obj, "summary")
		}
		return ContentBlock{Kind: BlockThinking, Text: text,
			Signature: getStr(obj, "signature"), EncryptedContent: getStr(obj, "encrypted_content"),
			Extensions: ext}, true
	case "input_image", "output_image", "image_url":
		var url, detail string
		if img, ok := decodeObject(field(obj, "image_url")); ok {
			url, detail = getStr(img, "url"), getStr(img, "detail")
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
			Extensions: collectExtensions(obj, nsResponses, "type", "image_url", "url", "detail")}, true
	case "input_file", "file":
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
			Extensions: collectExtensions(obj, nsResponses, "type", "file_data", "file_url", "mime_type", "file_id", "filename")}, true
	case "input_audio", "audio":
		audioObj := obj
		if a, ok := decodeObject(field(obj, "input_audio")); ok {
			audioObj = a
		}
		format := getStr(audioObj, "format")
		mt := getStr(audioObj, "media_type")
		if mt == "" && format != "" {
			mt = "audio/" + format
		}
		return ContentBlock{Kind: BlockAudio, Data: getStr(audioObj, "data"), Format: format, MediaType: mt,
			Extensions: collectExtensions(obj, nsResponses, "type", "input_audio")}, true
	default:
		return ContentBlock{Kind: BlockUnknown, RawType: normLower(getStr(obj, "type")), Payload: part}, true
	}
}

func responsesOutputToBlocks(raw json.RawMessage) ([]ContentBlock, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	items, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var blocks []ContentBlock
	for index, it := range items {
		obj, ok := decodeObject(it)
		if !ok {
			blocks = append(blocks, ContentBlock{Kind: BlockUnknown, Payload: it})
			continue
		}
		itemType := normLower(getStr(obj, "type"))
		switch itemType {
		case "message":
			b, ok := responsesContentToBlocks(field(obj, "content"))
			if !ok {
				return nil, false
			}
			blocks = append(blocks, b...)
		case "reasoning":
			emitted := false
			if summary, ok := decodeArray(field(obj, "summary")); ok {
				for _, s := range summary {
					sObj, ok := decodeObject(s)
					if !ok {
						continue
					}
					text := getStr(sObj, "text")
					if trimmed(text) == "" {
						continue
					}
					ext := collectExtensions(obj, nsResponses, "type", "id", "status", "summary", "encrypted_content")
					ext.ensure("openai")["omit_reasoning_parts"] = mustRaw(true)
					blocks = append(blocks, ContentBlock{Kind: BlockThinking, Text: text,
						EncryptedContent: getStr(obj, "encrypted_content"), Extensions: ext})
					emitted = true
				}
			}
			if !emitted {
				blocks = append(blocks, ContentBlock{Kind: BlockUnknown, RawType: itemType, Payload: it})
			}
		case "function_call":
			name := trimmed(getStr(obj, "name"))
			if name == "" {
				return nil, false
			}
			id := trimmed(strFirst(obj, "call_id", "id"))
			if id == "" {
				id = "call_auto_" + strconv.Itoa(index)
			}
			blocks = append(blocks, ContentBlock{Kind: BlockToolUse, ID: id, Name: name,
				Input:      parseJSONish(field(obj, "arguments")),
				Extensions: collectExtensions(obj, nsResponses, "type", "id", "call_id", "name", "arguments", "status")})
		case "function_call_output":
			id := trimmed(strFirst(obj, "call_id", "tool_call_id", "id"))
			if id == "" {
				id = "call_auto_" + strconv.Itoa(index)
			}
			rawOut := field(obj, "output")
			isErr, _ := asBool(field(obj, "is_error"))
			blk := ContentBlock{Kind: BlockToolResult, ToolUseID: id, IsError: isErr,
				Output: parseJSONish(rawOut), HasOutput: true,
				Extensions: collectExtensions(obj, nsResponses, "type", "id", "call_id", "tool_call_id", "output", "is_error")}
			if rawOut != nil {
				blk.ContentText, blk.HasContentText = toolOutputText(rawOut), true
			}
			blocks = append(blocks, blk)
		case "output_text", "text", "output_image", "image_url", "file", "input_file", "input_audio":
			b, ok := responsesPartToBlock(it)
			if !ok {
				return nil, false
			}
			blocks = append(blocks, b)
		default:
			blocks = append(blocks, ContentBlock{Kind: BlockUnknown, RawType: itemType, Payload: it})
		}
	}
	return blocks, true
}

// ----------------------------------------------------------------------------
// input/output emit（规范化 → Responses）
// ----------------------------------------------------------------------------

func canonicalInstructionsToResponses(req *Request) string {
	var texts []string
	for _, in := range req.Instructions {
		if trimmed(in.Text) != "" {
			texts = append(texts, in.Text)
		}
	}
	if t := strings.Join(texts, "\n\n"); trimmed(t) != "" {
		return t
	}
	if trimmed(req.System) != "" {
		return req.System
	}
	return ""
}

func canonicalMessagesToResponsesInput(req *Request) []json.RawMessage {
	var input []json.RawMessage
	for i := range req.Messages {
		msg := &req.Messages[i]
		var role string
		switch msg.Role {
		case RoleAssistant:
			role = "assistant"
		case RoleSystem, RoleDeveloper:
			continue
		default:
			role = "user"
		}
		var content []json.RawMessage
		flush := func() {
			if len(content) == 0 {
				return
			}
			input = append(input, mustRaw(map[string]any{"type": "message", "role": role, "content": content}))
			content = nil
		}
		for _, b := range msg.Content {
			switch b.Kind {
			case BlockToolUse:
				flush()
				input = append(input, mustRaw(map[string]any{
					"type": "function_call", "call_id": b.ID, "name": b.Name,
					"arguments": canonicalizeToolArgs(b.Input)}))
			case BlockToolResult:
				flush()
				input = append(input, mustRaw(map[string]any{
					"type": "function_call_output", "call_id": b.ToolUseID,
					"output": responsesToolResultOutput(b)}))
			case BlockThinking:
				// responses input 不带思考块
			default:
				if p, ok := canonicalBlockToResponsesInputPart(b, role); ok {
					content = append(content, p)
				}
			}
		}
		flush()
	}
	return input
}

func canonicalBlockToResponsesInputPart(b ContentBlock, role string) (json.RawMessage, bool) {
	switch b.Kind {
	case BlockText:
		if b.Text == "" {
			return nil, false
		}
		t := "input_text"
		if role == "assistant" {
			t = "output_text"
		}
		return mustRaw(map[string]any{"type": t, "text": b.Text}), true
	case BlockImage:
		t := "input_image"
		if role == "assistant" {
			t = "output_image"
		}
		item := map[string]any{"type": t, "image_url": mediaDataOrURL(b.MediaType, b.Data, b.URL)}
		if b.Detail != "" {
			item["detail"] = b.Detail
		}
		return mustRaw(item), true
	case BlockFile:
		item := map[string]any{"type": "input_file"}
		if b.FileID != "" {
			item["file_id"] = b.FileID
		}
		if b.Data != "" || b.FileURL != "" {
			item["file_data"] = mediaDataOrURL(b.MediaType, b.Data, b.FileURL)
		}
		if b.Filename != "" {
			item["filename"] = b.Filename
		}
		if len(item) <= 1 {
			return nil, false
		}
		return mustRaw(item), true
	case BlockAudio:
		format := b.Format
		if format == "" {
			format = "mp3"
		}
		return mustRaw(map[string]any{"type": "input_audio",
			"input_audio": map[string]any{"data": b.Data, "format": format}}), true
	default:
		return nil, false
	}
}

func canonicalBlockToResponsesPart(b ContentBlock) (json.RawMessage, bool) {
	switch b.Kind {
	case BlockText:
		part := map[string]any{"type": "output_text", "text": b.Text, "annotations": []any{}}
		if bucket := b.Extensions.get(nsResponses); bucket != nil {
			if ann := bucket["annotations"]; ann != nil {
				part["annotations"] = ann
			}
		}
		return mustRaw(part), true
	case BlockImage:
		part := map[string]any{"type": "output_image", "image_url": mediaDataOrURL(b.MediaType, b.Data, b.URL)}
		if b.Detail != "" {
			part["detail"] = b.Detail
		}
		return mustRaw(part), true
	case BlockFile:
		file := map[string]any{}
		if b.FileID != "" {
			file["file_id"] = b.FileID
		}
		if b.Data != "" || b.FileURL != "" {
			file["file_data"] = mediaDataOrURL(b.MediaType, b.Data, b.FileURL)
		}
		if b.Filename != "" {
			file["filename"] = b.Filename
		}
		return mustRaw(map[string]any{"type": "file", "file": file}), true
	case BlockAudio:
		format := b.Format
		if format == "" {
			format = "mp3"
		}
		return mustRaw(map[string]any{"type": "input_audio",
			"input_audio": map[string]any{"data": b.Data, "format": format}}), true
	default:
		return nil, false
	}
}

func coalesceResponsesText(content []json.RawMessage) []json.RawMessage {
	if len(content) <= 1 {
		return content
	}
	var text strings.Builder
	var annotations []json.RawMessage
	for _, p := range content {
		obj, ok := decodeObject(p)
		if !ok {
			return content
		}
		pt := getStr(obj, "type")
		if pt != "output_text" && pt != "text" {
			return content
		}
		t, ok := asString(field(obj, "text"))
		if !ok {
			return content
		}
		text.WriteString(t)
		if ann, ok := decodeArray(field(obj, "annotations")); ok {
			annotations = append(annotations, ann...)
		}
	}
	if annotations == nil {
		annotations = []json.RawMessage{}
	}
	return []json.RawMessage{mustRaw(map[string]any{
		"type": "output_text", "text": text.String(), "annotations": annotations})}
}

// ----------------------------------------------------------------------------
// tools / tool_choice / text / reasoning / usage（Responses）
// ----------------------------------------------------------------------------

func responsesToolsToCanonical(raw json.RawMessage) ([]ToolDefinition, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	arr, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var out []ToolDefinition
	for _, t := range arr {
		obj, ok := decodeObject(t)
		if !ok {
			return nil, false
		}
		toolType := normLower(strOr(obj, "type", "function"))
		switch {
		case toolType == "function":
			if fn, ok := decodeObject(field(obj, "function")); ok {
				name := trimmed(getStr(fn, "name"))
				if name == "" {
					return nil, false
				}
				out = append(out, ToolDefinition{Name: name, Description: getStr(fn, "description"),
					Parameters: field(fn, "parameters"),
					Extensions: collectExtensions(obj, nsResponses, "type", "function")})
				continue
			}
			name := trimmed(getStr(obj, "name"))
			if name == "" {
				return nil, false
			}
			out = append(out, ToolDefinition{Name: name, Description: getStr(obj, "description"),
				Parameters: field(obj, "parameters"),
				Extensions: collectExtensions(obj, nsResponses, "type", "name", "description", "parameters")})
		case toolType == "custom" || strings.HasPrefix(toolType, "web_search"):
			name := trimmed(getStr(obj, "name"))
			if custom, ok := decodeObject(field(obj, "custom")); ok && name == "" {
				name = trimmed(getStr(custom, "name"))
			}
			if name == "" {
				name = toolType
			}
			// 整工具原样存入扩展，emit 时回吐
			ext := Extensions{}
			for k, v := range obj {
				ext.setField(nsResponses, k, v)
			}
			out = append(out, ToolDefinition{Name: name, Extensions: ext})
		}
	}
	return out, true
}

func canonicalToolsToResponses(req *Request) []json.RawMessage {
	var tools []json.RawMessage
	for i := range req.Tools {
		tools = append(tools, canonicalToolToResponses(&req.Tools[i]))
	}
	if bucket := req.Extensions.get(nsResponses); bucket != nil {
		if extra, ok := decodeArray(bucket["tools"]); ok {
			tools = append(tools, extra...)
		}
	}
	return tools
}

func canonicalToolToResponses(t *ToolDefinition) json.RawMessage {
	if bucket := t.Extensions.get(nsResponses); bucket != nil {
		if tt, ok := asString(bucket["type"]); ok && (tt == "custom" || strings.HasPrefix(tt, "web_search")) {
			return mustRaw(rawObjToAny(bucket))
		}
	}
	out := map[string]any{"type": "function", "name": t.Name}
	if t.Description != "" {
		out["description"] = t.Description
	}
	if len(t.Parameters) > 0 {
		out["parameters"] = t.Parameters
	}
	emitExtensions(out, t.Extensions, nsResponses)
	return mustRaw(out)
}

func responsesToolChoiceToCanonical(raw json.RawMessage) *ToolChoice {
	return openaiResponsesToolChoiceToCanonical(raw)
}

// openaiResponsesToolChoiceToCanonical 处理 responses 的 tool_choice（字符串或
// {type:function|custom, name|function.name}）。
func openaiResponsesToolChoiceToCanonical(raw json.RawMessage) *ToolChoice {
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
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	switch normLower(getStr(obj, "type")) {
	case "function":
		name := getStr(obj, "name")
		if name == "" {
			if fn, ok := decodeObject(field(obj, "function")); ok {
				name = getStr(fn, "name")
			}
		}
		if trimmed(name) != "" {
			return &ToolChoice{Kind: ToolChoiceTool, Name: name}
		}
	case "custom":
		name := getStr(obj, "name")
		if name == "" {
			if custom, ok := decodeObject(field(obj, "custom")); ok {
				name = getStr(custom, "name")
			}
		}
		if trimmed(name) != "" {
			return &ToolChoice{Kind: ToolChoiceTool, Name: name}
		}
	}
	return nil
}

func canonicalToolChoiceToResponses(tc *ToolChoice) any {
	switch tc.Kind {
	case ToolChoiceAuto:
		return "auto"
	case ToolChoiceNone:
		return "none"
	case ToolChoiceRequired:
		return "required"
	case ToolChoiceTool:
		return map[string]any{"type": "function", "name": tc.Name}
	}
	return "auto"
}

func canonicalTextConfigToResponses(req *Request) map[string]any {
	text := map[string]any{}
	if req.ResponseFormat != nil {
		text["format"] = canonicalResponseFormatToOpenAI(req.ResponseFormat)
	}
	if bucket := req.Extensions.get(nsResponses); bucket != nil {
		if v := bucket["verbosity"]; v != nil {
			text["verbosity"] = v
		}
	}
	if len(text) == 0 {
		return nil
	}
	return text
}

func reasoningConfigToResponses(t *ThinkingConfig) any {
	if bucket := t.Extensions.get(nsResponses); len(bucket) > 0 {
		return rawObjToAny(bucket)
	}
	if effort := extString(t.Extensions, "openai", "reasoning_effort"); effort != "" {
		return map[string]any{"effort": responsesReasoningEffort(effort)}
	}
	if t.BudgetTokens != nil {
		return map[string]any{"effort": budgetToReasoningEffort(*t.BudgetTokens)}
	}
	return nil
}

func responsesReasoningEffort(effort string) string {
	switch strings.ToLower(trimmed(effort)) {
	case "xhigh", "max":
		return "xhigh"
	case "low", "medium", "high":
		return strings.ToLower(trimmed(effort))
	default:
		return effort
	}
}

func responsesToolResultOutput(b ContentBlock) any {
	if b.HasOutput && len(b.Output) > 0 {
		if s, ok := asString(b.Output); ok {
			return s
		}
		return compactJSON(b.Output)
	}
	return b.ContentText
}

func canonicalUsageToResponsesUsage(u *Usage) json.RawMessage {
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens
	}
	out := map[string]any{"input_tokens": u.InputTokens, "output_tokens": u.OutputTokens, "total_tokens": total}
	if u.ReasoningTokens > 0 {
		out["output_tokens_details"] = map[string]any{"reasoning_tokens": u.ReasoningTokens}
	}
	inputDetails := map[string]any{}
	if u.CacheReadTokens > 0 {
		inputDetails["cached_tokens"] = u.CacheReadTokens
	}
	if u.CacheWriteTokens > 0 {
		inputDetails["cached_creation_tokens"] = u.CacheWriteTokens
	}
	if len(inputDetails) > 0 {
		out["input_tokens_details"] = inputDetails
	}
	return mustRaw(out)
}
