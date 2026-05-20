package audit

import (
	"encoding/json"
	"sort"
	"strings"

	"xiaodou/uni-ai-api/internal/domain"
)

// ResponseAccumulator rebuilds an assistant message from streaming SSE data
// frames. Call AddChunk for each `data:` payload (JSON bytes, no prefix or
// newline); call Build after the stream ends.
type ResponseAccumulator struct {
	protocol domain.UpstreamProtocol

	// OpenAI Chat / Completions
	oaiRole  string
	oaiText  strings.Builder
	oaiTools map[int]*oaiToolState // index → partial tool call

	// OpenAI Responses
	respOutputs map[int]*respOutputState // output_index → state
	respOrder   []int

	// Anthropic
	antBlocks map[int]*antBlockState // index → block
	antOrder  []int

	// Gemini
	gemText strings.Builder
}

type respOutputState struct {
	itemType  string // "message" | "function_call"
	textParts map[int]*strings.Builder // content_index → accumulated text
	partOrder []int
	callID    string
	funcName  string
	funcArgs  strings.Builder
}

type oaiToolState struct {
	ID   string
	Name string
	Args strings.Builder
}

type antBlockState struct {
	BlockType string // "text" | "tool_use"
	Text      strings.Builder
	ToolID    string
	ToolName  string
	Input     strings.Builder // partial JSON for tool_use input
}

// NewResponseAccumulator creates an accumulator for the given wire protocol.
func NewResponseAccumulator(protocol domain.UpstreamProtocol) *ResponseAccumulator {
	return &ResponseAccumulator{
		protocol:    protocol,
		oaiTools:    make(map[int]*oaiToolState),
		respOutputs: make(map[int]*respOutputState),
		antBlocks:   make(map[int]*antBlockState),
	}
}

// AddChunk processes one SSE data payload (the JSON bytes after `data: `).
// [DONE] markers and empty slices are silently ignored.
func (a *ResponseAccumulator) AddChunk(data []byte) {
	if len(data) == 0 {
		return
	}
	switch a.protocol {
	case domain.ProtocolOpenAIChat:
		a.addOpenAIChat(data)
	case domain.ProtocolOpenAICompletions:
		a.addOpenAICompletions(data)
	case domain.ProtocolOpenAIResponses:
		a.addOpenAIResponses(data)
	case domain.ProtocolAnthropicMessages:
		a.addAnthropic(data)
	case domain.ProtocolGeminiGenerate:
		a.addGemini(data)
	}
}

// Build constructs the final assistant message JSON. Returns nil when no
// content was accumulated.
func (a *ResponseAccumulator) Build() json.RawMessage {
	switch a.protocol {
	case domain.ProtocolOpenAIChat:
		return a.buildOpenAIChat()
	case domain.ProtocolOpenAIResponses:
		return a.buildOpenAIResponses()
	case domain.ProtocolOpenAICompletions:
		if a.oaiText.Len() == 0 {
			return nil
		}
		out, _ := json.Marshal(map[string]string{"role": "assistant", "text": a.oaiText.String()})
		return out
	case domain.ProtocolAnthropicMessages:
		return a.buildAnthropic()
	case domain.ProtocolGeminiGenerate:
		if a.gemText.Len() == 0 {
			return nil
		}
		type part struct {
			Text string `json:"text"`
		}
		type gemMsg struct {
			Role  string `json:"role"`
			Parts []part `json:"parts"`
		}
		out, _ := json.Marshal(gemMsg{Role: "model", Parts: []part{{Text: a.gemText.String()}}})
		return out
	default:
		return nil
	}
}

// ============================================================================
// OpenAI Chat
// ============================================================================

func (a *ResponseAccumulator) addOpenAIChat(data []byte) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Role      string          `json:"role"`
				Content   string          `json:"content"`
				ToolCalls []oaiToolDelta  `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil || len(chunk.Choices) == 0 {
		return
	}
	d := chunk.Choices[0].Delta
	if d.Role != "" {
		a.oaiRole = d.Role
	}
	a.oaiText.WriteString(d.Content)
	for _, tc := range d.ToolCalls {
		t, ok := a.oaiTools[tc.Index]
		if !ok {
			t = &oaiToolState{}
			a.oaiTools[tc.Index] = t
		}
		if tc.ID != "" {
			t.ID = tc.ID
		}
		if tc.Function.Name != "" {
			t.Name = tc.Function.Name
		}
		t.Args.WriteString(tc.Function.Arguments)
	}
}

type oaiToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (a *ResponseAccumulator) buildOpenAIChat() json.RawMessage {
	role := a.oaiRole
	if role == "" {
		role = "assistant"
	}
	hasText := a.oaiText.Len() > 0
	hasTools := len(a.oaiTools) > 0

	if !hasText && !hasTools {
		return nil
	}

	if !hasTools {
		out, _ := json.Marshal(map[string]string{"role": role, "content": a.oaiText.String()})
		return out
	}

	// Build tool call array ordered by index.
	type toolCall struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	idxs := make([]int, 0, len(a.oaiTools))
	for i := range a.oaiTools {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	tools := make([]toolCall, 0, len(idxs))
	for _, i := range idxs {
		t := a.oaiTools[i]
		var tc toolCall
		tc.ID = t.ID
		tc.Type = "function"
		tc.Function.Name = t.Name
		tc.Function.Arguments = t.Args.String()
		tools = append(tools, tc)
	}

	type msg struct {
		Role      string      `json:"role"`
		Content   *string     `json:"content"`
		ToolCalls []toolCall  `json:"tool_calls,omitempty"`
	}
	m := msg{Role: role, ToolCalls: tools}
	if hasText {
		s := a.oaiText.String()
		m.Content = &s
	}
	out, _ := json.Marshal(m)
	return out
}

// ============================================================================
// OpenAI Completions
// ============================================================================

func (a *ResponseAccumulator) addOpenAICompletions(data []byte) {
	var chunk struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil || len(chunk.Choices) == 0 {
		return
	}
	a.oaiText.WriteString(chunk.Choices[0].Text)
}

// ============================================================================
// Anthropic Messages
// ============================================================================

func (a *ResponseAccumulator) addAnthropic(data []byte) {
	var frame struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock *struct {
			Type string `json:"type"`
			Text string `json:"text"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"content_block"`
		Delta *struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		return
	}
	switch frame.Type {
	case "content_block_start":
		if frame.ContentBlock == nil {
			return
		}
		blk := &antBlockState{BlockType: frame.ContentBlock.Type}
		if frame.ContentBlock.Type == "text" {
			blk.Text.WriteString(frame.ContentBlock.Text)
		} else if frame.ContentBlock.Type == "tool_use" {
			blk.ToolID = frame.ContentBlock.ID
			blk.ToolName = frame.ContentBlock.Name
		}
		if _, exists := a.antBlocks[frame.Index]; !exists {
			a.antOrder = append(a.antOrder, frame.Index)
		}
		a.antBlocks[frame.Index] = blk

	case "content_block_delta":
		blk, ok := a.antBlocks[frame.Index]
		if !ok || frame.Delta == nil {
			return
		}
		switch frame.Delta.Type {
		case "text_delta":
			blk.Text.WriteString(frame.Delta.Text)
		case "input_json_delta":
			blk.Input.WriteString(frame.Delta.PartialJSON)
		}
	}
}

func (a *ResponseAccumulator) buildAnthropic() json.RawMessage {
	if len(a.antBlocks) == 0 {
		return nil
	}
	type textBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type toolBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	var blocks []json.RawMessage
	for _, idx := range a.antOrder {
		blk := a.antBlocks[idx]
		switch blk.BlockType {
		case "text":
			b, _ := json.Marshal(textBlock{Type: "text", Text: blk.Text.String()})
			blocks = append(blocks, b)
		case "tool_use":
			inputRaw := json.RawMessage("{}")
			if blk.Input.Len() > 0 {
				raw := blk.Input.String()
				var check json.RawMessage
				if json.Unmarshal([]byte(raw), &check) == nil {
					inputRaw = check
				}
			}
			b, _ := json.Marshal(toolBlock{
				Type:  "tool_use",
				ID:    blk.ToolID,
				Name:  blk.ToolName,
				Input: inputRaw,
			})
			blocks = append(blocks, b)
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	type msg struct {
		Role    string            `json:"role"`
		Content []json.RawMessage `json:"content"`
	}
	out, _ := json.Marshal(msg{Role: "assistant", Content: blocks})
	return out
}

// ============================================================================
// Gemini
// ============================================================================

func (a *ResponseAccumulator) addGemini(data []byte) {
	var chunk struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil || len(chunk.Candidates) == 0 {
		return
	}
	for _, p := range chunk.Candidates[0].Content.Parts {
		a.gemText.WriteString(p.Text)
	}
}

// ============================================================================
// OpenAI Responses (streaming)
// ============================================================================
//
// Relevant event types:
//   response.output_text.delta      → append text to output_index/content_index
//   response.function_call_arguments.delta → append args to output_index
//   response.output_item.done       → capture function_call metadata (id/name)

func (a *ResponseAccumulator) addOpenAIResponses(data []byte) {
	var frame struct {
		Type         string `json:"type"`
		OutputIndex  int    `json:"output_index"`
		ContentIndex int    `json:"content_index"`
		Delta        string `json:"delta"`
		Item         *struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"item"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		return
	}

	switch frame.Type {
	case "response.output_text.delta":
		out := a.ensureRespOutput(frame.OutputIndex)
		out.itemType = "message"
		if _, ok := out.textParts[frame.ContentIndex]; !ok {
			out.textParts[frame.ContentIndex] = &strings.Builder{}
			out.partOrder = append(out.partOrder, frame.ContentIndex)
		}
		out.textParts[frame.ContentIndex].WriteString(frame.Delta)

	case "response.function_call_arguments.delta":
		out := a.ensureRespOutput(frame.OutputIndex)
		out.itemType = "function_call"
		out.funcArgs.WriteString(frame.Delta)

	case "response.output_item.done":
		if frame.Item == nil || frame.Item.Type != "function_call" {
			return
		}
		out := a.ensureRespOutput(frame.OutputIndex)
		out.itemType = "function_call"
		if out.callID == "" {
			out.callID = frame.Item.CallID
		}
		if out.funcName == "" {
			out.funcName = frame.Item.Name
		}
		// If arguments were not streamed incrementally, use the done item's value.
		if out.funcArgs.Len() == 0 && frame.Item.Arguments != "" {
			out.funcArgs.WriteString(frame.Item.Arguments)
		}
	}
}

func (a *ResponseAccumulator) ensureRespOutput(idx int) *respOutputState {
	out, ok := a.respOutputs[idx]
	if !ok {
		out = &respOutputState{textParts: make(map[int]*strings.Builder)}
		a.respOutputs[idx] = out
		a.respOrder = append(a.respOrder, idx)
	}
	return out
}

func (a *ResponseAccumulator) buildOpenAIResponses() json.RawMessage {
	if len(a.respOutputs) == 0 {
		return nil
	}

	type textContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type msgItem struct {
		Type    string        `json:"type"`
		Role    string        `json:"role"`
		Content []textContent `json:"content"`
	}
	type funcItem struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id,omitempty"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}

	var items []json.RawMessage
	for _, idx := range a.respOrder {
		out := a.respOutputs[idx]
		switch out.itemType {
		case "message":
			var parts []textContent
			for _, ci := range out.partOrder {
				if sb := out.textParts[ci]; sb != nil && sb.Len() > 0 {
					parts = append(parts, textContent{Type: "output_text", Text: sb.String()})
				}
			}
			if len(parts) == 0 {
				continue
			}
			b, _ := json.Marshal(msgItem{Type: "message", Role: "assistant", Content: parts})
			items = append(items, b)
		case "function_call":
			if out.funcName == "" && out.funcArgs.Len() == 0 {
				continue
			}
			args := out.funcArgs.String()
			if args == "" {
				args = "{}"
			}
			b, _ := json.Marshal(funcItem{
				Type:      "function_call",
				CallID:    out.callID,
				Name:      out.funcName,
				Arguments: args,
			})
			items = append(items, b)
		}
	}
	if len(items) == 0 {
		return nil
	}
	// Return first item (mirrors ExtractSyncResponseMessage for Responses).
	return items[0]
}
