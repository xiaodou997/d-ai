package imageedit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/imagepayload"
)

const (
	TransportJSON      = "application/json"
	TransportMultipart = "multipart/form-data"

	MaxImages = 16
)

var (
	ErrUnsupportedPartialImages = errors.New("partial_images is not supported; only the final completed image is returned")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil || e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

type Source struct {
	URL      string
	Data     []byte
	MIMEType string
	Filename string
}

type Request struct {
	Model             string
	N                 int
	Images            []Source
	Mask              *Source
	Prompt            string
	Size              string
	Background        string
	InputFidelity     string
	Moderation        string
	OutputFormat      string
	OutputCompression *int
	Stream            bool
	User              string
	ResponseFormat    string
}

type Encoded struct {
	Body        []byte
	ContentType string
}

func Decode(body []byte, contentType string) (Request, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return Request{}, &ValidationError{Field: "content_type", Message: "content type is required"}
	}
	switch mediaType {
	case TransportJSON:
		return decodeJSON(body)
	case TransportMultipart:
		return decodeMultipart(body, contentType)
	default:
		return Request{}, &ValidationError{Field: "content_type", Message: "must be application/json or multipart/form-data"}
	}
}

func Encode(ctx context.Context, req Request, transport string) (Encoded, error) {
	req.ResponseFormat = ""
	return encode(ctx, req, transport)
}

// EncodeForUpstream includes a response format owned by trusted model-binding
// configuration. Regular Encode deliberately strips the client preference.
func EncodeForUpstream(ctx context.Context, req Request, transport, responseFormat string) (Encoded, error) {
	switch strings.TrimSpace(responseFormat) {
	case "", domain.ImageResponseFormatURL, domain.ImageResponseFormatB64:
		req.ResponseFormat = strings.TrimSpace(responseFormat)
	default:
		return Encoded{}, &ValidationError{Field: "response_format", Message: "must be url, b64_json or empty"}
	}
	return encode(ctx, req, transport)
}

func encode(ctx context.Context, req Request, transport string) (Encoded, error) {
	if err := validateRequest(req); err != nil {
		return Encoded{}, err
	}
	switch strings.TrimSpace(transport) {
	case TransportJSON:
		body, err := encodeJSON(req)
		return Encoded{Body: body, ContentType: TransportJSON}, err
	case TransportMultipart:
		return encodeMultipart(ctx, req)
	default:
		return Encoded{}, &ValidationError{Field: "image_edit_transport", Message: "must be application/json or multipart/form-data"}
	}
}

// CanonicalJSON serializes the normalized request for internal handoff. The
// platform response preference is retained here and stripped only by Encode
// when the final upstream request is built.
func CanonicalJSON(req Request) ([]byte, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	return encodeJSON(req)
}

func decodeJSON(body []byte) (Request, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil || doc == nil {
		return Request{}, &ValidationError{Field: "request", Message: "expected a JSON object"}
	}
	if _, ok := doc["partial_images"]; ok {
		return Request{}, &ValidationError{Field: "partial_images", Message: ErrUnsupportedPartialImages.Error()}
	}
	var req Request
	if raw, ok := doc["n"]; ok {
		if err := json.Unmarshal(raw, &req.N); err != nil || req.N < 1 || req.N > domain.MaxImageOutputCount {
			return Request{}, &ValidationError{Field: "n", Message: "must be an integer between 1 and 10"}
		}
	}

	for _, field := range []struct {
		name   string
		target *string
	}{
		{name: "model", target: &req.Model},
		{name: "prompt", target: &req.Prompt},
		{name: "size", target: &req.Size},
		{name: "background", target: &req.Background},
		{name: "input_fidelity", target: &req.InputFidelity},
		{name: "moderation", target: &req.Moderation},
		{name: "output_format", target: &req.OutputFormat},
		{name: "user", target: &req.User},
		{name: "response_format", target: &req.ResponseFormat},
	} {
		if err := decodeJSONString(doc, field.name, field.target); err != nil {
			return Request{}, err
		}
	}
	if raw := doc["stream"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &req.Stream); err != nil {
			return Request{}, &ValidationError{Field: "stream", Message: "must be a boolean"}
		}
	}
	if raw := doc["output_compression"]; len(raw) > 0 {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return Request{}, &ValidationError{Field: "output_compression", Message: "must be an integer from 0 to 100"}
		}
		req.OutputCompression = &value
	}

	rawImages, ok := doc["images"]
	if !ok {
		return Request{}, &ValidationError{Field: "images", Message: "is required"}
	}
	var images []map[string]json.RawMessage
	if err := json.Unmarshal(rawImages, &images); err != nil {
		return Request{}, &ValidationError{Field: "images", Message: "must be an array of objects"}
	}
	for index, item := range images {
		source, err := decodeJSONSource(item, fmt.Sprintf("images[%d]", index))
		if err != nil {
			return Request{}, err
		}
		req.Images = append(req.Images, source)
	}
	if rawMask := doc["mask"]; len(rawMask) > 0 && string(bytes.TrimSpace(rawMask)) != "null" {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawMask, &item); err != nil {
			return Request{}, &ValidationError{Field: "mask", Message: "must be an object"}
		}
		source, err := decodeJSONSource(item, "mask")
		if err != nil {
			return Request{}, err
		}
		req.Mask = &source
	}
	if err := validateRequest(req); err != nil {
		return Request{}, err
	}
	return req, nil
}

func decodeJSONSource(item map[string]json.RawMessage, field string) (Source, error) {
	if item == nil {
		return Source{}, &ValidationError{Field: field, Message: "must be an object containing image_url"}
	}
	var imageURL string
	if raw := item["image_url"]; len(raw) == 0 || json.Unmarshal(raw, &imageURL) != nil {
		return Source{}, &ValidationError{Field: field + ".image_url", Message: "must be a string"}
	}
	return sourceFromImageURL(imageURL, field+".image_url")
}

func sourceFromImageURL(value, field string) (Source, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Source{}, &ValidationError{Field: field, Message: "is required"}
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		data, contentType, handled, err := imagepayload.DecodeInlineImageValue(value)
		if err != nil {
			return Source{}, &ValidationError{Field: field, Message: err.Error()}
		}
		if !handled {
			return Source{}, &ValidationError{Field: field, Message: "must be a valid base64 data URL"}
		}
		return Source{Data: data, MIMEType: contentType}, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Source{}, &ValidationError{Field: field, Message: "must be a fully qualified HTTP(S) URL or base64 data URL"}
	}
	return Source{URL: value}, nil
}

func decodeMultipart(body []byte, contentType string) (Request, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return Request{}, &ValidationError{Field: "content_type", Message: "multipart boundary is required"}
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var req Request
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Request{}, &ValidationError{Field: "request", Message: "invalid multipart body"}
		}
		name := strings.TrimSpace(part.FormName())
		if part.FileName() != "" {
			source, readErr := readMultipartImage(part)
			_ = part.Close()
			if readErr != nil {
				return Request{}, readErr
			}
			switch name {
			case "image[]":
				req.Images = append(req.Images, source)
			case "mask":
				if req.Mask != nil {
					return Request{}, &ValidationError{Field: "mask", Message: "must appear at most once"}
				}
				req.Mask = &source
			default:
				return Request{}, &ValidationError{Field: name, Message: "unsupported multipart file field"}
			}
			continue
		}
		value, readErr := io.ReadAll(io.LimitReader(part, 1<<20))
		_ = part.Close()
		if readErr != nil {
			return Request{}, &ValidationError{Field: name, Message: "cannot read multipart field"}
		}
		if name == "image[]" || name == "mask" {
			return Request{}, &ValidationError{Field: name, Message: "must be a file upload"}
		}
		if err := setScalar(&req, name, strings.TrimSpace(string(value))); err != nil {
			return Request{}, err
		}
	}
	if err := validateRequest(req); err != nil {
		return Request{}, err
	}
	return req, nil
}

func readMultipartImage(part *multipart.Part) (Source, error) {
	data, err := io.ReadAll(part)
	if err != nil {
		return Source{}, &ValidationError{Field: part.FormName(), Message: "cannot read uploaded image"}
	}
	contentType := strings.TrimSpace(part.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return Source{}, &ValidationError{Field: part.FormName(), Message: "uploaded file must be an image"}
	}
	return Source{Data: data, MIMEType: contentType, Filename: part.FileName()}, nil
}

func setScalar(req *Request, name, value string) error {
	switch name {
	case "model":
		req.Model = value
	case "prompt":
		req.Prompt = value
	case "size":
		req.Size = value
	case "background":
		req.Background = value
	case "input_fidelity":
		req.InputFidelity = value
	case "moderation":
		req.Moderation = value
	case "output_format":
		req.OutputFormat = value
	case "output_compression":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return &ValidationError{Field: name, Message: "must be an integer from 0 to 100"}
		}
		req.OutputCompression = &parsed
	case "stream":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return &ValidationError{Field: name, Message: "must be true or false"}
		}
		req.Stream = parsed
	case "user":
		req.User = value
	case "response_format":
		req.ResponseFormat = value
	case "n":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > domain.MaxImageOutputCount {
			return &ValidationError{Field: name, Message: "must be an integer between 1 and 10"}
		}
		req.N = parsed
	case "partial_images":
		return &ValidationError{Field: name, Message: ErrUnsupportedPartialImages.Error()}
	}
	return nil
}

func validateRequest(req Request) error {
	if req.N < 0 || req.N > domain.MaxImageOutputCount {
		return &ValidationError{Field: "n", Message: "must be an integer between 1 and 10"}
	}
	if len(req.Images) == 0 {
		return &ValidationError{Field: "images", Message: "at least one image is required"}
	}
	if len(req.Images) > MaxImages {
		return &ValidationError{Field: "images", Message: "at most 16 images are supported"}
	}
	for index, source := range req.Images {
		if err := validateSource(source, fmt.Sprintf("images[%d]", index)); err != nil {
			return err
		}
	}
	if req.Mask != nil {
		if err := validateSource(*req.Mask, "mask"); err != nil {
			return err
		}
	}
	if req.OutputCompression != nil && (*req.OutputCompression < 0 || *req.OutputCompression > 100) {
		return &ValidationError{Field: "output_compression", Message: "must be between 0 and 100"}
	}
	if req.OutputFormat != "" && req.OutputFormat != "png" && req.OutputFormat != "jpeg" && req.OutputFormat != "webp" {
		return &ValidationError{Field: "output_format", Message: "must be png, jpeg or webp"}
	}
	if req.Background != "" && req.Background != "auto" && req.Background != "opaque" && req.Background != "transparent" {
		return &ValidationError{Field: "background", Message: "must be auto, opaque or transparent"}
	}
	if req.InputFidelity != "" && req.InputFidelity != "low" && req.InputFidelity != "high" {
		return &ValidationError{Field: "input_fidelity", Message: "must be low or high"}
	}
	if req.Moderation != "" && req.Moderation != "auto" && req.Moderation != "low" {
		return &ValidationError{Field: "moderation", Message: "must be auto or low"}
	}
	if req.ResponseFormat != "" && req.ResponseFormat != "b64_json" && req.ResponseFormat != "url" {
		return &ValidationError{Field: "response_format", Message: "must be b64_json or url"}
	}
	return nil
}

func validateSource(source Source, field string) error {
	hasURL := strings.TrimSpace(source.URL) != ""
	hasData := len(source.Data) > 0
	if hasURL == hasData {
		return &ValidationError{Field: field, Message: "must contain exactly one URL or image payload"}
	}
	if hasData && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(source.MIMEType)), "image/") {
		return &ValidationError{Field: field, Message: "image MIME type is required"}
	}
	return nil
}

func encodeJSON(req Request) ([]byte, error) {
	images := make([]map[string]string, 0, len(req.Images))
	for _, source := range req.Images {
		images = append(images, map[string]string{"image_url": sourceJSONURL(source)})
	}
	doc := map[string]any{"images": images}
	setJSONFields(doc, req)
	if req.Mask != nil {
		doc["mask"] = map[string]string{"image_url": sourceJSONURL(*req.Mask)}
	}
	return json.Marshal(doc)
}

func sourceJSONURL(source Source) string {
	if strings.TrimSpace(source.URL) != "" {
		return strings.TrimSpace(source.URL)
	}
	return "data:" + source.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(source.Data)
}

func setJSONFields(doc map[string]any, req Request) {
	for key, value := range requestScalarFields(req) {
		if value != "" {
			doc[key] = value
		}
	}
	if req.OutputCompression != nil {
		doc["output_compression"] = *req.OutputCompression
	}
	if req.N > domain.DefaultImageOutputCount {
		doc["n"] = req.N
	}
	if req.Stream {
		doc["stream"] = true
	}
}

func encodeMultipart(ctx context.Context, req Request) (Encoded, error) {
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	for key, value := range requestScalarFields(req) {
		if value != "" {
			if err := writer.WriteField(key, value); err != nil {
				_ = writer.Close()
				return Encoded{}, err
			}
		}
	}
	if req.OutputCompression != nil {
		if err := writer.WriteField("output_compression", strconv.Itoa(*req.OutputCompression)); err != nil {
			_ = writer.Close()
			return Encoded{}, err
		}
	}
	if req.N > domain.DefaultImageOutputCount {
		if err := writer.WriteField("n", strconv.Itoa(req.N)); err != nil {
			_ = writer.Close()
			return Encoded{}, err
		}
	}
	if req.Stream {
		if err := writer.WriteField("stream", "true"); err != nil {
			_ = writer.Close()
			return Encoded{}, err
		}
	}
	for index, source := range req.Images {
		materialized, err := materializeSource(ctx, source)
		if err != nil {
			_ = writer.Close()
			return Encoded{}, fmt.Errorf("materialize images[%d]: %w", index, err)
		}
		if err := writeImagePart(writer, "image[]", index, materialized); err != nil {
			_ = writer.Close()
			return Encoded{}, err
		}
	}
	if req.Mask != nil {
		materialized, err := materializeSource(ctx, *req.Mask)
		if err != nil {
			_ = writer.Close()
			return Encoded{}, fmt.Errorf("materialize mask: %w", err)
		}
		if err := writeImagePart(writer, "mask", 0, materialized); err != nil {
			_ = writer.Close()
			return Encoded{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return Encoded{}, err
	}
	return Encoded{Body: out.Bytes(), ContentType: writer.FormDataContentType()}, nil
}

func requestScalarFields(req Request) map[string]string {
	return map[string]string{
		"model":           strings.TrimSpace(req.Model),
		"prompt":          strings.TrimSpace(req.Prompt),
		"size":            strings.TrimSpace(req.Size),
		"background":      strings.TrimSpace(req.Background),
		"input_fidelity":  strings.TrimSpace(req.InputFidelity),
		"moderation":      strings.TrimSpace(req.Moderation),
		"output_format":   strings.TrimSpace(req.OutputFormat),
		"user":            strings.TrimSpace(req.User),
		"response_format": strings.TrimSpace(req.ResponseFormat),
	}
}

func materializeSource(ctx context.Context, source Source) (Source, error) {
	if len(source.Data) > 0 {
		return source, nil
	}
	data, contentType, err := imageassets.DownloadExternalImage(ctx, source.URL)
	if err != nil {
		return Source{}, err
	}
	return Source{Data: data, MIMEType: contentType}, nil
}

func writeImagePart(writer *multipart.Writer, field string, index int, source Source) error {
	filename := strings.TrimSpace(source.Filename)
	if filename == "" {
		extensions, _ := mime.ExtensionsByType(source.MIMEType)
		extension := ".png"
		if len(extensions) > 0 {
			extension = extensions[0]
		}
		filename = fmt.Sprintf("image_%d%s", index+1, extension)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": filename}))
	header.Set("Content-Type", source.MIMEType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(source.Data)
	return err
}

func decodeJSONString(doc map[string]json.RawMessage, key string, target *string) error {
	raw := bytes.TrimSpace(doc[key])
	if len(raw) == 0 {
		return nil
	}
	if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, target) != nil {
		return &ValidationError{Field: key, Message: "must be a string"}
	}
	*target = strings.TrimSpace(*target)
	return nil
}
