package formats

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Gemini generateContent（/v1beta/models/{model}:generateContent）的 parse/emit。
// 字段映射对照 Aether formats/gemini/generate_content/*.rs + 各 gemini_* helper。
//
// P-A 范围说明：未知顶层字段（safetySettings / cachedContent 等）经 extensions 原样
// 透传；generationConfig 内的 thinkingConfig/responseModalities 等结构化子字段、
// 内置工具（googleSearch/codeExecution）暂走基础路径，不做 Aether 的全保真重组。
// gemini 流式由 URL 动作（:streamGenerateContent）决定，body 不带 stream 标志。

// ----------------------------------------------------------------------------
// request
// ----------------------------------------------------------------------------

func geminiRequestFrom(body json.RawMessage) (*Request, bool) {
	obj, ok := decodeObject(body)
	if !ok {
		return nil, false
	}
	req := &Request{Model: getStr(obj, "model")}

	insts, ok := geminiSystemToInstructions(fieldOr(obj, "systemInstruction", "system_instruction"))
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

	msgs, ok := geminiContentsToMessages(field(obj, "contents"))
	if !ok {
		return nil, false
	}
	req.Messages = msgs
	genCfg := fieldOr(obj, "generationConfig", "generation_config")
	req.Generation = geminiGenerationConfig(genCfg)
	req.Thinking = geminiThinkingToCanonical(genCfg)
	req.ResponseFormat = geminiResponseFormatToCanonical(genCfg)

	tools, builtin, ok := geminiToolsToCanonical(field(obj, "tools"))
	if !ok {
		return nil, false
	}
	req.Tools = tools
	req.ToolChoice = geminiToolChoiceToCanonical(fieldOr(obj, "toolConfig", "tool_config"))
	req.Extensions = collectExtensions(obj, "gemini",
		"model", "systemInstruction", "system_instruction", "contents",
		"generationConfig", "generation_config", "tools", "toolConfig", "tool_config", "stream")
	if len(builtin) > 0 {
		req.Extensions.setField("gemini", "builtin_tools", mustRaw(builtin))
	}
	return req, true
}

func geminiRequestTo(req *Request, _ upstreamStreamUnused) json.RawMessage {
	out := map[string]any{}
	if trimmed(req.Model) != "" {
		out["model"] = trimmed(req.Model)
	}
	out["contents"] = compactGeminiContents(canonicalMessagesToGeminiContents(req.Messages))
	if si := geminiSystemInstruction(req); si != nil {
		out["systemInstruction"] = si
	}
	if gc := canonicalGenerationConfigToGemini(req); gc != nil {
		out["generationConfig"] = gc
	}
	if tools := canonicalToolsToGemini(req); tools != nil {
		out["tools"] = tools
	}
	if tc := canonicalToolChoiceToGemini(req.ToolChoice); tc != nil {
		out["toolConfig"] = tc
	}
	emitExtensions(out, req.Extensions, "gemini")
	return mustRaw(out)
}

// upstreamStreamUnused 让 geminiRequestTo 的签名与其它格式一致（gemini body 不带
// stream 标志），同时显式标注该参数无用。
type upstreamStreamUnused = bool

// ----------------------------------------------------------------------------
// response
// ----------------------------------------------------------------------------

func geminiResponseFrom(body json.RawMessage) (*Response, bool) {
	obj, ok := decodeObject(body)
	if !ok || objErrorFieldIsPresent(obj) {
		return nil, false
	}
	candidates, ok := decodeArray(field(obj, "candidates"))
	if !ok {
		return nil, false
	}
	var outputs []ResponseOutput
	for i, c := range candidates {
		cand, ok := decodeObject(c)
		if !ok {
			return nil, false
		}
		var content []ContentBlock
		if cc, ok := decodeObject(field(cand, "content")); ok {
			if parts, ok := decodeArray(field(cc, "parts")); ok {
				for idx, p := range parts {
					if b, ok := geminiPartToBlock(p, idx); ok {
						content = append(content, b)
					}
				}
			}
		}
		sr := geminiStopReasonToCanonical(strFirst(cand, "finishReason", "finish_reason"))
		if hasToolUse(content) && (sr == nil || *sr == StopEndTurn) {
			tu := StopToolUse
			sr = &tu
		}
		idx := i
		if v, ok := asUint(field(cand, "index")); ok {
			idx = int(v)
		}
		outputs = append(outputs, ResponseOutput{Index: idx, Role: RoleAssistant, Content: content, StopReason: sr})
	}
	resp := &Response{
		ID:         strFirstOr(obj, "gemini-local-finalize", "responseId", "_v1internal_response_id"),
		Model:      strOr(obj, "modelVersion", "unknown"),
		Outputs:    outputs,
		Usage:      geminiUsageToCanonical(field(obj, "usageMetadata")),
		Extensions: collectExtensions(obj, "gemini", "responseId", "_v1internal_response_id", "modelVersion", "candidates", "usageMetadata"),
	}
	if len(outputs) > 0 {
		resp.Content = outputs[0].Content
		resp.StopReason = outputs[0].StopReason
	}
	return resp, true
}

func geminiResponseTo(resp *Response) json.RawMessage {
	outputs := resp.Outputs
	if len(outputs) == 0 {
		outputs = []ResponseOutput{{Index: 0, Role: RoleAssistant, Content: resp.Content, StopReason: resp.StopReason}}
	}
	candidates := make([]json.RawMessage, 0, len(outputs))
	for _, o := range outputs {
		parts := canonicalBlocksToGeminiParts(o.Content)
		if len(parts) == 0 {
			parts = []json.RawMessage{mustRaw(map[string]any{"text": ""})}
		}
		sr := o.StopReason
		if sr == nil {
			sr = resp.StopReason
		}
		candidates = append(candidates, mustRaw(map[string]any{
			"index":        o.Index,
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": canonicalStopReasonToGemini(sr),
		}))
	}
	id := resp.ID
	if trimmed(id) == "" {
		id = "resp-local-finalize"
	}
	model := resp.Model
	if trimmed(model) == "" {
		model = "unknown"
	}
	out := map[string]any{"responseId": id, "modelVersion": model, "candidates": candidates}
	if resp.Usage != nil {
		out["usageMetadata"] = canonicalUsageToGeminiMetadata(resp.Usage)
	}
	emitExtensions(out, resp.Extensions, "gemini")
	return mustRaw(out)
}

// ----------------------------------------------------------------------------
// 内容块 parse（Gemini → 规范化块）
// ----------------------------------------------------------------------------

func geminiSystemToInstructions(raw json.RawMessage) ([]Instruction, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if s, ok := asString(raw); ok {
		if trimmed(s) == "" {
			return nil, true
		}
		return []Instruction{{Role: RoleSystem, Text: s}}, true
	}
	obj, ok := decodeObject(raw)
	if !ok {
		return nil, false
	}
	parts, ok := decodeArray(field(obj, "parts"))
	if !ok {
		return nil, false
	}
	var insts []Instruction
	for _, p := range parts {
		pObj, ok := decodeObject(p)
		if !ok {
			return nil, false
		}
		text := getStr(pObj, "text")
		if trimmed(text) == "" {
			continue
		}
		insts = append(insts, Instruction{Role: RoleSystem, Text: text,
			Extensions: collectExtensions(pObj, "gemini", "text")})
	}
	return insts, true
}

func geminiContentsToMessages(raw json.RawMessage) ([]Message, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	arr, ok := decodeArray(raw)
	if !ok {
		return nil, false
	}
	var out []Message
	for _, c := range arr {
		obj, ok := decodeObject(c)
		if !ok {
			return nil, false
		}
		var role Role
		switch normLower(strOr(obj, "role", "user")) {
		case "model":
			role = RoleAssistant
		case "system":
			role = RoleSystem
		case "tool", "function":
			role = RoleTool
		default:
			role = RoleUser
		}
		parts, ok := decodeArray(field(obj, "parts"))
		if !ok {
			return nil, false
		}
		var blocks []ContentBlock
		for idx, p := range parts {
			b, ok := geminiPartToBlock(p, idx)
			if !ok {
				return nil, false
			}
			blocks = append(blocks, b)
		}
		if len(blocks) == 0 {
			continue
		}
		out = append(out, Message{Role: role, Content: blocks,
			Extensions: collectExtensions(obj, "gemini", "role", "parts")})
	}
	return out, true
}

func geminiPartToBlock(raw json.RawMessage, index int) (ContentBlock, bool) {
	obj, ok := decodeObject(raw)
	if !ok {
		return ContentBlock{}, false
	}
	if text, ok := asString(field(obj, "text")); ok {
		if thought, _ := asBool(field(obj, "thought")); thought {
			return ContentBlock{Kind: BlockThinking, Text: text,
				Signature:  strFirst(obj, "thoughtSignature", "thought_signature"),
				Extensions: collectExtensions(obj, "gemini", "text", "thought", "thoughtSignature", "thought_signature")}, true
		}
		return ContentBlock{Kind: BlockText, Text: text,
			Extensions: collectExtensions(obj, "gemini", "text")}, true
	}
	if inline, ok := decodeObject(fieldOr(obj, "inlineData", "inline_data")); ok {
		return geminiInlineDataToBlock(inline, obj), true
	}
	if fileData, ok := decodeObject(fieldOr(obj, "fileData", "file_data")); ok {
		return geminiFileDataToBlock(fileData, obj), true
	}
	if fc, ok := decodeObject(fieldOr(obj, "functionCall", "function_call")); ok {
		name := trimmed(getStr(fc, "name"))
		id := trimmed(getStr(fc, "id"))
		if id == "" {
			id = "call_auto_" + strconv.Itoa(index)
		}
		args := field(fc, "args")
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		return ContentBlock{Kind: BlockToolUse, ID: id, Name: name, Input: args,
			Extensions: collectExtensions(obj, "gemini", "functionCall", "function_call")}, true
	}
	if fr, ok := decodeObject(fieldOr(obj, "functionResponse", "function_response")); ok {
		name := trimmed(getStr(fr, "name"))
		toolUseID := trimmed(getStr(fr, "id"))
		if toolUseID == "" {
			toolUseID = name
		}
		if toolUseID == "" {
			toolUseID = "toolu_response_" + strconv.Itoa(index)
		}
		respVal := field(fr, "response")
		if len(respVal) == 0 {
			respVal = json.RawMessage("{}")
		}
		// 若 response 是对象且含 result，取 result；否则整体作为 output
		output := respVal
		if rObj, ok := decodeObject(respVal); ok {
			if r := field(rObj, "result"); r != nil {
				output = r
			}
		}
		return ContentBlock{Kind: BlockToolResult, ToolUseID: toolUseID, Name: name,
			Output: output, HasOutput: true, ContentText: toolOutputText(output), HasContentText: true,
			Extensions: collectExtensions(obj, "gemini", "functionResponse", "function_response")}, true
	}
	return ContentBlock{Kind: BlockUnknown, RawType: "unknown", Payload: raw,
		Extensions: Extensions{"gemini": {"_raw": raw}}}, true
}

func geminiInlineDataToBlock(inline, part map[string]json.RawMessage) ContentBlock {
	mediaType := trimmed(strFirst(inline, "mimeType", "mime_type"))
	data := trimmed(getStr(inline, "data"))
	ext := collectExtensions(part, "gemini", "inlineData", "inline_data")
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return ContentBlock{Kind: BlockImage, Data: data, MediaType: mediaType, Extensions: ext}
	case strings.HasPrefix(mediaType, "audio/"):
		return ContentBlock{Kind: BlockAudio, Data: data, MediaType: mediaType,
			Format: strings.TrimPrefix(mediaType, "audio/"), Extensions: ext}
	default:
		return ContentBlock{Kind: BlockFile, Data: data, MediaType: mediaType, Extensions: ext}
	}
}

func geminiFileDataToBlock(fileData, part map[string]json.RawMessage) ContentBlock {
	fileURI := trimmed(strFirst(fileData, "fileUri", "file_uri"))
	mediaType := trimmed(strFirst(fileData, "mimeType", "mime_type"))
	ext := collectExtensions(part, "gemini", "fileData", "file_data")
	if strings.HasPrefix(mediaType, "image/") {
		return ContentBlock{Kind: BlockImage, URL: fileURI, MediaType: mediaType, Extensions: ext}
	}
	return ContentBlock{Kind: BlockFile, FileURL: fileURI, MediaType: mediaType, Extensions: ext}
}

// ----------------------------------------------------------------------------
// 内容块 emit（规范化 → Gemini）
// ----------------------------------------------------------------------------

func geminiSystemInstruction(req *Request) map[string]any {
	var texts []string
	for _, in := range req.Instructions {
		if trimmed(in.Text) != "" {
			texts = append(texts, in.Text)
		}
	}
	text := strings.Join(texts, "\n\n")
	if trimmed(text) == "" {
		text = req.System
	}
	if trimmed(text) == "" {
		return nil
	}
	return map[string]any{"parts": []any{map[string]any{"text": text}}}
}

func canonicalMessagesToGeminiContents(messages []Message) []json.RawMessage {
	var out []json.RawMessage
	for i := range messages {
		msg := &messages[i]
		var role string
		switch msg.Role {
		case RoleAssistant:
			role = "model"
		case RoleSystem, RoleDeveloper:
			continue
		default:
			role = "user"
		}
		var parts []json.RawMessage
		for _, b := range msg.Content {
			if p, ok := canonicalBlockToGeminiPart(b); ok {
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, mustRaw(map[string]any{"role": role, "parts": parts}))
	}
	return out
}

func canonicalBlocksToGeminiParts(blocks []ContentBlock) []json.RawMessage {
	var parts []json.RawMessage
	for _, b := range blocks {
		if p, ok := canonicalBlockToGeminiPart(b); ok {
			parts = append(parts, p)
		}
	}
	return parts
}

func canonicalBlockToGeminiPart(b ContentBlock) (json.RawMessage, bool) {
	switch b.Kind {
	case BlockText:
		return mustRaw(map[string]any{"text": b.Text}), true
	case BlockThinking:
		if trimmed(b.Text) == "" {
			return nil, false
		}
		part := map[string]any{"text": b.Text, "thought": true}
		if b.Signature != "" {
			part["thoughtSignature"] = b.Signature
		}
		return mustRaw(part), true
	case BlockImage:
		mt := b.MediaType
		if mt == "" {
			mt = "image/png"
		}
		return canonicalMediaToGeminiPart(mt, b.Data, b.URL), true
	case BlockFile:
		mt := b.MediaType
		if mt == "" {
			mt = "application/octet-stream"
		}
		return canonicalMediaToGeminiPart(mt, b.Data, b.FileURL), true
	case BlockAudio:
		if b.Data == "" {
			return nil, false
		}
		mt := b.MediaType
		if mt == "" {
			mt = "audio/mpeg"
		}
		return mustRaw(map[string]any{"inlineData": map[string]any{"mimeType": mt, "data": b.Data}}), true
	case BlockToolUse:
		return mustRaw(map[string]any{"functionCall": map[string]any{
			"id": b.ID, "name": b.Name, "args": geminiFunctionArgs(b.Input)}}), true
	case BlockToolResult:
		name := b.Name
		if name == "" {
			name = b.ToolUseID
		}
		return mustRaw(map[string]any{"functionResponse": map[string]any{
			"id": b.ToolUseID, "name": name, "response": geminiFunctionResponse(b)}}), true
	default:
		return nil, false
	}
}

func canonicalMediaToGeminiPart(mediaType, data, url string) json.RawMessage {
	if data != "" {
		return mustRaw(map[string]any{"inlineData": map[string]any{"mimeType": mediaType, "data": data}})
	}
	return mustRaw(map[string]any{"fileData": map[string]any{"mimeType": mediaType, "fileUri": url}})
}

func geminiFunctionArgs(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return json.RawMessage("{}")
	}
	if _, ok := decodeObject(input); ok {
		return input
	}
	if string(input) == "null" {
		return json.RawMessage("{}")
	}
	return mustRaw(map[string]any{"value": input})
}

func geminiFunctionResponse(b ContentBlock) json.RawMessage {
	if b.HasOutput && len(b.Output) > 0 {
		if obj, ok := decodeObject(b.Output); ok {
			return mustRaw(obj)
		}
		return mustRaw(map[string]any{"result": b.Output})
	}
	return mustRaw(map[string]any{"result": b.ContentText})
}

func compactGeminiContents(contents []json.RawMessage) []json.RawMessage {
	var compact []json.RawMessage
	for _, c := range contents {
		obj, _ := decodeObject(c)
		role := getStr(obj, "role")
		parts, _ := decodeArray(field(obj, "parts"))
		if len(parts) == 0 {
			continue
		}
		if len(compact) > 0 {
			lastObj, _ := decodeObject(compact[len(compact)-1])
			if getStr(lastObj, "role") == role {
				lastParts, _ := decodeArray(field(lastObj, "parts"))
				merged := append(lastParts, parts...)
				compact[len(compact)-1] = mustRaw(map[string]any{"role": role, "parts": merged})
				continue
			}
		}
		compact = append(compact, mustRaw(map[string]any{"role": role, "parts": parts}))
	}
	return compact
}

// ----------------------------------------------------------------------------
// generation / tools / tool_choice / thinking / usage / stop（Gemini）
// ----------------------------------------------------------------------------

func geminiGenerationConfig(raw json.RawMessage) GenerationConfig {
	obj, ok := decodeObject(raw)
	if !ok {
		return GenerationConfig{}
	}
	var g GenerationConfig
	if v, ok := asUint(fieldOr(obj, "maxOutputTokens", "max_output_tokens")); ok {
		g.MaxTokens = &v
	}
	if v, ok := asFloat(field(obj, "temperature")); ok {
		g.Temperature = &v
	}
	if v, ok := asFloat(fieldOr(obj, "topP", "top_p")); ok {
		g.TopP = &v
	}
	if v, ok := asUint(fieldOr(obj, "topK", "top_k")); ok {
		g.TopK = &v
	}
	if ss := openaiStopToVec(fieldOr(obj, "stopSequences", "stop_sequences")); ss != nil {
		g.StopSequences = &ss
	}
	if v, ok := asUint(fieldOr(obj, "candidateCount", "candidate_count")); ok {
		g.N = &v
	}
	if v, ok := asInt(field(obj, "seed")); ok {
		g.Seed = &v
	}
	return g
}

func canonicalGenerationConfigToGemini(req *Request) map[string]any {
	g := &req.Generation
	gc := map[string]any{}
	if g.MaxTokens != nil {
		gc["maxOutputTokens"] = *g.MaxTokens
	}
	if g.Temperature != nil {
		gc["temperature"] = *g.Temperature
	}
	if g.TopP != nil {
		gc["topP"] = *g.TopP
	}
	if g.TopK != nil {
		gc["topK"] = *g.TopK
	}
	if g.N != nil && *g.N > 1 {
		gc["candidateCount"] = *g.N
	}
	if g.Seed != nil {
		gc["seed"] = *g.Seed
	}
	if g.StopSequences != nil {
		gc["stopSequences"] = *g.StopSequences
	}
	if req.ResponseFormat != nil {
		applyResponseFormatToGemini(gc, req.ResponseFormat)
	}
	if req.Thinking != nil {
		if tc := geminiThinkingConfig(req.Thinking); tc != nil {
			gc["thinkingConfig"] = tc
		}
	}
	if len(gc) == 0 {
		return nil
	}
	return gc
}

func applyResponseFormatToGemini(gc map[string]any, rf *ResponseFormat) {
	switch rf.FormatType {
	case "json_schema":
		gc["responseMimeType"] = "application/json"
		if len(rf.JSONSchema) > 0 {
			schema := rf.JSONSchema
			if obj, ok := decodeObject(rf.JSONSchema); ok {
				if inner := field(obj, "schema"); inner != nil {
					schema = inner
				}
			}
			gc["responseSchema"] = schema
		}
	case "json_object":
		gc["responseMimeType"] = "application/json"
	}
}

func geminiThinkingConfig(t *ThinkingConfig) map[string]any {
	// 优先用解析时保留的原始 thinking_config
	if bucket := t.Extensions.get("gemini"); bucket != nil {
		if tc, ok := decodeObject(bucket["thinking_config"]); ok {
			return rawObjToAny(tc)
		}
	}
	var budget *uint64
	if t.BudgetTokens != nil {
		budget = t.BudgetTokens
	} else if effort := openaiReasoningEffort(t); effort != "" {
		if b, ok := reasoningEffortToBudget(effort); ok {
			budget = &b
		}
	}
	if budget == nil {
		return nil
	}
	return map[string]any{"includeThoughts": true, "thinkingBudget": *budget}
}

func geminiThinkingToCanonical(raw json.RawMessage) *ThinkingConfig {
	gc, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	tc, ok := decodeObject(fieldOr(gc, "thinkingConfig", "thinking_config"))
	if !ok {
		return nil
	}
	ext := Extensions{}
	ext.setField("gemini", "thinking_config", fieldOr(gc, "thinkingConfig", "thinking_config"))
	var budget *uint64
	if b, ok := asUint(fieldOr(tc, "thinkingBudget", "thinking_budget")); ok {
		budget = &b
		ext.setField("openai", "reasoning_effort", rawString(budgetToReasoningEffort(b)))
	}
	enabled := true
	if v, ok := asBool(fieldOr(tc, "includeThoughts", "include_thoughts")); ok {
		enabled = v
	}
	return &ThinkingConfig{Enabled: enabled, BudgetTokens: budget, Extensions: ext}
}

func geminiResponseFormatToCanonical(raw json.RawMessage) *ResponseFormat {
	gc, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	mime := strFirst(gc, "responseMimeType", "response_mime_type")
	if mime != "application/json" {
		return nil
	}
	schema := fieldOr(gc, "responseSchema", "response_schema")
	if len(schema) > 0 {
		return &ResponseFormat{FormatType: "json_schema",
			JSONSchema: mustRaw(map[string]any{"name": "response_schema", "schema": schema})}
	}
	return &ResponseFormat{FormatType: "json_object"}
}

func geminiToolsToCanonical(raw json.RawMessage) (tools []ToolDefinition, builtin []json.RawMessage, ok bool) {
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
		if objHasKey(obj, "googleSearch") || objHasKey(obj, "google_search") ||
			objHasKey(obj, "codeExecution") || objHasKey(obj, "code_execution") ||
			objHasKey(obj, "urlContext") || objHasKey(obj, "url_context") {
			builtin = append(builtin, t)
		}
		decls, ok := decodeArray(fieldOr(obj, "functionDeclarations", "function_declarations"))
		if !ok {
			continue
		}
		for _, d := range decls {
			dObj, ok := decodeObject(d)
			if !ok {
				return nil, nil, false
			}
			name := trimmed(getStr(dObj, "name"))
			if name == "" {
				return nil, nil, false
			}
			tools = append(tools, ToolDefinition{
				Name:        name,
				Description: getStr(dObj, "description"),
				Parameters:  field(dObj, "parameters"),
				Extensions:  collectExtensions(dObj, "gemini", "name", "description", "parameters"),
			})
		}
	}
	return tools, builtin, true
}

func canonicalToolsToGemini(req *Request) []json.RawMessage {
	var tools []json.RawMessage
	var decls []json.RawMessage
	for i := range req.Tools {
		t := &req.Tools[i]
		decl := map[string]any{"name": t.Name}
		if t.Description != "" {
			decl["description"] = t.Description
		}
		if len(t.Parameters) > 0 {
			decl["parameters"] = t.Parameters
		} else {
			decl["parameters"] = map[string]any{}
		}
		decls = append(decls, mustRaw(decl))
	}
	if len(decls) > 0 {
		tools = append(tools, mustRaw(map[string]any{"functionDeclarations": decls}))
	}
	if bucket := req.Extensions.get("gemini"); bucket != nil {
		if arr, ok := decodeArray(bucket["builtin_tools"]); ok {
			tools = append(tools, arr...)
		}
	}
	if len(tools) == 0 {
		return nil
	}
	return tools
}

func geminiToolChoiceToCanonical(raw json.RawMessage) *ToolChoice {
	cfg, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	fc, ok := decodeObject(fieldOr(cfg, "functionCallingConfig", "function_calling_config"))
	if !ok {
		return nil
	}
	if names, ok := decodeArray(fieldOr(fc, "allowedFunctionNames", "allowed_function_names")); ok && len(names) > 0 {
		if name, ok := asString(names[0]); ok && trimmed(name) != "" {
			return &ToolChoice{Kind: ToolChoiceTool, Name: name}
		}
	}
	switch strings.ToUpper(trimmed(getStr(fc, "mode"))) {
	case "NONE":
		return &ToolChoice{Kind: ToolChoiceNone}
	case "AUTO":
		return &ToolChoice{Kind: ToolChoiceAuto}
	case "ANY", "REQUIRED":
		return &ToolChoice{Kind: ToolChoiceRequired}
	default:
		return nil
	}
}

func canonicalToolChoiceToGemini(tc *ToolChoice) map[string]any {
	if tc == nil {
		return nil
	}
	mode := "AUTO"
	switch tc.Kind {
	case ToolChoiceNone:
		mode = "NONE"
	case ToolChoiceRequired, ToolChoiceTool:
		mode = "ANY"
	}
	fcc := map[string]any{"mode": mode}
	if tc.Kind == ToolChoiceTool {
		fcc["allowedFunctionNames"] = []any{tc.Name}
	}
	return map[string]any{"functionCallingConfig": fcc}
}

func geminiUsageToCanonical(raw json.RawMessage) *Usage {
	obj, ok := decodeObject(raw)
	if !ok {
		return nil
	}
	input, _ := asUint(fieldOr(obj, "promptTokenCount", "prompt_token_count"))
	visible, _ := asUint(fieldOr(obj, "candidatesTokenCount", "candidates_token_count"))
	reasoning, _ := asUint(fieldOr(obj, "thoughtsTokenCount", "thoughts_token_count"))
	output := visible + reasoning
	u := &Usage{InputTokens: input, OutputTokens: output, ReasoningTokens: reasoning}
	if total, ok := asUint(fieldOr(obj, "totalTokenCount", "total_token_count")); ok {
		u.TotalTokens = total
	} else {
		u.TotalTokens = input + output
	}
	return u
}

func canonicalUsageToGeminiMetadata(u *Usage) json.RawMessage {
	out := map[string]any{
		"promptTokenCount":     u.InputTokens,
		"candidatesTokenCount": u.OutputTokens - min64(u.ReasoningTokens, u.OutputTokens),
		"totalTokenCount":      u.TotalTokens,
	}
	if u.ReasoningTokens > 0 {
		out["thoughtsTokenCount"] = u.ReasoningTokens
	}
	return mustRaw(out)
}

func geminiStopReasonToCanonical(value string) *StopReason {
	if value == "" {
		return nil
	}
	var sr StopReason
	switch strings.ToUpper(trimmed(value)) {
	case "STOP":
		sr = StopEndTurn
	case "MAX_TOKENS":
		sr = StopMaxTokens
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		sr = StopContentFiltered
	default:
		sr = StopUnknown
	}
	return &sr
}

func canonicalStopReasonToGemini(sr *StopReason) string {
	if sr == nil {
		return "STOP"
	}
	switch *sr {
	case StopMaxTokens:
		return "MAX_TOKENS"
	case StopContentFiltered, StopRefusal:
		return "SAFETY"
	case StopUnknown:
		return "OTHER"
	default:
		return "STOP"
	}
}

// ----------------------------------------------------------------------------
// 小工具
// ----------------------------------------------------------------------------

func strFirst(obj map[string]json.RawMessage, keys ...string) string {
	s, _ := asString(fieldOr(obj, keys...))
	return s
}

func strFirstOr(obj map[string]json.RawMessage, def string, keys ...string) string {
	if s, ok := asString(fieldOr(obj, keys...)); ok {
		return s
	}
	return def
}

func hasToolUse(blocks []ContentBlock) bool {
	for _, b := range blocks {
		if b.Kind == BlockToolUse {
			return true
		}
	}
	return false
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// rawObjToAny 把 RawMessage 对象转成 map[string]any（用于嵌入 mustRaw 时保持原值）。
func rawObjToAny(obj map[string]json.RawMessage) map[string]any {
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		out[k] = v
	}
	return out
}
