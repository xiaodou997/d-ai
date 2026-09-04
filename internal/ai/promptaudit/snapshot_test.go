package promptaudit

import (
	"strings"
	"testing"
)

func TestExtractSnapshotScansClientControlledRolesAndPrioritizesLatestUser(t *testing.T) {
	in := Input{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"system","content":"policy"},{"role":"assistant","content":"prior output"},{"role":"tool","content":"tool text"},{"role":"user","content":"latest attack"}]}`)}
	got, err := ExtractSnapshot(in, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.ScanText, "latest attack") {
		t.Fatalf("scan text=%q", got.ScanText)
	}
	if got.MessageCount != 4 {
		t.Fatalf("message count=%d", got.MessageCount)
	}
}

func TestExtractSnapshotLatestTurnIncludesPreviousAssistant(t *testing.T) {
	in := Input{Protocol: "anthropic_messages", Body: []byte(`{"system":"sys","messages":[{"role":"user","content":"old"},{"role":"assistant","content":"previous output"},{"role":"user","content":"current"}]}`)}
	got, err := ExtractSnapshot(in, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanText != "current\n\nprevious output" {
		t.Fatalf("scan text=%q", got.ScanText)
	}
}

func TestRedactedPreviewNeverKeepsShortPromptAndMasksSecrets(t *testing.T) {
	if got := RedactedPreview("short confidential"); got != "***" {
		t.Fatalf("short preview=%q", got)
	}
	got := RedactedPreview("contact alice@example.com with token: abcdefghijklmnop and explain this sufficiently long request")
	if strings.Contains(got, "alice@example.com") || strings.Contains(got, "abcdefghijklmnop") {
		t.Fatalf("secret leaked: %q", got)
	}
}

func TestImageSnapshotDoesNotIncludeImagePayload(t *testing.T) {
	in := Input{Protocol: "openai_images", Body: []byte(`{"prompt":"draw a castle","image":"data:image/png;base64,SECRET"}`)}
	got, err := ExtractSnapshot(in, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanText != "draw a castle" {
		t.Fatalf("scan text=%q", got.ScanText)
	}
}
