package console

import (
	"bytes"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageedit"
)

func TestDecodeConsoleImageSubmissionRejectsUnknownJSONFields(t *testing.T) {
	_, err := decodeConsoleImageSubmission(
		[]byte(`{"model":"gpt-image-1","prompt":"draw","unexpected":true}`),
		"application/json",
		"generation",
	)
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if validation.Field != "request" {
		t.Fatalf("validation field = %q, want request", validation.Field)
	}
}

func TestDecodeConsoleImageMultipartSubmissionPersistsSelfContainedEdit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":     "gpt-image-1",
		"group_id":  "group-1",
		"prompt":    "retouch",
		"variables": `{"tone":"warm"}`,
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	image, err := writer.CreateFormFile("image[]", "source.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode fixture image: %v", err)
	}
	if _, err := image.Write(imageBytes); err != nil {
		t.Fatalf("write image part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}

	input, err := decodeConsoleImageSubmission(
		body.Bytes(),
		writer.FormDataContentType(),
		"edit",
	)
	if err != nil {
		t.Fatalf("decode multipart submission: %v", err)
	}
	if input.Operation != "edit" || input.Model != "gpt-image-1" || input.GroupID != "group-1" {
		t.Fatalf("persisted routing input = %+v", input)
	}
	if input.Variables["tone"] != "warm" {
		t.Fatalf("persisted variables = %#v", input.Variables)
	}
	request, err := imageedit.Decode(input.EditRequest, imageedit.TransportJSON)
	if err != nil {
		t.Fatalf("decode persisted edit request: %v", err)
	}
	if len(request.Images) != 1 || !bytes.Equal(request.Images[0].Data, imageBytes) {
		t.Fatalf("persisted images = %#v", request.Images)
	}
	if request.Model != "" || request.Prompt != "" {
		t.Fatalf("persisted edit request leaked resolved fields: model=%q prompt=%q", request.Model, request.Prompt)
	}
}
