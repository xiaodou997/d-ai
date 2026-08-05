package imageedit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"testing"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/lAAAAABJRU5ErkJggg=="

func TestJSONToJSONUsesOfficialImagesSchema(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-image-2",
		"images":[{"image_url":"data:image/png;base64,` + onePixelPNG + `"}],
		"mask":{"image_url":"https://example.com/mask.png"},
		"prompt":"edit",
		"output_compression":90,
		"response_format":"url"
	}`)
	req, err := Decode(raw, TransportJSON)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	encoded, err := Encode(context.Background(), req, TransportJSON)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	images := doc["images"].([]any)
	image := images[0].(map[string]any)
	if image["image_url"] == "" {
		t.Fatalf("image_url missing: %#v", image)
	}
	if _, ok := doc["response_format"]; ok {
		t.Fatalf("client response_format leaked upstream: %#v", doc)
	}
}

func TestCanonicalJSONRetainsClientResponsePreference(t *testing.T) {
	req, err := Decode([]byte(`{"images":[{"image_url":"https://example.com/a.png"}],"response_format":"url"}`), TransportJSON)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	body, err := CanonicalJSON(req)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["response_format"] != "url" {
		t.Fatalf("response_format = %#v", doc["response_format"])
	}
}

func TestMultipartToJSONUsesDataURL(t *testing.T) {
	body, contentType := multipartBody(t)
	req, err := Decode(body, contentType)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	encoded, err := Encode(context.Background(), req, TransportJSON)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var doc struct {
		Images []struct {
			ImageURL string `json:"image_url"`
		} `json:"images"`
	}
	if err := json.Unmarshal(encoded.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Images) != 1 || doc.Images[0].ImageURL != "data:image/png;base64,"+onePixelPNG {
		t.Fatalf("images = %#v", doc.Images)
	}
}

func TestJSONToMultipartWritesImageArrayFiles(t *testing.T) {
	raw := []byte(`{"images":[{"image_url":"data:image/png;base64,` + onePixelPNG + `"}],"prompt":"edit"}`)
	req, err := Decode(raw, TransportJSON)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	encoded, err := Encode(context.Background(), req, TransportMultipart)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(encoded.ContentType)
	if err != nil || mediaType != TransportMultipart {
		t.Fatalf("content type = %q, err=%v", encoded.ContentType, err)
	}
	reader := multipart.NewReader(bytes.NewReader(encoded.Body), params["boundary"])
	foundImage := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		if part.FormName() == "image[]" && part.FileName() != "" {
			foundImage = true
		}
		_ = part.Close()
	}
	if !foundImage {
		t.Fatal("image[] file part missing")
	}
}

func TestMultipartToMultipartKeepsOfficialShape(t *testing.T) {
	body, contentType := multipartBody(t)
	req, err := Decode(body, contentType)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	encoded, err := Encode(context.Background(), req, TransportMultipart)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded.Body, encoded.ContentType)
	if err != nil {
		t.Fatalf("Decode encoded: %v", err)
	}
	if len(decoded.Images) != 1 || string(decoded.Images[0].Data) != string(mustPNG(t)) {
		t.Fatalf("images = %#v", decoded.Images)
	}
	if bytes.Contains(encoded.Body, []byte(`name="quality"`)) || bytes.Contains(encoded.Body, []byte(`name="style"`)) {
		t.Fatalf("unmapped client options leaked upstream")
	}
}

func TestJSONIgnoresUnmappedFields(t *testing.T) {
	raw := []byte(`{
		"images":[{"image_url":"https://example.com/a.png","extra":"ignored"}],
		"unmapped":"ignored",
		"quality":"high",
		"style":"vivid"
	}`)
	req, err := Decode(raw, TransportJSON)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	encoded, err := Encode(context.Background(), req, TransportJSON)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"unmapped", "quality", "style"} {
		if _, ok := doc[field]; ok {
			t.Fatalf("unmapped field %q leaked upstream: %#v", field, doc)
		}
	}
	images := doc["images"].([]any)
	image := images[0].(map[string]any)
	if len(image) != 1 || image["image_url"] != "https://example.com/a.png" {
		t.Fatalf("image source = %#v", image)
	}
}

func TestJSONRequiresCurrentImageURLForUnmappedSource(t *testing.T) {
	_, err := Decode([]byte(`{"images":[{"unmapped":"ignored"}]}`), TransportJSON)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if validationErr.Field != "images[0].image_url" {
		t.Fatalf("field = %q, want images[0].image_url", validationErr.Field)
	}
}

func TestRejectsInvalidCurrentImageInputs(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "raw base64 image url", body: `{"images":[{"image_url":"` + onePixelPNG + `"}]}`},
		{name: "too many outputs", body: `{"images":[{"image_url":"https://example.com/a.png"}],"n":11}`},
		{name: "partial images", body: `{"images":[{"image_url":"https://example.com/a.png"}],"partial_images":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode([]byte(tc.body), TransportJSON); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestJSONAcceptsMultipleOutputs(t *testing.T) {
	req, err := Decode([]byte(`{"images":[{"image_url":"https://example.com/a.png"}],"n":2}`), TransportJSON)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if req.N != 2 {
		t.Fatalf("n = %d, want 2", req.N)
	}
	body, err := CanonicalJSON(req)
	if err != nil || !bytes.Contains(body, []byte(`"n":2`)) {
		t.Fatalf("CanonicalJSON = %s, err = %v", body, err)
	}
}

func TestJSONRejectsNonStringScalarFields(t *testing.T) {
	fields := []string{
		"model",
		"prompt",
		"size",
		"background",
		"input_fidelity",
		"moderation",
		"output_format",
		"user",
		"response_format",
	}
	invalidValues := []string{`123`, `{}`, `[]`}

	for index, field := range fields {
		t.Run(field, func(t *testing.T) {
			body := []byte(`{"images":[{"image_url":"https://example.com/a.png"}],"` + field + `":` + invalidValues[index%len(invalidValues)] + `}`)
			_, err := Decode(body, TransportJSON)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want ValidationError", err)
			}
			if validationErr.Field != field {
				t.Fatalf("field = %q, want %q", validationErr.Field, field)
			}
			if validationErr.Message != "must be a string" {
				t.Fatalf("message = %q, want must be a string", validationErr.Message)
			}
		})
	}
}

func TestMultipartRejectsUnsupportedFileField(t *testing.T) {
	body, contentType := multipartBodyWithImageField(t, "attachment")
	_, err := Decode(body, contentType)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if validationErr.Message != "unsupported multipart file field" {
		t.Fatalf("message = %q", validationErr.Message)
	}
}

func multipartBody(t *testing.T) ([]byte, string) {
	return multipartBodyWithImageField(t, "image[]")
}

func multipartBodyWithImageField(t *testing.T, field string) ([]byte, string) {
	t.Helper()
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{`form-data; name="` + field + `"; filename="input.png"`}
	header["Content-Type"] = []string{"image/png"}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(mustPNG(t)); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := writer.WriteField("prompt", "edit"); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := writer.WriteField("quality", "high"); err != nil {
		t.Fatalf("write quality: %v", err)
	}
	if err := writer.WriteField("style", "vivid"); err != nil {
		t.Fatalf("write style: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return out.Bytes(), writer.FormDataContentType()
}

func mustPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return data
}
