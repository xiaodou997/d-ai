package formats

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/imagepayload"
)

var (
	ErrOpenAIImageStreamNoImage   = errors.New("openai image stream contains no final image")
	ErrOpenAIImageResponseNoImage = errors.New("openai image response contains no images")
	ErrOpenAIImageStreamUpstream  = errors.New("openai image stream returned an upstream error")
)

// OpenAIImageResponse is the canonical final-image representation shared by
// OpenAI Images sync responses and aggregated image SSE streams.
type OpenAIImageResponse struct {
	Created int64             `json:"created"`
	Model   string            `json:"model,omitempty"`
	Data    []OpenAIImageData `json:"data"`
	Usage   json.RawMessage   `json:"usage,omitempty"`
}

// OpenAIImageData permits either inline Base64 or a URL. Providers are not
// required to honor a requested response format, so callers must handle both.
type OpenAIImageData struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// DecodeOpenAIImageResponse parses a final OpenAI Images response and rejects
// bodies without a usable image. It intentionally accepts either b64_json or
// url because upstream providers may return either representation.
func DecodeOpenAIImageResponse(body []byte) (OpenAIImageResponse, error) {
	var response OpenAIImageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return OpenAIImageResponse{}, fmt.Errorf("parse openai image response: %w", err)
	}
	if _, err := response.FinalImage(); err != nil {
		return OpenAIImageResponse{}, err
	}
	return response, nil
}

// FinalImage chooses the first inline result, falling back to the first URL.
// Base64 takes precedence so a mixed provider response keeps image bytes local.
func (r OpenAIImageResponse) FinalImage() (OpenAIImageData, error) {
	var inlineImage OpenAIImageData
	var urlImage OpenAIImageData
	for _, item := range r.Data {
		item.B64JSON = strings.TrimSpace(item.B64JSON)
		item.URL = strings.TrimSpace(item.URL)
		if item.B64JSON != "" {
			return item, nil
		}
		if item.URL == "" {
			continue
		}
		data, _, handled, err := imagepayload.DecodeInlineImageValue(item.URL)
		if handled {
			if err != nil {
				return OpenAIImageData{}, fmt.Errorf("decode image url payload: %w", err)
			}
			item.B64JSON = base64.StdEncoding.EncodeToString(data)
			item.URL = ""
			if inlineImage.B64JSON == "" {
				inlineImage = item
			}
			continue
		}
		if urlImage.URL == "" {
			urlImage = item
		}
	}
	if inlineImage.B64JSON != "" {
		return inlineImage, nil
	}
	if urlImage.URL != "" {
		return urlImage, nil
	}
	return OpenAIImageData{}, ErrOpenAIImageResponseNoImage
}

// EncodeOpenAIImageCompletedSSE emits one completed event per final image.
func EncodeOpenAIImageCompletedSSE(body []byte, eventName string) ([]byte, error) {
	response, err := DecodeOpenAIImageResponse(body)
	if err != nil {
		return nil, err
	}
	eventName = strings.TrimSpace(eventName)
	if eventName == "" {
		eventName = "image_generation.completed"
	}
	var stream bytes.Buffer
	outputIndex := 0
	for _, rawImage := range response.Data {
		image, err := normalizeOpenAIImageData(rawImage)
		if err != nil {
			return nil, err
		}
		if image.B64JSON == "" && image.URL == "" {
			continue
		}
		payload := map[string]any{
			"type":         eventName,
			"output_index": outputIndex,
			"usage":        response.Usage,
		}
		if image.B64JSON != "" {
			payload["b64_json"] = image.B64JSON
		} else {
			payload["url"] = image.URL
		}
		if image.RevisedPrompt != "" {
			payload["revised_prompt"] = image.RevisedPrompt
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		stream.WriteString("event: " + eventName + "\ndata: " + string(encoded) + "\n\n")
		outputIndex++
	}
	if outputIndex == 0 {
		return nil, ErrOpenAIImageResponseNoImage
	}
	return stream.Bytes(), nil
}

func normalizeOpenAIImageData(item OpenAIImageData) (OpenAIImageData, error) {
	item.B64JSON = strings.TrimSpace(item.B64JSON)
	item.URL = strings.TrimSpace(item.URL)
	if item.B64JSON != "" || item.URL == "" {
		return item, nil
	}
	data, _, handled, err := imagepayload.DecodeInlineImageValue(item.URL)
	if !handled {
		return item, nil
	}
	if err != nil {
		return OpenAIImageData{}, fmt.Errorf("decode image url payload: %w", err)
	}
	item.B64JSON = base64.StdEncoding.EncodeToString(data)
	item.URL = ""
	return item, nil
}

// AggregateOpenAIImageSSE collapses a complete OpenAI-compatible image SSE
// response into canonical OpenAI Images JSON containing only final images.
func AggregateOpenAIImageSSE(raw []byte) ([]byte, error) {
	result := OpenAIImageResponse{}
	seen := map[string]bool{}
	for _, block := range splitOpenAIImageSSEBlocks(raw) {
		eventName, payload, ok := parseOpenAIImageSSEBlock(block)
		if !ok {
			continue
		}
		obj, ok := decodeObject(payload)
		if !ok {
			continue
		}
		if eventName == "error" || getStr(obj, "type") == "error" || objErrorFieldIsPresent(obj) {
			return nil, openAIImageStreamUpstreamError(obj)
		}
		switch {
		case getStr(obj, "object") == "image.generation.result":
			result.recordMetadata(obj)
			result.appendDataItems(obj, seen)
		case eventName == "image_generation.completed" || getStr(obj, "type") == "image_generation.completed":
			result.recordMetadata(obj)
			result.appendDataItems(obj, seen)
			result.appendImage(obj, seen)
		case eventName == "response.output_item.done" || getStr(obj, "type") == "response.output_item.done":
			result.recordMetadata(obj)
			if item, ok := decodeObject(field(obj, "item")); ok {
				result.appendResponseImage(item, seen)
				if len(result.Usage) == 0 {
					if usage := field(item, "usage"); usage != nil {
						result.Usage = append(result.Usage[:0], usage...)
					}
				}
			}
		case eventName == "response.completed" || getStr(obj, "type") == "response.completed":
			result.recordMetadata(obj)
			if response, ok := decodeObject(field(obj, "response")); ok {
				result.recordResponseMetadata(response)
				if output, ok := decodeArray(field(response, "output")); ok {
					for _, rawItem := range output {
						if item, ok := decodeObject(rawItem); ok {
							result.appendResponseImage(item, seen)
						}
					}
				}
			}
		}
	}
	if len(result.Data) == 0 {
		return nil, ErrOpenAIImageStreamNoImage
	}
	if result.Created == 0 {
		result.Created = time.Now().Unix()
	}
	return json.Marshal(result)
}

func openAIImageStreamUpstreamError(obj map[string]json.RawMessage) error {
	message := strings.TrimSpace(getStr(obj, "message"))
	if errorObj, ok := decodeObject(field(obj, "error")); ok {
		if nestedMessage := strings.TrimSpace(getStr(errorObj, "message")); nestedMessage != "" {
			message = nestedMessage
		}
	}
	if message == "" {
		return ErrOpenAIImageStreamUpstream
	}
	return fmt.Errorf("%w: %s", ErrOpenAIImageStreamUpstream, message)
}

func (r *OpenAIImageResponse) recordMetadata(obj map[string]json.RawMessage) {
	if created, ok := asInt(field(obj, "created")); ok && created > 0 {
		r.Created = created
	}
	if model := strings.TrimSpace(getStr(obj, "model")); model != "" {
		r.Model = model
	}
	if usage := field(obj, "usage"); usage != nil {
		r.Usage = append(r.Usage[:0], usage...)
	}
}

func (r *OpenAIImageResponse) appendDataItems(obj map[string]json.RawMessage, seen map[string]bool) {
	items, ok := decodeArray(field(obj, "data"))
	if !ok {
		return
	}
	for _, rawItem := range items {
		item, ok := decodeObject(rawItem)
		if ok {
			r.appendImage(item, seen)
		}
	}
}

func (r *OpenAIImageResponse) appendImage(obj map[string]json.RawMessage, seen map[string]bool) {
	image := OpenAIImageData{
		B64JSON:       strings.TrimSpace(getStr(obj, "b64_json")),
		URL:           strings.TrimSpace(getStr(obj, "url")),
		OutputFormat:  strings.TrimSpace(getStr(obj, "output_format")),
		RevisedPrompt: strings.TrimSpace(getStr(obj, "revised_prompt")),
	}
	if image.B64JSON == "" && image.URL == "" {
		return
	}
	key := image.B64JSON + "\x00" + image.URL
	if seen[key] {
		return
	}
	seen[key] = true
	r.Data = append(r.Data, image)
}

func (r *OpenAIImageResponse) appendResponseImage(item map[string]json.RawMessage, seen map[string]bool) {
	if strings.TrimSpace(getStr(item, "type")) != "image_generation_call" {
		return
	}
	image := OpenAIImageData{
		B64JSON:       strings.TrimSpace(getStr(item, "result")),
		OutputFormat:  strings.TrimSpace(getStr(item, "output_format")),
		RevisedPrompt: strings.TrimSpace(getStr(item, "revised_prompt")),
	}
	if image.B64JSON == "" {
		return
	}
	key := image.B64JSON + "\x00"
	if seen[key] {
		return
	}
	seen[key] = true
	r.Data = append(r.Data, image)
}

func (r *OpenAIImageResponse) recordResponseMetadata(response map[string]json.RawMessage) {
	if created, ok := asInt(field(response, "created_at")); ok && created > 0 {
		r.Created = created
	}
	if model := strings.TrimSpace(getStr(response, "model")); model != "" {
		r.Model = model
	}
	usage := field(response, "tool_usage")
	if usage == nil {
		usage = field(response, "usage")
	}
	if usage != nil {
		r.Usage = append(r.Usage[:0], usage...)
	}
}

func splitOpenAIImageSSEBlocks(raw []byte) []string {
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n\n")
}

func parseOpenAIImageSSEBlock(block string) (string, json.RawMessage, bool) {
	block = strings.TrimSpace(block)
	if block == "" {
		return "", nil, false
	}
	var eventName string
	dataLines := make([]string, 0, 1)
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" && data != "[DONE]" {
				dataLines = append(dataLines, data)
			}
		}
	}
	if len(dataLines) == 0 {
		return "", nil, false
	}
	return eventName, json.RawMessage(strings.Join(dataLines, "\n")), true
}
