package audit

import (
	"encoding/json"
	"strings"
	"testing"

	"xiaodou/uni-ai-api/internal/domain"
)

// ============================================================================
// OpenAI Responses streaming accumulator
// ============================================================================

func TestResponsesAccumulatorText(t *testing.T) {
	acc := NewResponseAccumulator(domain.ProtocolOpenAIResponses)

	chunks := []string{
		`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello"}`,
		`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":", world"}`,
	}
	for _, c := range chunks {
		acc.AddChunk([]byte(c))
	}

	out := acc.Build()
	if out == nil {
		t.Fatal("Build returned nil for text accumulation")
	}
	var msg struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "message" {
		t.Errorf("type = %q, want message", msg.Type)
	}
	if msg.Role != "assistant" {
		t.Errorf("role = %q, want assistant", msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(msg.Content))
	}
	if msg.Content[0].Text != "Hello, world" {
		t.Errorf("text = %q, want %q", msg.Content[0].Text, "Hello, world")
	}
}

func TestResponsesAccumulatorFunctionCall(t *testing.T) {
	acc := NewResponseAccumulator(domain.ProtocolOpenAIResponses)

	chunks := []string{
		`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"k\""}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"delta":":\"v\"}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"my_func","arguments":""}}`,
	}
	for _, c := range chunks {
		acc.AddChunk([]byte(c))
	}

	out := acc.Build()
	if out == nil {
		t.Fatal("Build returned nil for function call")
	}
	var fn struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(out, &fn); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fn.Type != "function_call" {
		t.Errorf("type = %q, want function_call", fn.Type)
	}
	if fn.Name != "my_func" {
		t.Errorf("name = %q, want my_func", fn.Name)
	}
	if !strings.Contains(fn.Arguments, `"k"`) {
		t.Errorf("arguments = %q, expected to contain incremental deltas", fn.Arguments)
	}
}

func TestResponsesAccumulatorFunctionCallDoneArgs(t *testing.T) {
	// When function_call_arguments.delta events are absent, fall back to
	// the arguments provided in the output_item.done event.
	acc := NewResponseAccumulator(domain.ProtocolOpenAIResponses)

	acc.AddChunk([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"c1","name":"search","arguments":"{\"q\":\"go\"}}"}}`))

	out := acc.Build()
	if out == nil {
		t.Fatal("Build returned nil")
	}
	var fn struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	_ = json.Unmarshal(out, &fn)
	if fn.Name != "search" {
		t.Errorf("name = %q, want search", fn.Name)
	}
}

func TestResponsesAccumulatorEmptyReturnsNil(t *testing.T) {
	acc := NewResponseAccumulator(domain.ProtocolOpenAIResponses)
	if acc.Build() != nil {
		t.Error("Build should return nil when nothing was accumulated")
	}
}

func TestResponsesAccumulatorIgnoresUnknownEvents(t *testing.T) {
	acc := NewResponseAccumulator(domain.ProtocolOpenAIResponses)
	acc.AddChunk([]byte(`{"type":"response.created","response":{}}`))
	acc.AddChunk([]byte(`{"type":"response.done","response":{}}`))
	if acc.Build() != nil {
		t.Error("unknown events should not produce output")
	}
}

func TestResponsesAccumulatorMultipleContentParts(t *testing.T) {
	acc := NewResponseAccumulator(domain.ProtocolOpenAIResponses)

	acc.AddChunk([]byte(`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Part A"}`))
	acc.AddChunk([]byte(`{"type":"response.output_text.delta","output_index":0,"content_index":1,"delta":"Part B"}`))

	out := acc.Build()
	if out == nil {
		t.Fatal("Build returned nil")
	}
	var msg struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("content parts = %d, want 2", len(msg.Content))
	}
	if msg.Content[0].Text != "Part A" || msg.Content[1].Text != "Part B" {
		t.Errorf("unexpected content: %+v", msg.Content)
	}
}
