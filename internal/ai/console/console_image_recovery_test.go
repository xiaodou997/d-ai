package console

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/imageedit"
)

func TestConsoleImageEditTaskInputPersistsCanonicalRequestForRecovery(t *testing.T) {
	editRequest := imageedit.Request{
		Model:  "gpt-image-1",
		Prompt: "private rendered app prompt",
		Images: []imageedit.Source{{URL: "data:image/png;base64,aW1hZ2UtYnl0ZXM="}},
		Mask:   &imageedit.Source{URL: "data:image/png;base64,bWFzay1ieXRlcw=="},
	}

	input, err := consoleImageInputFromRequest("edit", consoleImageGenerateRequest{
		Operation: "edit",
		ModelCode: "gpt-image-1",
		Prompt:    "retouch",
	}, editRequest)
	if err != nil {
		t.Fatalf("prepare persisted task input: %v", err)
	}
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal persisted task input: %v", err)
	}
	var stored struct {
		EditRequest map[string]any `json:"edit_request"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("unmarshal persisted task input: %v", err)
	}
	if _, ok := stored.EditRequest["model"]; ok {
		t.Fatalf("persisted edit request exposed model: %#v", stored.EditRequest)
	}
	if _, ok := stored.EditRequest["prompt"]; ok {
		t.Fatalf("persisted edit request exposed rendered prompt: %#v", stored.EditRequest)
	}
	recovered, err := decodePersistedConsoleImageEditRequest(payload)
	if err != nil {
		t.Fatalf("decode persisted image edit request: %v", err)
	}
	if len(recovered.Images) != 1 || string(recovered.Images[0].Data) != "image-bytes" || recovered.Images[0].MIMEType != "image/png" {
		t.Fatalf("recovered images = %#v", recovered.Images)
	}
	if recovered.Mask == nil || string(recovered.Mask.Data) != "mask-bytes" || recovered.Mask.MIMEType != "image/png" {
		t.Fatalf("recovered mask = %#v", recovered.Mask)
	}
}
