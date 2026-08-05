package formats

import "testing"

func TestExtractReasoningEffort(t *testing.T) {
	cases := []struct {
		name string
		src  FormatID
		body string
		want string
	}{
		{"openai chat effort", FormatOpenAIChat, `{"model":"gpt-5","reasoning_effort":"high"}`, "high"},
		{"openai chat minimal 归一为 low", FormatOpenAIChat, `{"reasoning_effort":"minimal"}`, "low"},
		{"openai chat 非法值", FormatOpenAIChat, `{"reasoning_effort":"turbo"}`, ""},
		{"openai chat 未声明", FormatOpenAIChat, `{"model":"gpt-5"}`, ""},
		{"responses effort", FormatOpenAIResponses, `{"reasoning":{"effort":"xhigh"}}`, "xhigh"},
		{"responses budget 反映射", FormatOpenAIResponses, `{"reasoning":{"budget_tokens":2048}}`, "medium"},
		{"responses budget 0 视为未启用", FormatOpenAIResponses, `{"reasoning":{"budget_tokens":0}}`, ""},
		{"claude output_config 优先", FormatClaudeMessages, `{"output_config":{"effort":"max"},"thinking":{"type":"enabled","budget_tokens":1280}}`, "max"},
		{"claude budget 反映射", FormatClaudeMessages, `{"thinking":{"type":"enabled","budget_tokens":8000}}`, "xhigh"},
		{"claude thinking disabled", FormatClaudeMessages, `{"thinking":{"type":"disabled","budget_tokens":8000}}`, ""},
		{"gemini thinkingBudget", FormatGeminiGenerate, `{"generationConfig":{"thinkingConfig":{"thinkingBudget":1280}}}`, "low"},
		{"gemini snake_case", FormatGeminiGenerate, `{"generation_config":{"thinking_config":{"thinking_budget":4096}}}`, "high"},
		{"gemini budget 0 视为关闭", FormatGeminiGenerate, `{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`, ""},
		{"非法 JSON 容错", FormatOpenAIChat, `not-json`, ""},
		{"空 body 容错", FormatOpenAIChat, ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractReasoningEffort(tc.src, []byte(tc.body)); got != tc.want {
				t.Fatalf("ExtractReasoningEffort(%s) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
