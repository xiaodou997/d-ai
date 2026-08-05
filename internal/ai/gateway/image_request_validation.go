package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/imagepayload"
	"xiaodou/dai/internal/ai/serving"
)

const (
	openAIImagePartialImagesUnsupportedMessage = "partial_images is not supported; only the final completed image is returned."
	maxImageRequestBodyBytes                   = imagepayload.MaxImageRequestBodyBytes
	maxImageRawInputBytes                      = imagepayload.MaxImageRawInputBytes
)

// normalizeOpenAIImageRequest applies the platform-wide client contract before
// routing. quality and style are not client inputs, n is bounded to the
// platform maximum, and partial image events are unsupported.
func normalizeOpenAIImageRequest(body []byte, contentType string) ([]byte, string, error) {
	if len(body) == 0 {
		return body, contentType, nil
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType == "multipart/form-data" {
		return rewriteOpenAIImageMultipart(body, contentType)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || !strings.HasPrefix(string(trimmed), "{") {
		return body, contentType, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil || fields == nil {
		return body, contentType, nil
	}
	if _, ok := fields["partial_images"]; ok {
		return nil, "", unsupportedOpenAIImageOptionError(openAIImagePartialImagesUnsupportedMessage)
	}
	if rawCount, ok := fields["n"]; ok {
		var count int
		if err := json.Unmarshal(rawCount, &count); err != nil || !validOpenAIImageCount(count) {
			return nil, "", invalidOpenAIImageCountError()
		}
	}
	delete(fields, "quality")
	delete(fields, "style")
	normalized, err := json.Marshal(fields)
	if err != nil {
		return nil, "", err
	}
	return normalized, contentType, nil
}

func normalizeOpenAIImageRuntimeRequest(body []byte, contentType, path string) ([]byte, string, error) {
	if !strings.Contains(path, "/images/edits") {
		return normalizeOpenAIImageRequest(body, contentType)
	}
	req, err := decodeRunImageEditRequestBody(body, contentType)
	if err != nil {
		return nil, "", err
	}
	canonical, err := imageedit.CanonicalJSON(req)
	if err != nil {
		return nil, "", err
	}
	return canonical, imageedit.TransportJSON, nil
}

func validateOpenAIImageInputLimits(body []byte, contentType string) error {
	return validateOpenAIImageInputSize(body, contentType, maxImageRawInputBytes)
}

func validateOpenAIImageInputSize(body []byte, contentType string, maxRawBytes int64) error {
	if len(body) == 0 {
		return nil
	}
	if mediaType, params, err := mime.ParseMediaType(contentType); err == nil && mediaType == "multipart/form-data" {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return nil
			}
			name := strings.ToLower(strings.TrimSpace(part.FormName()))
			if part.FileName() != "" && isOpenAIImageInputField(name) {
				size, readErr := io.Copy(io.Discard, io.LimitReader(part, maxRawBytes+1))
				_ = part.Close()
				if readErr != nil {
					return readErr
				}
				if size > maxRawBytes {
					return imageInputTooLargeError(maxRawBytes)
				}
				continue
			}
			_ = part.Close()
		}
	}

	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	return validateOpenAIImageJSONValue(doc, maxRawBytes)
}

func validateOpenAIImageJSONValue(value any, maxRawBytes int64) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isOpenAIImageInputField(strings.ToLower(strings.TrimSpace(key))) {
				if err := validateOpenAIBase64Value(item, maxRawBytes); err != nil {
					return err
				}
				continue
			}
			if err := validateOpenAIImageJSONValue(item, maxRawBytes); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := validateOpenAIImageJSONValue(item, maxRawBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func isOpenAIImageInputField(name string) bool {
	switch name {
	case "image[]", "images", "mask":
		return true
	default:
		return false
	}
}

func validateOpenAIBase64Value(value any, maxRawBytes int64) error {
	switch typed := value.(type) {
	case string:
		return validateOpenAIBase64String(typed, maxRawBytes)
	case []any:
		for _, item := range typed {
			if err := validateOpenAIBase64Value(item, maxRawBytes); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range typed {
			if err := validateOpenAIBase64Value(item, maxRawBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOpenAIBase64String(raw string, maxRawBytes int64) error {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return nil
	}
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma <= len("data:") || !strings.Contains(strings.ToLower(value[:comma]), ";base64") {
			return nil
		}
		value = value[comma+1:]
	}
	decoded, err := io.Copy(io.Discard, io.LimitReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(value)), maxRawBytes+1))
	if err != nil {
		return nil // Non-base64 image references remain the upstream's validation concern.
	}
	if decoded > maxRawBytes {
		return imageInputTooLargeError(maxRawBytes)
	}
	return nil
}

func imageInputTooLargeError(maxRawBytes int64) error {
	return &serving.APIError{
		Status:  http.StatusRequestEntityTooLarge,
		Code:    "image_too_large",
		Message: "Image input exceeds the maximum raw size of " + strconv.FormatInt(maxRawBytes>>20, 10) + " MiB.",
	}
}

func sanitizeGeminiImageCount(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	if isImage, _ := formats.GeminiRequestImageIntent(body); !isImage {
		return body, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil || doc == nil {
		return body, nil
	}
	for _, key := range []string{"generationConfig", "generation_config"} {
		value, ok := doc[key]
		config, ok := value.(map[string]any)
		if !ok {
			continue
		}
		for _, countKey := range []string{"candidateCount", "candidate_count"} {
			if value, exists := config[countKey]; exists {
				count, valid := imageCountFromAny(value)
				if !valid || !validOpenAIImageCount(count) {
					return nil, invalidOpenAIImageCountError()
				}
			}
		}
	}
	return body, nil
}

func rewriteOpenAIImageMultipart(body []byte, contentType string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return body, contentType, nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return body, contentType, nil
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = writer.Close()
			return body, contentType, nil
		}
		name := part.FormName()
		if name == "partial_images" && part.FileName() == "" {
			_ = part.Close()
			_ = writer.Close()
			return nil, "", unsupportedOpenAIImageOptionError(openAIImagePartialImagesUnsupportedMessage)
		}
		if name == "n" && part.FileName() == "" {
			value, readErr := io.ReadAll(io.LimitReader(part, 64))
			_ = part.Close()
			count, parseErr := strconv.Atoi(strings.TrimSpace(string(value)))
			if readErr != nil || parseErr != nil || !validOpenAIImageCount(count) {
				_ = writer.Close()
				return nil, "", invalidOpenAIImageCountError()
			}
			if err := writer.WriteField("n", strconv.Itoa(count)); err != nil {
				_ = writer.Close()
				return nil, "", err
			}
			continue
		}
		if (name == "quality" || name == "style") && part.FileName() == "" {
			_ = part.Close()
			continue
		}
		header := cloneRunMIMEHeader(part.Header)
		dst, err := writer.CreatePart(header)
		if err != nil {
			_ = part.Close()
			_ = writer.Close()
			return nil, "", err
		}
		if _, err := io.Copy(dst, part); err != nil {
			_ = part.Close()
			_ = writer.Close()
			return nil, "", err
		}
		_ = part.Close()
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), writer.FormDataContentType(), nil
}

func invalidOpenAIImageCountError() error {
	return &serving.APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_request_error",
		Message: "n: must be an integer between 1 and 10",
	}
}

func validOpenAIImageCount(count int) bool {
	return count >= domain.DefaultImageOutputCount && count <= domain.MaxImageOutputCount
}

func imageCountFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		count := int(typed)
		return count, typed == float64(count)
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func decodeRunImageGenerationRequestBody(body []byte) (runImageGenerationRequest, error) {
	if err := rejectAppRunPromptJSON(body); err != nil {
		return runImageGenerationRequest{}, err
	}
	body, _, err := normalizeOpenAIImageRequest(body, "application/json")
	if err != nil {
		return runImageGenerationRequest{}, err
	}
	var req runImageGenerationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return runImageGenerationRequest{}, err
	}
	req.Input = strings.TrimSpace(req.Input)
	return req, nil
}

func decodeAppRunImageEditRequestBody(body []byte, contentType string) (imageedit.Request, map[string]string, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return imageedit.Request{}, nil, &serving.APIError{
			Status: http.StatusBadRequest, Code: "invalid_request_error", Message: "a valid Content-Type is required",
		}
	}

	var input string
	var variables map[string]string
	switch mediaType {
	case imageedit.TransportJSON:
		if err := rejectAppRunPromptJSON(body); err != nil {
			return imageedit.Request{}, nil, err
		}
		var metadata struct {
			Input     string            `json:"input"`
			Variables map[string]string `json:"variables"`
		}
		if err := json.Unmarshal(body, &metadata); err != nil {
			return imageedit.Request{}, nil, err
		}
		input = strings.TrimSpace(metadata.Input)
		variables = metadata.Variables
	case imageedit.TransportMultipart:
		fields, err := formats.MultipartScalarFields(body, contentType, 1<<20)
		if err != nil {
			return imageedit.Request{}, nil, err
		}
		if _, exists := fields["prompt"]; exists {
			return imageedit.Request{}, nil, appRunPromptFieldError()
		}
		input = strings.TrimSpace(fields["input"])
		variables = decodeStringMap(fields["variables"])
	default:
		return imageedit.Request{}, nil, &serving.APIError{
			Status: http.StatusBadRequest, Code: "invalid_request_error", Message: "application/json or multipart/form-data is required",
		}
	}
	if input == "" {
		return imageedit.Request{}, nil, errRunInputRequired
	}
	req, err := decodeRunImageEditRequestBody(body, contentType)
	if err != nil {
		return imageedit.Request{}, nil, err
	}
	req.Prompt = input
	return req, variables, nil
}

func rejectAppRunPromptJSON(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return nil
	}
	if _, exists := fields["prompt"]; exists {
		return appRunPromptFieldError()
	}
	return nil
}

func appRunPromptFieldError() error {
	return &serving.APIError{
		Status: http.StatusBadRequest, Code: "invalid_request_error",
		Message: "prompt is not supported for app runs; use input",
	}
}

func decodeRunImageEditRequestBody(body []byte, contentType string) (imageedit.Request, error) {
	if err := validateOpenAIImageInputLimits(body, contentType); err != nil {
		return imageedit.Request{}, err
	}
	req, err := imageedit.Decode(body, contentType)
	if err != nil {
		return imageedit.Request{}, &serving.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request_error",
			Message: err.Error(),
		}
	}
	return req, nil
}

func unsupportedOpenAIImageOptionError(message string) error {
	return &serving.APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_request_error",
		Message: message,
	}
}
