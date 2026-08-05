package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

const (
	requestStringPreviewBytes = 160
	requestFilePreviewBytes   = 24
)

// ExtractRequestPayloadWithContentType extends ExtractRequestPayload to image
// multipart requests and bounds inline image values before audit persistence.
func ExtractRequestPayloadWithContentType(body []byte, protocol domain.UpstreamProtocol, contentType string) (messages, params json.RawMessage) {
	if isMultipartFormData(contentType) {
		fields := summarizeMultipartRequest(body, contentType)
		delete(fields, "model")
		delete(fields, "stream")
		return nil, marshalSummary(fields)
	}
	messages, params = ExtractRequestPayload(body, protocol)
	if protocol != domain.ProtocolOpenAIImages || len(params) == 0 {
		return messages, params
	}
	var value any
	if json.Unmarshal(params, &value) != nil {
		return messages, params
	}
	return messages, marshalSummary(summarizeRequestValue(value))
}

// SummarizeRequestBody keeps the outbound wire shape visible in logs while
// bounding inline Base64/data URLs and replacing multipart files with metadata.
func SummarizeRequestBody(body []byte, contentType string) string {
	if len(body) == 0 {
		return ""
	}
	if isMultipartFormData(contentType) {
		return string(marshalSummary(summarizeMultipartRequest(body, contentType)))
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		return string(marshalSummary(summarizeRequestValue(value)))
	}
	return summarizeRequestString(string(body))
}

func isMultipartFormData(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "multipart/form-data"
}

func summarizeMultipartRequest(body []byte, contentType string) map[string]any {
	fields := map[string]any{}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return fields
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fields["_parse_error"] = err.Error()
			break
		}
		payload, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			appendSummaryField(fields, part.FormName(), map[string]any{"read_error": readErr.Error()}, true)
			continue
		}
		if part.FileName() == "" {
			appendSummaryField(fields, part.FormName(), summarizeRequestString(string(payload)), false)
			continue
		}
		contentType := strings.TrimSpace(part.Header.Get("Content-Type"))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		previewLen := len(payload)
		if previewLen > requestFilePreviewBytes {
			previewLen = requestFilePreviewBytes
		}
		preview := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(payload[:previewLen])
		if previewLen < len(payload) {
			preview += fmt.Sprintf("...<truncated %d bytes>", len(payload)-previewLen)
		}
		digest := sha256.Sum256(payload)
		appendSummaryField(fields, part.FormName(), map[string]any{
			"filename":      part.FileName(),
			"content_type":  contentType,
			"size_bytes":    len(payload),
			"sha256":        hex.EncodeToString(digest[:]),
			"value_preview": preview,
		}, true)
	}
	return fields
}

func appendSummaryField(fields map[string]any, name string, value any, forceArray bool) {
	if name == "" {
		name = "_unnamed"
	}
	existing, ok := fields[name]
	if !ok {
		if forceArray {
			fields[name] = []any{value}
		} else {
			fields[name] = value
		}
		return
	}
	if values, ok := existing.([]any); ok {
		fields[name] = append(values, value)
		return
	}
	fields[name] = []any{existing, value}
}

func summarizeRequestValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = summarizeRequestValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = summarizeRequestValue(item)
		}
		return out
	case string:
		return summarizeRequestString(typed)
	default:
		return value
	}
}

func summarizeRequestString(value string) string {
	if parsed, err := url.Parse(value); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && (parsed.RawQuery != "" || parsed.Fragment != "") {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		value = parsed.String() + "?<redacted>"
	}
	if len(value) <= requestStringPreviewBytes {
		return value
	}
	return value[:requestStringPreviewBytes] + fmt.Sprintf("...<truncated %d bytes>", len(value)-requestStringPreviewBytes)
}

func marshalSummary(value any) json.RawMessage {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil
	}
	return bytes.TrimSpace(out.Bytes())
}
