package console

import (
	"encoding/json"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

func TestConsoleImageGenerationIgnoresQualityAndStyle(t *testing.T) {
	var req consoleImageGenerateRequest
	if err := json.Unmarshal([]byte(`{
		"model":"gpt-image-1",
		"prompt":"poster",
		"quality":"high",
		"style":"vivid"
	}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	body, err := buildConsoleImageBody(req, req.ModelCode, req.Prompt, nil)
	if err != nil {
		t.Fatalf("buildConsoleImageBody: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	for _, field := range []string{"quality", "style"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("console request must ignore %s: %#v", field, payload)
		}
	}
}

func TestDefaultConsoleImagePolicyDefaultsToForceSync(t *testing.T) {
	t.Parallel()

	policy := defaultConsoleImagePolicy()
	if policy.StreamMode != domain.ImageStreamModeForceSync {
		t.Fatalf("default stream mode = %q, want %q", policy.StreamMode, domain.ImageStreamModeForceSync)
	}
}

func TestParseConsoleImagePolicyReadsStreamMode(t *testing.T) {
	t.Parallel()

	policy := parseConsoleImagePolicy([]byte(`{
		"image_generation": {"stream_mode": "auto"}
	}`))
	if policy.StreamMode != domain.ImageStreamModeAuto {
		t.Fatalf("stream mode = %q, want %q", policy.StreamMode, domain.ImageStreamModeAuto)
	}
}

func TestEffectiveConsoleImageResponseFormatDefaultsToURL(t *testing.T) {
	t.Parallel()

	if got := effectiveConsoleImageResponseFormat(""); got != domain.ImageResponseFormatURL {
		t.Fatalf("effectiveConsoleImageResponseFormat(\"\") = %q, want %q", got, domain.ImageResponseFormatURL)
	}
	if got := effectiveConsoleImageResponseFormat(domain.ImageResponseFormatURL); got != domain.ImageResponseFormatURL {
		t.Fatalf("effectiveConsoleImageResponseFormat(url) = %q, want %q", got, domain.ImageResponseFormatURL)
	}
}

func TestApplyConsoleImagePolicyUsesURLForConsoleClient(t *testing.T) {
	policy := defaultConsoleImagePolicy()
	req := consoleImageGenerateRequest{}

	applyConsoleImagePolicy(&req, policy, false)

	if req.ResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("console response format = %q, want url", req.ResponseFormat)
	}
}

func TestConsoleImageEditAppAllowsOnlyPerCallInputs(t *testing.T) {
	stream := true
	req := consoleImageGenerateRequest{
		AgentID:        "app-1",
		Prompt:         "retouch",
		Images:         []consoleImageSource{{ImageURL: "https://example.com/reference.png"}},
		ResponseFormat: domain.ImageResponseFormatURL,
		Stream:         &stream,
	}
	if consoleImageEditOverridesApp(req) {
		t.Fatal("prompt, images, response_format and stream must remain caller-controlled")
	}
	req.Size = "1536x1024"
	if !consoleImageEditOverridesApp(req) {
		t.Fatal("size must be locked by the app")
	}
}

func TestDecodeConsoleImageEditRequestUsesOfficialJSONSchema(t *testing.T) {
	stream := true
	decoded, err := decodeConsoleImageEditRequest(consoleImageGenerateRequest{
		Prompt:         "retouch",
		Images:         []consoleImageSource{{ImageURL: "https://example.com/reference.png"}},
		Mask:           &consoleImageSource{ImageURL: "https://example.com/mask.png"},
		ResponseFormat: domain.ImageResponseFormatURL,
		Stream:         &stream,
	})
	if err != nil {
		t.Fatalf("decodeConsoleImageEditRequest: %v", err)
	}
	if len(decoded.Images) != 1 || decoded.Images[0].URL != "https://example.com/reference.png" {
		t.Fatalf("images = %#v", decoded.Images)
	}
	if decoded.Mask == nil || decoded.Mask.URL != "https://example.com/mask.png" {
		t.Fatalf("mask = %#v", decoded.Mask)
	}
	if !decoded.Stream || decoded.ResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("request options = %+v", decoded)
	}
}

func TestConsoleImageSubjectPreservesAppIdentity(t *testing.T) {
	agent := &consoleChatAgentRuntime{
		ID:            "app-1",
		Name:          "Image Editor",
		OwnerType:     "tenant",
		OwnerTenantID: "publisher-tenant",
		OwnerUserID:   "publisher-user",
	}

	subject := consoleImageSubjectFromAgent(&coreidentity.Subject{
		TenantID: "caller-tenant",
		UserID:   "caller-user",
	}, "app-group", agent)
	if subject == nil {
		t.Fatal("runtime subject is nil")
	}
	if subject.ForcedGroupID != "app-group" {
		t.Fatalf("forced group = %q, want app-group", subject.ForcedGroupID)
	}
	if subject.AppID != "app-1" || subject.AppName != "Image Editor" {
		t.Fatalf("app identity = %q/%q", subject.AppID, subject.AppName)
	}
	if subject.AppOwnerType != "tenant" || subject.AppOwnerTenantID != "publisher-tenant" || subject.AppOwnerUserID != "publisher-user" {
		t.Fatalf("app owner identity = %q/%q/%q", subject.AppOwnerType, subject.AppOwnerTenantID, subject.AppOwnerUserID)
	}
}
