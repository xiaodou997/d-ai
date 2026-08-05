package riskcontrol

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestExtractLastUserText_OpenAIChatPlainString(t *testing.T) {
	messages := json.RawMessage(`[
		{"role":"system","content":"you are a bot"},
		{"role":"user","content":"hello there"}
	]`)
	got := ExtractLastUserText(domain.ProtocolOpenAIChat, messages)
	if got != "hello there" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractLastUserText_OpenAIChatBlocksSkipImages(t *testing.T) {
	messages := json.RawMessage(`[
		{"role":"user","content":[
			{"type":"text","text":"look at this"},
			{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}
		]}
	]`)
	got := ExtractLastUserText(domain.ProtocolOpenAIChat, messages)
	if got != "look at this" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractLastUserText_AnthropicMessages(t *testing.T) {
	messages := json.RawMessage(`[
		{"role":"user","content":"first turn"},
		{"role":"assistant","content":"reply"},
		{"role":"user","content":"second turn"}
	]`)
	got := ExtractLastUserText(domain.ProtocolAnthropicMessages, messages)
	if got != "second turn" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractLastUserText_GeminiParts(t *testing.T) {
	messages := json.RawMessage(`[
		{"role":"user","parts":[{"text":"hi"},{"text":"there"}]}
	]`)
	got := ExtractLastUserText(domain.ProtocolGeminiGenerate, messages)
	if got != "hi\nthere" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractLastUserText_OpenAIResponsesBareString(t *testing.T) {
	messages := json.RawMessage(`"just a plain prompt"`)
	got := ExtractLastUserText(domain.ProtocolOpenAIResponses, messages)
	if got != "just a plain prompt" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractLastUserText_IgnoresTrailingAssistantMessage(t *testing.T) {
	messages := json.RawMessage(`[
		{"role":"user","content":"question"},
		{"role":"assistant","content":"answer"}
	]`)
	got := ExtractLastUserText(domain.ProtocolOpenAIChat, messages)
	if got != "question" {
		t.Fatalf("got %q, want the last user turn not the trailing assistant reply", got)
	}
}

func TestExtractLastUserText_EmptyInput(t *testing.T) {
	if got := ExtractLastUserText(domain.ProtocolOpenAIChat, nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
