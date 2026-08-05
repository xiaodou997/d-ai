package formats

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

// 转换矩阵测试：构造一个规范化请求/响应，emit 到每个生成类格式，再 parse 回 IR，
// 校验跨任意格式对的语义不变量（模型、system、用户文本、工具、tool_use/tool_result、
// 用量、停止原因）。这验证 canonical IR 作为「中枢」对全部 4×4 转换对成立。

var allFormats = []FormatID{
	FormatOpenAIChat, FormatOpenAIResponses, FormatClaudeMessages, FormatGeminiGenerate,
}

// requestCore 抽取请求的格式无关语义指纹。
type requestCore struct {
	model       string
	system      string
	userTexts   []string
	toolUseIDs  []string
	toolResults []string
	toolDefs    []string
	maxTokens   uint64
}

func extractRequestCore(r *Request) requestCore {
	c := requestCore{model: r.Model, system: strings.TrimSpace(r.System)}
	if r.Generation.MaxTokens != nil {
		c.maxTokens = *r.Generation.MaxTokens
	}
	for _, t := range r.Tools {
		c.toolDefs = append(c.toolDefs, t.Name)
	}
	for _, m := range r.Messages {
		for _, b := range m.Content {
			switch b.Kind {
			case BlockText:
				if m.Role == RoleUser && strings.TrimSpace(b.Text) != "" {
					c.userTexts = append(c.userTexts, b.Text)
				}
			case BlockToolUse:
				c.toolUseIDs = append(c.toolUseIDs, b.ID+":"+b.Name)
			case BlockToolResult:
				c.toolResults = append(c.toolResults, b.ToolUseID)
			}
		}
	}
	sort.Strings(c.userTexts)
	sort.Strings(c.toolUseIDs)
	sort.Strings(c.toolResults)
	sort.Strings(c.toolDefs)
	return c
}

func sampleRequest() *Request {
	maxTok := uint64(2048)
	temp := 0.7
	return &Request{
		Model:        "logical-model",
		System:       "you are a helpful assistant",
		Instructions: []Instruction{{Role: RoleSystem, Text: "you are a helpful assistant"}},
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{{Kind: BlockText, Text: "what is the weather in SF?"}}},
			{Role: RoleAssistant, Content: []ContentBlock{{
				Kind: BlockToolUse, ID: "call_abc", Name: "get_weather",
				Input: json.RawMessage(`{"city":"SF"}`)}}},
			{Role: RoleTool, Content: []ContentBlock{{
				Kind: BlockToolResult, ToolUseID: "call_abc", HasOutput: true,
				Output: json.RawMessage(`"sunny"`), ContentText: "sunny", HasContentText: true}}},
		},
		Generation: GenerationConfig{MaxTokens: &maxTok, Temperature: &temp},
		Tools: []ToolDefinition{{
			Name: "get_weather", Description: "get weather",
			Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}},
		ToolChoice: &ToolChoice{Kind: ToolChoiceAuto},
	}
}

func TestRequestMatrixRoundTrip(t *testing.T) {
	want := extractRequestCore(sampleRequest())
	for _, dst := range allFormats {
		body, err := EmitRequest(dst, sampleRequest(), "", false)
		if err != nil {
			t.Fatalf("emit %s: %v", dst, err)
		}
		if !json.Valid(body) {
			t.Fatalf("emit %s produced invalid JSON: %s", dst, body)
		}
		got, err := ParseRequest(dst, body)
		if err != nil {
			t.Fatalf("parse %s: %v\nbody=%s", dst, err, body)
		}
		gotCore := extractRequestCore(got)
		if gotCore.model != want.model {
			t.Errorf("[%s] model = %q, want %q", dst, gotCore.model, want.model)
		}
		if gotCore.system != want.system {
			t.Errorf("[%s] system = %q, want %q", dst, gotCore.system, want.system)
		}
		if strings.Join(gotCore.userTexts, "|") != strings.Join(want.userTexts, "|") {
			t.Errorf("[%s] userTexts = %v, want %v", dst, gotCore.userTexts, want.userTexts)
		}
		if strings.Join(gotCore.toolUseIDs, "|") != strings.Join(want.toolUseIDs, "|") {
			t.Errorf("[%s] toolUse = %v, want %v", dst, gotCore.toolUseIDs, want.toolUseIDs)
		}
		if strings.Join(gotCore.toolResults, "|") != strings.Join(want.toolResults, "|") {
			t.Errorf("[%s] toolResults = %v, want %v", dst, gotCore.toolResults, want.toolResults)
		}
		if strings.Join(gotCore.toolDefs, "|") != strings.Join(want.toolDefs, "|") {
			t.Errorf("[%s] toolDefs = %v, want %v", dst, gotCore.toolDefs, want.toolDefs)
		}
		if gotCore.maxTokens != want.maxTokens {
			t.Errorf("[%s] maxTokens = %d, want %d", dst, gotCore.maxTokens, want.maxTokens)
		}
	}
}

// TestRequestCrossConvert 校验任意 src→dst 直接转换（ConvertRequest）后 dst 可解析、
// 且核心语义一致。
func TestRequestCrossConvert(t *testing.T) {
	want := extractRequestCore(sampleRequest())
	srcBodies := map[FormatID]json.RawMessage{}
	for _, f := range allFormats {
		b, err := EmitRequest(f, sampleRequest(), "", false)
		if err != nil {
			t.Fatalf("seed emit %s: %v", f, err)
		}
		srcBodies[f] = b
	}
	for _, src := range allFormats {
		for _, dst := range allFormats {
			out, err := ConvertRequest(src, dst, srcBodies[src], "", false)
			if err != nil {
				t.Fatalf("convert %s->%s: %v", src, dst, err)
			}
			got, err := ParseRequest(dst, out)
			if err != nil {
				t.Fatalf("reparse %s (from %s): %v\n%s", dst, src, err, out)
			}
			gc := extractRequestCore(got)
			if gc.model != want.model || gc.system != want.system ||
				strings.Join(gc.userTexts, "|") != strings.Join(want.userTexts, "|") ||
				strings.Join(gc.toolUseIDs, "|") != strings.Join(want.toolUseIDs, "|") ||
				strings.Join(gc.toolResults, "|") != strings.Join(want.toolResults, "|") ||
				strings.Join(gc.toolDefs, "|") != strings.Join(want.toolDefs, "|") {
				t.Errorf("%s->%s core mismatch:\n got=%+v\nwant=%+v", src, dst, gc, want)
			}
		}
	}
}

// responseCore 抽取响应的格式无关语义指纹。
type responseCore struct {
	model        string
	text         string
	toolUseNames []string
	stop         StopReason
	inputTokens  uint64
	outputTokens uint64
}

func extractResponseCore(r *Response) responseCore {
	c := responseCore{model: r.Model}
	if r.StopReason != nil {
		c.stop = *r.StopReason
	}
	if r.Usage != nil {
		c.inputTokens = r.Usage.InputTokens
		c.outputTokens = r.Usage.OutputTokens
	}
	var texts []string
	for _, b := range r.Content {
		switch b.Kind {
		case BlockText:
			if strings.TrimSpace(b.Text) != "" {
				texts = append(texts, b.Text)
			}
		case BlockToolUse:
			c.toolUseNames = append(c.toolUseNames, b.Name)
		}
	}
	c.text = strings.Join(texts, "")
	sort.Strings(c.toolUseNames)
	return c
}

func sampleResponse() *Response {
	stop := StopToolUse
	return &Response{
		ID:    "chatcmpl-xyz",
		Model: "logical-model",
		Content: []ContentBlock{
			{Kind: BlockText, Text: "Let me check the weather."},
			{Kind: BlockToolUse, ID: "call_abc", Name: "get_weather", Input: json.RawMessage(`{"city":"SF"}`)},
		},
		StopReason: &stop,
		Usage:      &Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	}
}

func TestResponseMatrixRoundTrip(t *testing.T) {
	want := extractResponseCore(sampleResponse())
	for _, dst := range allFormats {
		body, err := EmitResponse(dst, sampleResponse())
		if err != nil {
			t.Fatalf("emit %s: %v", dst, err)
		}
		if !json.Valid(body) {
			t.Fatalf("emit %s invalid JSON: %s", dst, body)
		}
		got, err := ParseResponse(dst, body)
		if err != nil {
			t.Fatalf("parse %s: %v\n%s", dst, err, body)
		}
		gc := extractResponseCore(got)
		if gc.model != want.model {
			t.Errorf("[%s] model = %q, want %q", dst, gc.model, want.model)
		}
		if gc.text != want.text {
			t.Errorf("[%s] text = %q, want %q", dst, gc.text, want.text)
		}
		if strings.Join(gc.toolUseNames, "|") != strings.Join(want.toolUseNames, "|") {
			t.Errorf("[%s] toolUse = %v, want %v", dst, gc.toolUseNames, want.toolUseNames)
		}
		if gc.stop != want.stop {
			t.Errorf("[%s] stop = %q, want %q", dst, gc.stop, want.stop)
		}
		if gc.inputTokens != want.inputTokens || gc.outputTokens != want.outputTokens {
			t.Errorf("[%s] usage = %d/%d, want %d/%d", dst,
				gc.inputTokens, gc.outputTokens, want.inputTokens, want.outputTokens)
		}
	}
}

func TestResponseCrossConvert(t *testing.T) {
	want := extractResponseCore(sampleResponse())
	srcBodies := map[FormatID]json.RawMessage{}
	for _, f := range allFormats {
		b, _ := EmitResponse(f, sampleResponse())
		srcBodies[f] = b
	}
	for _, src := range allFormats {
		for _, dst := range allFormats {
			out, err := ConvertResponse(src, dst, srcBodies[src])
			if err != nil {
				t.Fatalf("convert %s->%s: %v", src, dst, err)
			}
			got, err := ParseResponse(dst, out)
			if err != nil {
				t.Fatalf("reparse %s (from %s): %v\n%s", dst, src, err, out)
			}
			gc := extractResponseCore(got)
			if gc.text != want.text ||
				strings.Join(gc.toolUseNames, "|") != strings.Join(want.toolUseNames, "|") ||
				gc.stop != want.stop ||
				gc.inputTokens != want.inputTokens || gc.outputTokens != want.outputTokens {
				t.Errorf("%s->%s core mismatch:\n got=%+v\nwant=%+v", src, dst, gc, want)
			}
		}
	}
}

// TestEmitRequestMappedModel 校验 mappedModel 覆盖逻辑（对齐上游真实模型名）。
func TestEmitRequestMappedModel(t *testing.T) {
	body, err := EmitRequest(FormatOpenAIChat, sampleRequest(), "gpt-real-4", false)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := ParseRequest(FormatOpenAIChat, body)
	if got.Model != "gpt-real-4" {
		t.Fatalf("mappedModel override failed: model = %q", got.Model)
	}
}

// TestClaudeToOpenAIChatShape 针对头号场景做一次具体形状校验：
// Claude /v1/messages 请求 → OpenAI chat 请求。
func TestClaudeToOpenAIChatShape(t *testing.T) {
	claudeBody := json.RawMessage(`{
		"model": "claude-sonnet-4",
		"system": "be concise",
		"max_tokens": 512,
		"messages": [{"role":"user","content":"hello"}]
	}`)
	out, err := ConvertRequest(FormatClaudeMessages, FormatOpenAIChat, claudeBody, "gpt-4o", false)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := decodeObject(out)
	if !ok {
		t.Fatalf("not an object: %s", out)
	}
	if getStr(obj, "model") != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", getStr(obj, "model"))
	}
	msgs, ok := decodeArray(field(obj, "messages"))
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages = %s, want 2 (system + user)", field(obj, "messages"))
	}
	first, _ := decodeObject(msgs[0])
	if getStr(first, "role") != "system" {
		t.Errorf("messages[0].role = %q, want system", getStr(first, "role"))
	}
	if getStr(first, "content") != "be concise" {
		t.Errorf("system content = %q", getStr(first, "content"))
	}
	if v, _ := asUint(field(obj, "max_completion_tokens")); v != 512 {
		t.Errorf("max_completion_tokens = %d, want 512", v)
	}
}

func TestOpenAIChatToClaudeUsesDefaultMaxTokens(t *testing.T) {
	openAIBody := json.RawMessage(`{
		"model": "claude-sonnet",
		"messages": [{"role":"user","content":"implement this feature"}]
	}`)
	out, err := ConvertRequest(FormatOpenAIChat, FormatClaudeMessages, openAIBody, "claude-sonnet-upstream", false)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := decodeObject(out)
	if !ok {
		t.Fatalf("not an object: %s", out)
	}
	if got, _ := asUint(field(obj, "max_tokens")); got != 65536 {
		t.Errorf("max_tokens = %d, want 65536", got)
	}
}

// TestMatrixHelpers 校验矩阵的协议映射与偏好分桶。
func TestMatrixHelpers(t *testing.T) {
	if f, ok := FormatIDForProtocol(domain.ProtocolAnthropicMessages); !ok || f != FormatClaudeMessages {
		t.Errorf("FormatIDForProtocol(anthropic) = %q,%v", f, ok)
	}
	if _, ok := FormatIDForProtocol(domain.ProtocolOpenAIEmbeddings); ok {
		t.Errorf("embeddings should not map to a generation format")
	}
	if !ConversionSupported(FormatClaudeMessages, FormatOpenAIChat) {
		t.Errorf("claude->openai:chat should be supported")
	}
	// 同格式 bucket 0 优先于跨家族 bucket 3
	b0, _, _ := CandidatePreference(FormatOpenAIChat, FormatOpenAIChat)
	b3, _, _ := CandidatePreference(FormatClaudeMessages, FormatGeminiGenerate)
	if b0 != 0 || b3 != 3 {
		t.Errorf("buckets = %d (want 0), %d (want 3)", b0, b3)
	}
}
