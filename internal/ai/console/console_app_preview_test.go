package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/internal/ai/application"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
)

type recordingPreviewGateway struct {
	replay gateway.ReplayInput
}

func (g *recordingPreviewGateway) Replay(_ context.Context, input gateway.ReplayInput) gateway.ReplayResult {
	g.replay = input
	return gateway.ReplayResult{StatusCode: http.StatusOK}
}

func (*recordingPreviewGateway) ExecuteRuntime(
	http.ResponseWriter,
	*http.Request,
	domain.CapabilityType,
	gateway.RuntimeOverride,
	*coreidentity.Subject,
	bool,
) coreruntime.Result {
	return coreruntime.Result{}
}

func TestConsoleSubjectForPreviewUsesAppPreviewSource(t *testing.T) {
	t.Parallel()

	subject := &coreidentity.Subject{
		Scope:         coreidentity.ScopeUser,
		TenantID:      "tenant-1",
		UserID:        "user-1",
		RequestSource: coreidentity.RequestSourceWebChat,
	}

	got := consoleSubjectForPreview(subject)
	if got == nil {
		t.Fatal("consoleSubjectForPreview returned nil")
	}
	if got == subject {
		t.Fatal("consoleSubjectForPreview should return a copy")
	}
	if got.RequestSource != coreidentity.RequestSourceAppPreview {
		t.Fatalf("request source = %q, want %q", got.RequestSource, coreidentity.RequestSourceAppPreview)
	}
	if subject.RequestSource != coreidentity.RequestSourceWebChat {
		t.Fatalf("source subject mutated to %q", subject.RequestSource)
	}
}

func TestExecuteConsoleImagePreviewPreservesAppPreviewSource(t *testing.T) {
	t.Parallel()

	recorder := &recordingPreviewGateway{}
	s := &Console{gateway: recorder}
	req := httptest.NewRequest(http.MethodPost, "/runtime/v1/app-previews/app-1", nil)

	s.executeConsoleImagePreview(req, gateway.ReplayInput{
		Subject: coreidentity.Subject{
			TenantID:      "tenant-1",
			UserID:        "user-1",
			RequestSource: coreidentity.RequestSourceAppPreview,
		},
	})

	if recorder.replay.Subject.RequestSource != coreidentity.RequestSourceAppPreview {
		t.Fatalf("replay request source = %q, want %q", recorder.replay.Subject.RequestSource, coreidentity.RequestSourceAppPreview)
	}
}

func TestValidatePreviewAttachments(t *testing.T) {
	t.Parallel()

	cfg := application.ChatRuntimeConfig{AllowAttachments: true}
	if err := validatePreviewAttachments([]runAttachment{{URL: "https://example.test/a.png"}}, cfg); err != nil {
		t.Fatalf("validatePreviewAttachments returned error for valid input: %v", err)
	}
	if err := validatePreviewAttachments([]runAttachment{{URL: "ftp://example.test/a.png"}}, cfg); err == nil {
		t.Fatal("expected invalid scheme error")
	}
	tooMany := make([]runAttachment, 0, application.MaxAppAttachments+1)
	for i := 0; i < application.MaxAppAttachments+1; i++ {
		tooMany = append(tooMany, runAttachment{URL: "https://example.test/a.png"})
	}
	if err := validatePreviewAttachments(tooMany, cfg); err == nil {
		t.Fatal("expected too many attachments error")
	}
	if err := validatePreviewAttachments(
		[]runAttachment{{URL: "https://example.test/a.png"}},
		application.ChatRuntimeConfig{AllowAttachments: false},
	); err == nil {
		t.Fatal("expected attachments disabled error")
	}
}

func TestBuildConsoleProtocolBodyWithStreamIncludesAttachmentsForOpenAIChat(t *testing.T) {
	t.Parallel()

	body, clientPath, err := buildConsoleProtocolBodyWithStream(
		domain.ProtocolOpenAIChat,
		"gpt-4o",
		[]consoleChatMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "hello"}},
		0.7,
		512,
		false,
		[]runAttachment{
			{URL: "https://example.test/cat.png", MIMEType: "image/png"},
			{URL: "https://example.test/spec.pdf", Name: "spec.pdf"},
		},
	)
	if err != nil {
		t.Fatalf("buildConsoleProtocolBodyWithStream: %v", err)
	}
	if clientPath != "/v1/chat/completions" {
		t.Fatalf("clientPath = %q, want /v1/chat/completions", clientPath)
	}

	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(payload.Messages))
	}
	content, ok := payload.Messages[1].Content.([]any)
	if !ok {
		t.Fatalf("user content type = %T, want []any", payload.Messages[1].Content)
	}
	if len(content) != 3 {
		t.Fatalf("attachment content len = %d, want 3", len(content))
	}
	last, ok := content[2].(map[string]any)
	if !ok {
		t.Fatalf("last content part type = %T, want map[string]any", content[2])
	}
	if last["type"] != "file" {
		t.Fatalf("last content part type field = %v, want file", last["type"])
	}
}

func TestBuildConsoleProtocolBodyWithStreamRejectsAttachmentsForNonOpenAIChat(t *testing.T) {
	t.Parallel()

	_, _, err := buildConsoleProtocolBodyWithStream(
		domain.ProtocolAnthropicMessages,
		"claude",
		[]consoleChatMessage{{Role: "user", Content: "hello"}},
		0.2,
		256,
		false,
		[]runAttachment{{URL: "https://example.test/cat.png"}},
	)
	if err == nil {
		t.Fatal("expected attachments protocol error")
	}
}

func TestExtractConsoleAssistantTextFromSync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol domain.UpstreamProtocol
		body     string
		want     string
	}{
		{
			name:     "openai chat",
			protocol: domain.ProtocolOpenAIChat,
			body:     `{"choices":[{"message":{"content":"hello world"}}]}`,
			want:     "hello world",
		},
		{
			name:     "openai responses",
			protocol: domain.ProtocolOpenAIResponses,
			body:     `{"output":[{"content":[{"type":"output_text","text":"hello "},{"type":"text","text":"world"}]}]}`,
			want:     "hello world",
		},
		{
			name:     "anthropic",
			protocol: domain.ProtocolAnthropicMessages,
			body:     `{"content":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]}`,
			want:     "hello world",
		},
		{
			name:     "gemini",
			protocol: domain.ProtocolGeminiGenerate,
			body:     `{"candidates":[{"content":{"parts":[{"text":"hello world"}]}}]}`,
			want:     "hello world",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := extractConsoleAssistantTextFromSync([]byte(tc.body), tc.protocol); got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreviewImagesFromBody(t *testing.T) {
	t.Parallel()

	got := previewImagesFromBody([]byte(`{"data":[{"url":"https://example.test/cat.png"},{"b64_json":"aGVsbG8="}]}`))
	if len(got) != 2 {
		t.Fatalf("images len = %d, want 2", len(got))
	}
	if got[0]["url"] != "https://example.test/cat.png" {
		t.Fatalf("first image = %+v", got[0])
	}
}

func TestUsageMapFromResult(t *testing.T) {
	t.Parallel()

	result := &coreruntime.Result{
		Usage: map[string]any{
			"prompt_tokens":      120,
			"completion_tokens":  45,
			"total_tokens":       165,
			"cache_read_tokens":  10,
			"cache_write_tokens": 8,
			"reasoning_tokens":   6,
			"image_count":        2,
			"image_resolution":   "1024x1024",
		},
	}
	got := usageMapFromResult(result)
	if got["prompt_tokens"] != 120 {
		t.Fatalf("prompt_tokens = %v, want 120", got["prompt_tokens"])
	}
	if got["total_tokens"] != 165 {
		t.Fatalf("total_tokens = %v, want 165", got["total_tokens"])
	}
	if got["image_count"] != 2 {
		t.Fatalf("image_count = %v, want 2", got["image_count"])
	}
	if got["image_resolution"] != "1024x1024" {
		t.Fatalf("image_resolution = %v, want 1024x1024", got["image_resolution"])
	}
}

func TestUsageMapFromResultReturnsNilForEmptyUsage(t *testing.T) {
	t.Parallel()

	if got := usageMapFromResult(&coreruntime.Result{}); got != nil {
		t.Fatalf("usageMapFromResult = %+v, want nil", got)
	}
}

func TestWriteConsolePreviewErrorReadsJSONBodyMessage(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeConsolePreviewError(rec, http.StatusBadGateway, []byte(`{"error":{"message":"upstream preview failed"}}`))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Message != "upstream preview failed" {
		t.Fatalf("message = %q, want %q", resp.Message, "upstream preview failed")
	}
}
