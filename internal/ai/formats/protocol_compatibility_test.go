package formats

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResponsesCreationTimeCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  FormatID
		body string
		want uint64
	}{
		{"Claude conversion", FormatClaudeMessages, `{"id":"msg-1","model":"claude","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`, 0},
		{"preserve Responses time", FormatOpenAIResponses, `{"id":"resp-1","model":"gpt","status":"completed","created_at":1700000000,"output":[]}`, 1700000000},
		{"repair null time", FormatOpenAIResponses, `{"id":"resp-1","model":"gpt","status":"completed","created_at":null,"output":[]}`, 0},
		{"preserve Chat time", FormatOpenAIChat, `{"id":"chatcmpl-1","model":"gpt","created":1700000001,"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`, 1700000001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := uint64(time.Now().Unix())
			body, err := ConvertResponse(tc.src, FormatOpenAIResponses, []byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			obj, _ := decodeObject(body)
			created, ok := asUint(field(obj, "created_at"))
			if !ok || created == 0 {
				t.Fatal("Responses object has no numeric creation time")
			}
			if tc.want != 0 && created != tc.want {
				t.Fatalf("creation time = %d, want %d", created, tc.want)
			}
			if tc.want == 0 && (created < before || created > uint64(time.Now().Unix())) {
				t.Fatal("fallback timestamp is outside response generation time")
			}
		})
	}
}

func TestResponsesStreamUsesOneCreationTime(t *testing.T) {
	provider, _ := NewStreamProvider(FormatClaudeMessages, "claude")
	emitter, _ := NewStreamEmitter(FormatOpenAIResponses)
	output := pumpStream(t, provider, emitter, []string{
		`data: {"type":"message_start","message":{"id":"msg-1","model":"claude","usage":{"input_tokens":1,"output_tokens":0}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		`data: {"type":"message_stop"}`,
	})
	events, _ := collectSSE(t, output)
	var timestamp uint64
	count := 0
	for _, event := range events {
		response, ok := decodeObject(field(event, "response"))
		if !ok {
			continue
		}
		created, ok := asUint(field(response, "created_at"))
		if !ok || created == 0 {
			t.Fatal("streamed Responses object missing creation time")
		}
		if timestamp != 0 && timestamp != created {
			t.Fatal("creation time changed within one stream")
		}
		timestamp = created
		count++
	}
	if count != 3 {
		t.Fatalf("checked %d response objects, want created, in_progress, completed", count)
	}
}

func TestGeminiThinkingUsesMappedModelProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, model, effort, level string
		budget                     uint64
	}{
		{"Gemini 4", "gemini-4-argon", "medium", "medium", 0},
		{"Gemini 3", "gemini-3-flash", "high", "high", 0},
		{"resource model", "projects/test/locations/global/publishers/google/models/gemini-4-argon", "low", "low", 0},
		{"cap stronger effort", "gemini-4-argon", "xhigh", "high", 0},
		{"Gemini 2.5", "gemini-2.5-pro", "high", "", 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(map[string]any{"model": "public-alias", "messages": []any{map[string]any{"role": "user", "content": "hi"}}, "reasoning_effort": tc.effort})
			output, err := ConvertRequest(FormatOpenAIChat, FormatGeminiGenerate, input, tc.model, true)
			if err != nil {
				t.Fatal(err)
			}
			root, _ := decodeObject(output)
			gc, _ := decodeObject(field(root, "generationConfig"))
			thinking, _ := decodeObject(field(gc, "thinkingConfig"))
			if tc.level != "" {
				if getStr(thinking, "thinkingLevel") != tc.level {
					t.Fatalf("unexpected thinking config: %s", field(gc, "thinkingConfig"))
				}
				if field(thinking, "thinkingBudget") != nil {
					t.Fatal("level and budget must not be sent together")
				}
			} else if budget, _ := asUint(field(thinking, "thinkingBudget")); budget != tc.budget {
				t.Fatalf("legacy budget = %d, want %d", budget, tc.budget)
			}
		})
	}
}

func TestGeminiThinkingLevelSurvivesCrossProtocolConversion(t *testing.T) {
	for _, src := range []string{
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH","includeThoughts":true}}}`,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generation_config":{"thinking_config":{"thinking_level":"high","include_thoughts":true}}}`,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"thinkingConfig":{"thinkingLevel":"high","includeThoughts":false}}}`,
	} {
		for _, dst := range []FormatID{FormatOpenAIChat, FormatOpenAIResponses, FormatClaudeMessages} {
			output, err := ConvertRequest(FormatGeminiGenerate, dst, []byte(src), "mapped", true)
			if err != nil {
				t.Fatal(err)
			}
			if effort := ExtractReasoningEffort(dst, output); effort != "high" {
				t.Fatalf("%s lost Gemini level: %s", dst, output)
			}
		}
		if effort := ExtractReasoningEffort(FormatGeminiGenerate, []byte(src)); effort != "high" {
			t.Fatal("usage metadata lost Gemini level")
		}
	}
}
