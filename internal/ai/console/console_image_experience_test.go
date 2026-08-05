package console

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"testing"
)

func TestRejectConsoleImageApplicationInputAllowsDirectModelRequest(t *testing.T) {
	err := rejectConsoleImageApplicationInput(
		[]byte(`{"model":"gpt-image-1","prompt":"poster"}`),
		"application/json",
	)
	if err != nil {
		t.Fatalf("direct model request rejected: %v", err)
	}
}

func TestRejectConsoleImageApplicationInputRejectsJSONApplicationFields(t *testing.T) {
	for _, body := range []string{
		`{"agent_id":"app-1","prompt":"poster"}`,
		`{"model":"gpt-image-1","prompt":"poster","variables":{"tone":"warm"}}`,
	} {
		if err := rejectConsoleImageApplicationInput([]byte(body), "application/json"); err == nil {
			t.Fatalf("application request was accepted: %s", body)
		}
	}
}

func TestRejectConsoleImageApplicationInputRejectsMultipartAgent(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("agent_id", "app-1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rejectConsoleImageApplicationInput(body.Bytes(), writer.FormDataContentType()); err == nil {
		t.Fatal("multipart application request was accepted")
	}
}

func TestConsoleImageTaskUsesApplication(t *testing.T) {
	appPayload, _ := json.Marshal(map[string]any{"agent_id": "app-1"})
	if !consoleImageTaskUsesApplication(appPayload) {
		t.Fatal("application task was not identified")
	}
	modelPayload, _ := json.Marshal(map[string]any{"model": "gpt-image-1"})
	if consoleImageTaskUsesApplication(modelPayload) {
		t.Fatal("model task was identified as application")
	}
}
