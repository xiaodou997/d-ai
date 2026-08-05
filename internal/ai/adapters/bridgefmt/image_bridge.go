package bridgefmt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/serving"
)

const (
	bridgeMetaClientPath                 = "client_path"
	bridgeMetaContentType                = "content_type"
	bridgeMetaImageEditTransport         = "image_edit_transport"
	imagePartialImagesUnsupportedMessage = "partial_images is not supported; only the final completed image is returned."
)

type openAIImagePart struct {
	mimeType string
	data     string
	url      string
}

func (r *Runtime) registerImageBridges() {
	if r == nil {
		return
	}
	r.registerBridge(
		corebridge.Definition{Kind: corebridge.IRKindImage, Source: surface.OpenAIImages, Target: surface.GeminiImages},
		func(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
			rewritten, _, _, err := convertImageRequestDetailed(env, body)
			return rewritten, err
		},
		func(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
			return buildGeminiImageResponseFromOpenAI(env, body)
		},
		nil,
		nil,
	)
	r.registerBridge(
		corebridge.Definition{Kind: corebridge.IRKindImage, Source: surface.GeminiImages, Target: surface.OpenAIImages},
		func(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
			rewritten, _, _, err := convertImageRequestDetailed(env, body)
			return rewritten, err
		},
		func(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
			return buildOpenAIImageResponseFromGemini(env, body)
		},
		nil,
		nil,
	)
}

func (r *Runtime) prepareImageRequest(req *serving.Request, body []byte) (corebridge.PreparedRequest, error) {
	if req != nil && req.Candidate != nil && req.ClientProtocol == domain.ProtocolOpenAIImages &&
		req.Candidate.Protocol == domain.ProtocolOpenAIImages && strings.Contains(bridgeClientPath(req), "/images/edits") {
		ctx := context.Background()
		if req.Envelope != nil && req.Envelope.R != nil {
			ctx = req.Envelope.R.Context()
		}
		parsed, err := imageedit.Decode(body, requestContentType(requestEnvelope(req)))
		if err != nil {
			return corebridge.PreparedRequest{}, err
		}
		parsed.Model = req.Candidate.UpstreamModel
		encoded, err := imageedit.Encode(ctx, parsed, req.Candidate.ImageEditTransport)
		if err != nil {
			return corebridge.PreparedRequest{}, err
		}
		return corebridge.PreparedRequest{
			Body:        encoded.Body,
			ContentType: encoded.ContentType,
			RequestPath: requestPathFromEnvelope(requestEnvelope(req)),
		}, nil
	}

	rewritten, contentType, requestPath, err := convertImageRequestDetailed(requestEnvelope(req), body)
	if err != nil {
		return corebridge.PreparedRequest{}, err
	}
	return corebridge.PreparedRequest{
		Body:        rewritten,
		RequestPath: requestPath,
		ContentType: contentType,
	}, nil
}

func convertImageRequestDetailed(env corebridge.RequestEnvelope, body []byte) ([]byte, string, string, error) {
	if !isImageCapability(env.Capability) {
		return nil, "", "", fmt.Errorf("bridgefmt: unsupported image capability %q", env.Capability)
	}
	switch {
	case env.Source == surface.OpenAIImages && env.Target == surface.GeminiImages:
		out, err := buildGeminiImageRequestFromOpenAI(env, body)
		if err != nil {
			return nil, "", "", err
		}
		return out, "application/json", "", nil
	case env.Source == surface.GeminiImages && env.Target == surface.OpenAIImages:
		out, contentType, requestPath, err := buildOpenAIImageRequestFromGemini(env, body)
		if err != nil {
			return nil, "", "", err
		}
		return out, contentType, requestPath, nil
	default:
		return nil, "", "", fmt.Errorf("bridgefmt: unsupported image bridge %q -> %q", env.Source, env.Target)
	}
}

func convertImageResponse(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
	if !isImageCapability(env.Capability) {
		return nil, fmt.Errorf("bridgefmt: unsupported image capability %q", env.Capability)
	}
	switch {
	case env.Source == surface.GeminiImages && env.Target == surface.OpenAIImages:
		return buildOpenAIImageResponseFromGemini(env, body)
	case env.Source == surface.OpenAIImages && env.Target == surface.GeminiImages:
		return buildGeminiImageResponseFromOpenAI(env, body)
	default:
		return nil, fmt.Errorf("bridgefmt: unsupported image bridge %q -> %q", env.Source, env.Target)
	}
}

func (r *Runtime) BridgeImageStream(req *serving.Request, rawBody []byte) (corebridge.ImageStreamResult, error) {
	if req == nil || req.Candidate == nil {
		return corebridge.ImageStreamResult{}, fmt.Errorf("bridgefmt: missing image stream request")
	}
	// Stream bridge intentionally exposes only the final completed image.
	// Upstream preview frames such as partial_image are not surfaced.
	providerBody, err := aggregateImageProviderBody(req, rawBody)
	if err != nil {
		return corebridge.ImageStreamResult{}, err
	}
	clientBody, err := r.BridgeResponse(req, providerBody)
	if err != nil {
		return corebridge.ImageStreamResult{}, err
	}
	clientStream, err := buildImageClientStream(req, clientBody)
	if err != nil {
		return corebridge.ImageStreamResult{}, err
	}
	return corebridge.ImageStreamResult{
		ClientStream: clientStream,
		ProviderBody: providerBody,
	}, nil
}

func aggregateImageProviderBody(req *serving.Request, rawBody []byte) ([]byte, error) {
	if len(rawBody) == 0 {
		return nil, fmt.Errorf("bridgefmt: empty image stream body")
	}
	if !looksLikeSSEPayload(rawBody) {
		return rawBody, nil
	}
	switch req.Candidate.Protocol {
	case domain.ProtocolOpenAIImages:
		return aggregateOpenAIImageStreamBody(req, rawBody)
	case domain.ProtocolGeminiGenerate:
		return aggregateGeminiImageStreamBody(req, rawBody)
	default:
		return rawBody, nil
	}
}

func buildImageClientStream(req *serving.Request, clientBody []byte) ([]byte, error) {
	switch req.ClientProtocol {
	case domain.ProtocolOpenAIImages:
		return buildOpenAIImageClientStream(req, clientBody)
	case domain.ProtocolGeminiGenerate:
		return []byte("data: " + strings.TrimSpace(string(clientBody)) + "\n\n"), nil
	default:
		return nil, fmt.Errorf("bridgefmt: unsupported image client stream %q", req.ClientProtocol)
	}
}

func buildOpenAIImageClientStream(req *serving.Request, clientBody []byte) ([]byte, error) {
	eventName := "image_generation.completed"
	if strings.Contains(req.ClientPath, "/images/edits") {
		eventName = "image_edit.completed"
	}
	stream, err := formats.EncodeOpenAIImageCompletedSSE(clientBody, eventName)
	if err != nil {
		return nil, fmt.Errorf("bridgefmt: build openai image client stream: %w", err)
	}
	return stream, nil
}

func aggregateOpenAIImageStreamBody(_ *serving.Request, rawBody []byte) ([]byte, error) {
	aggregated, err := formats.AggregateOpenAIImageSSE(rawBody)
	if err != nil {
		return nil, fmt.Errorf("bridgefmt: %w", err)
	}
	return aggregated, nil
}

func aggregateGeminiImageStreamBody(_ *serving.Request, rawBody []byte) ([]byte, error) {
	var (
		responseID string
		model      string
		usage      any
		reason     string
		text       string
		images     []map[string]any
		seenB64    = map[string]bool{}
	)
	for _, block := range splitSSEBlocks(rawBody) {
		_, payload, ok := parseSSEBlock(block)
		if !ok {
			continue
		}
		doc, err := decodeObject(payload, "bridgefmt: parse gemini image stream event")
		if err != nil || doc == nil {
			continue
		}
		if s, ok := textValue(doc["responseId"]); ok && strings.TrimSpace(s) != "" {
			responseID = strings.TrimSpace(s)
		}
		if s, ok := textValue(doc["modelVersion"]); ok && strings.TrimSpace(s) != "" {
			model = strings.TrimSpace(s)
		}
		if v := doc["usageMetadata"]; v != nil {
			usage = v
		}
		if s, ok := textValue(doc["finishReason"]); ok && strings.TrimSpace(s) != "" {
			reason = strings.TrimSpace(s)
		}
		candidates, ok := doc["candidates"].([]any)
		if !ok {
			continue
		}
		for _, candValue := range candidates {
			cand, ok := candValue.(map[string]any)
			if !ok {
				continue
			}
			if s, ok := textValue(cand["finishReason"]); ok && strings.TrimSpace(s) != "" {
				reason = strings.TrimSpace(s)
			}
			content, ok := cand["content"].(map[string]any)
			if !ok {
				continue
			}
			parts, ok := content["parts"].([]any)
			if !ok {
				continue
			}
			for _, partValue := range parts {
				part, ok := partValue.(map[string]any)
				if !ok {
					continue
				}
				if partText, ok := textValue(part["text"]); ok && strings.TrimSpace(partText) != "" {
					text = strings.TrimSpace(partText)
				}
				mimeType, b64, ok := geminiInlineImage(part)
				if !ok {
					continue
				}
				if seenB64[b64] {
					continue
				}
				images = append(images, map[string]any{
					"inlineData": map[string]any{
						"mimeType": mimeType,
						"data":     b64,
					},
				})
				seenB64[b64] = true
			}
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("bridgefmt: gemini image stream contains no images")
	}
	parts := make([]map[string]any, 0, len(images)+1)
	if text != "" {
		parts = append(parts, map[string]any{"text": text})
	}
	for _, image := range images {
		parts = append(parts, image)
	}
	out := map[string]any{
		"candidates": []any{
			map[string]any{
				"index":   0,
				"content": map[string]any{"role": "model", "parts": parts},
			},
		},
	}
	if responseID != "" {
		out["responseId"] = responseID
	}
	if model != "" {
		out["modelVersion"] = model
	}
	if reason != "" {
		out["candidates"].([]any)[0].(map[string]any)["finishReason"] = reason
	}
	if usage != nil {
		out["usageMetadata"] = usage
	}
	return json.Marshal(out)
}

// rewriteOpenAIImageJSONFields prepares an OpenAI Images JSON body for the
// upstream. It sets stream and replaces the platform-only client response
// preference with the model binding's upstream preference. An empty preference
// removes response_format entirely.
// Non-object or unparseable bodies are returned as-is.
func rewriteOpenAIImageJSONFields(body []byte, stream bool, responseFormat string) []byte {
	if len(bytes.TrimSpace(body)) == 0 {
		return body
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil || doc == nil {
		return body
	}
	current, streamMatches := doc["stream"].(bool)
	rawResponseFormat, hasResponseFormat := doc["response_format"]
	currentResponseFormat, responseFormatIsString := rawResponseFormat.(string)
	responseFormatMatches := (responseFormat == "" && !hasResponseFormat) ||
		(responseFormatIsString && currentResponseFormat == responseFormat)
	if streamMatches && current == stream && responseFormatMatches {
		return body
	}
	doc["stream"] = stream
	if responseFormat == "" {
		delete(doc, "response_format")
	} else {
		doc["response_format"] = responseFormat
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return body
	}
	return out
}

func looksLikeSSEPayload(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") || strings.Contains(trimmed, "\ndata:")
}

func splitSSEBlocks(raw []byte) []string {
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n\n")
}

func parseSSEBlock(block string) (string, []byte, bool) {
	block = strings.TrimSpace(block)
	if block == "" {
		return "", nil, false
	}
	var event string
	var dataLines []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) == 0 {
		return event, nil, false
	}
	data := strings.Join(dataLines, "\n")
	if data == "" || data == "[DONE]" {
		return event, nil, false
	}
	return event, []byte(data), true
}

func encodeJSONSSE(event string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(event) == "" {
		return []byte("data: " + string(body) + "\n\n"), nil
	}
	return []byte("event: " + strings.TrimSpace(event) + "\ndata: " + string(body) + "\n\n"), nil
}

func buildGeminiImageRequestFromOpenAI(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
	req, err := parseOpenAIImageRequest(env, body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(req.requestedPath, "/images/edits") && len(req.images) == 0 {
		return nil, fmt.Errorf("bridgefmt: image edit request requires at least one input image")
	}
	prompt := strings.TrimSpace(req.prompt)
	if prompt == "" {
		if strings.Contains(req.requestedPath, "/images/edits") {
			prompt = "Edit the provided image."
		} else {
			prompt = "Generate a high quality image."
		}
	}
	parts := make([]map[string]any, 0, len(req.images)+1)
	if prompt != "" {
		parts = append(parts, map[string]any{"text": prompt})
	}
	for _, image := range req.images {
		if p := openAIImagePartToGemini(image); p != nil {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("bridgefmt: empty image request")
	}
	generationConfig := map[string]any{
		"responseModalities": []string{"TEXT", "IMAGE"},
	}
	if req.size != "" {
		generationConfig["imageSize"] = req.size
	}
	if req.count > domain.DefaultImageOutputCount {
		generationConfig["candidateCount"] = req.count
	}
	out := map[string]any{
		"model":            env.TargetModel,
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": generationConfig,
	}
	return json.Marshal(out)
}

func buildOpenAIImageRequestFromGemini(env corebridge.RequestEnvelope, body []byte) ([]byte, string, string, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, "", "", fmt.Errorf("bridgefmt: parse gemini image request: %w", err)
	}
	if doc == nil {
		return nil, "", "", fmt.Errorf("bridgefmt: empty gemini image request")
	}
	if !geminiImageRequestIntent(doc) {
		return nil, "", "", fmt.Errorf("bridgefmt: gemini request is not an image request")
	}

	promptParts := make([]string, 0, 4)
	contentParts := make([]map[string]any, 0, 8)
	collectGeminiImageTexts(doc["systemInstruction"], &promptParts)
	collectGeminiImageTexts(doc["system_instruction"], &promptParts)
	collectGeminiImageContents(doc["contents"], &promptParts, &contentParts)
	generationConfig, _ := nestedObject(doc, "generationConfig", "generation_config")
	count, err := imageOutputCountFromGenerationConfig(generationConfig)
	if err != nil {
		return nil, "", "", err
	}
	prompt := strings.TrimSpace(strings.Join(promptParts, "\n\n"))
	action := "generate"
	for _, part := range contentParts {
		if part["type"] == "input_image" {
			action = "edit"
			break
		}
	}
	if prompt == "" {
		if action == "edit" {
			prompt = "Edit the provided image."
		} else {
			prompt = "Generate a high quality image."
		}
	}
	if prompt != "" {
		contentParts = append([]map[string]any{{"type": "input_text", "text": prompt}}, contentParts...)
	}
	if len(contentParts) == 0 {
		return nil, "", "", fmt.Errorf("bridgefmt: gemini image request has no prompt or images")
	}

	if action == "edit" {
		body, contentType, err := buildOpenAIImageEditRequest(contentParts, env.TargetModel, generationConfig, count, requestImageEditTransport(env))
		if err != nil {
			return nil, "", "", err
		}
		return body, contentType, "/v1/images/edits", nil
	}

	out := map[string]any{
		"model":  env.TargetModel,
		"prompt": prompt,
		"stream": false,
	}
	if size := strings.TrimSpace(textFromGenerationConfig(generationConfig, "imageSize", "image_size")); size != "" {
		out["size"] = size
	}
	if count > domain.DefaultImageOutputCount {
		out["n"] = count
	}
	body, err = json.Marshal(out)
	if err != nil {
		return nil, "", "", err
	}
	return body, "application/json", "/v1/images/generations", nil
}

func buildOpenAIImageResponseFromGemini(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("bridgefmt: parse gemini image response: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty gemini image response")
	}
	created := env.ReceivedAt.Unix()
	if created == 0 {
		created = time.Now().Unix()
	}
	data := make([]map[string]any, 0, 4)
	var revisedPrompt string
	if candidates, ok := doc["candidates"].([]any); ok {
		for _, candidateValue := range candidates {
			candidate, _ := candidateValue.(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, partValue := range parts {
				part, _ := partValue.(map[string]any)
				if text, ok := textValue(part["text"]); ok {
					revisedPrompt = text
				}
				mimeType, b64, ok := geminiInlineImage(part)
				if !ok {
					continue
				}
				item := map[string]any{
					"b64_json":      b64,
					"output_format": outputFormatFromMimeType(mimeType),
				}
				if revisedPrompt != "" {
					item["revised_prompt"] = revisedPrompt
				}
				data = append(data, item)
			}
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("bridgefmt: gemini response contains no images")
	}
	out := map[string]any{
		"created": created,
		"model":   env.Model,
		"data":    data,
	}
	if usage := geminiUsageToOpenAIUsage(doc["usageMetadata"]); usage != nil {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func buildGeminiImageResponseFromOpenAI(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("bridgefmt: parse openai image response: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty openai image response")
	}
	created := env.ReceivedAt.Unix()
	if created == 0 {
		created = time.Now().Unix()
	}
	candidates := make([]any, 0, 4)
	if data, ok := doc["data"].([]any); ok {
		for index, itemValue := range data {
			item, _ := itemValue.(map[string]any)
			if item == nil {
				continue
			}
			parts := make([]map[string]any, 0, 2)
			if text, ok := textValue(item["revised_prompt"]); ok {
				parts = append(parts, map[string]any{"text": text})
			}
			mimeType, b64, ok := openAIImageOutputItem(item)
			if !ok {
				continue
			}
			parts = append(parts, map[string]any{
				"inlineData": map[string]any{
					"mimeType": mimeType,
					"data":     b64,
				},
			})
			candidates = append(candidates, map[string]any{
				"index":        index,
				"content":      map[string]any{"role": "model", "parts": parts},
				"finishReason": "STOP",
			})
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("bridgefmt: openai response contains no images")
	}
	out := map[string]any{
		"modelVersion": env.Model,
		"candidates":   candidates,
	}
	if usage := openAIUsageToGeminiUsage(doc["usage"]); usage != nil {
		out["usageMetadata"] = usage
	}
	if created > 0 {
		out["created"] = created
	}
	return json.Marshal(out)
}

type parsedOpenAIImageRequest struct {
	prompt        string
	images        []openAIImagePart
	size          string
	count         int
	requestedPath string
}

func parseOpenAIImageRequest(env corebridge.RequestEnvelope, body []byte) (*parsedOpenAIImageRequest, error) {
	contentType := requestContentType(env)
	requestedPath := requestPathFromEnvelope(env)
	if strings.Contains(requestedPath, "/images/edits") {
		decoded, err := imageedit.Decode(body, contentType)
		if err != nil {
			return nil, err
		}
		if decoded.Mask != nil {
			return nil, fmt.Errorf("bridgefmt: image mask inputs are not supported by the Gemini image bridge")
		}
		request := &parsedOpenAIImageRequest{
			prompt:        decoded.Prompt,
			size:          decoded.Size,
			count:         normalizedImageOutputCount(decoded.N),
			requestedPath: requestedPath,
		}
		for _, source := range decoded.Images {
			part := openAIImagePart{mimeType: source.MIMEType, url: source.URL}
			if len(source.Data) > 0 {
				part.data = base64.StdEncoding.EncodeToString(source.Data)
			}
			request.images = append(request.images, part)
		}
		return request, nil
	}
	if isMultipartContentType(contentType) {
		return nil, fmt.Errorf("bridgefmt: image generations require application/json")
	}
	return parseOpenAIImageJSONRequest(env, body)
}

func parseOpenAIImageJSONRequest(env corebridge.RequestEnvelope, body []byte) (*parsedOpenAIImageRequest, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("bridgefmt: parse openai image request: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty openai image request")
	}
	if _, ok := doc["partial_images"]; ok {
		return nil, unsupportedImageRequestOptionError(imagePartialImagesUnsupportedMessage)
	}
	count := domain.DefaultImageOutputCount
	if rawCount, ok := doc["n"]; ok {
		parsed, valid := integerFromAny(rawCount)
		if !valid || parsed < 1 || parsed > domain.MaxImageOutputCount {
			return nil, unsupportedImageRequestOptionError("n must be an integer between 1 and 10")
		}
		count = int(parsed)
	}
	req := &parsedOpenAIImageRequest{
		prompt:        strings.TrimSpace(textFromAny(doc["prompt"])),
		size:          strings.TrimSpace(textFromAny(doc["size"])),
		count:         count,
		requestedPath: requestPathFromEnvelope(env),
	}
	return req, nil
}

func unsupportedImageRequestOptionError(message string) error {
	return &serving.APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_request_error",
		Message: message,
	}
}

func buildOpenAIImageEditRequest(parts []map[string]any, model string, generationConfig map[string]any, count int, transport string) ([]byte, string, error) {
	prompt := ""
	images := make([]map[string]string, 0, len(parts))
	for _, part := range parts {
		if text, ok := textValue(part["text"]); ok {
			prompt = text
			continue
		}
		if part["type"] == "input_image" {
			if imageURL := strings.TrimSpace(textFromAny(part["image_url"])); imageURL != "" {
				images = append(images, map[string]string{"image_url": imageURL})
			}
		}
	}
	payload := map[string]any{
		"model":  model,
		"prompt": prompt,
		"images": images,
	}
	if count > domain.DefaultImageOutputCount {
		payload["n"] = count
	}
	if size := strings.TrimSpace(textFromGenerationConfig(generationConfig, "imageSize", "image_size")); size != "" {
		payload["size"] = size
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	request, err := imageedit.Decode(body, imageedit.TransportJSON)
	if err != nil {
		return nil, "", err
	}
	encoded, err := imageedit.Encode(context.Background(), request, transport)
	if err != nil {
		return nil, "", err
	}
	return encoded.Body, encoded.ContentType, nil
}

func openAIImagePartToGemini(image openAIImagePart) map[string]any {
	if image.data != "" {
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": normalizeImageMimeType(image.mimeType),
				"data":     image.data,
			},
		}
	}
	if image.url != "" {
		if mimeType, data, ok := parseDataURL(image.url); ok {
			return map[string]any{
				"inlineData": map[string]any{
					"mimeType": mimeType,
					"data":     data,
				},
			}
		}
		return map[string]any{
			"fileData": map[string]any{
				"mimeType": mimeTypeFromURL(image.url),
				"fileUri":  image.url,
			},
		}
	}
	return nil
}

func collectGeminiImageTexts(value any, text *[]string) {
	switch typed := value.(type) {
	case string:
		if s := strings.TrimSpace(typed); s != "" {
			*text = append(*text, s)
		}
	case map[string]any:
		if parts, ok := typed["parts"].([]any); ok {
			for _, part := range parts {
				if m, ok := part.(map[string]any); ok {
					if s, ok := textValue(m["text"]); ok {
						*text = append(*text, s)
					}
				}
			}
			return
		}
		if s, ok := textValue(typed["text"]); ok {
			*text = append(*text, s)
		}
	case []any:
		for _, item := range typed {
			collectGeminiImageTexts(item, text)
		}
	}
}

func collectGeminiImageContents(value any, promptParts *[]string, contentParts *[]map[string]any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectGeminiImageContent(item, promptParts, contentParts)
		}
	case map[string]any:
		collectGeminiImageContent(typed, promptParts, contentParts)
	}
}

func collectGeminiImageContent(value any, promptParts *[]string, contentParts *[]map[string]any) {
	obj, ok := value.(map[string]any)
	if !ok {
		return
	}
	if parts, ok := obj["parts"].([]any); ok {
		for _, part := range parts {
			collectGeminiImagePart(part, promptParts, contentParts)
		}
		return
	}
	collectGeminiImagePart(obj, promptParts, contentParts)
}

func collectGeminiImagePart(value any, promptParts *[]string, contentParts *[]map[string]any) {
	part, ok := value.(map[string]any)
	if !ok {
		return
	}
	if s, ok := textValue(part["text"]); ok {
		*promptParts = append(*promptParts, s)
		return
	}
	if mimeType, data, ok := geminiInlineImage(part); ok {
		*contentParts = append(*contentParts, map[string]any{
			"type":      "input_image",
			"image_url": formatDataURL(mimeType, data),
		})
		return
	}
	fileData, ok := nestedObjectValue(part, "fileData", "file_data")
	if ok {
		fileURI := textFromAny(fileData["fileUri"])
		if fileURI == "" {
			fileURI = textFromAny(fileData["file_uri"])
		}
		if fileURI != "" {
			*contentParts = append(*contentParts, map[string]any{
				"type":      "input_image",
				"image_url": fileURI,
			})
		}
	}
}

func geminiImageRequestIntent(doc map[string]any) bool {
	generationConfig, ok := nestedObject(doc, "generationConfig", "generation_config")
	if !ok {
		return false
	}
	modality, ok := generationConfig["responseModalities"]
	if !ok {
		modality, ok = generationConfig["response_modalities"]
	}
	if !ok {
		return false
	}
	items, ok := modality.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(textFromAny(item)), "IMAGE") {
			return true
		}
	}
	return false
}

func geminiInlineImage(part map[string]any) (string, string, bool) {
	inline, ok := nestedObjectValue(part, "inlineData", "inline_data")
	if !ok {
		return "", "", false
	}
	mimeType := normalizeImageMimeType(textFromAny(inline["mimeType"]))
	if mimeType == "" {
		mimeType = normalizeImageMimeType(textFromAny(inline["mime_type"]))
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	data := strings.TrimSpace(textFromAny(inline["data"]))
	if data == "" {
		return "", "", false
	}
	return mimeType, data, true
}

func openAIImageOutputItem(item map[string]any) (string, string, bool) {
	if b64 := strings.TrimSpace(textFromAny(item["b64_json"])); b64 != "" {
		mimeType := normalizeOutputFormat(textFromAny(item["output_format"]))
		return mimeType, b64, true
	}
	if url := strings.TrimSpace(textFromAny(item["url"])); url != "" {
		return parseDataURL(url)
	}
	return "", "", false
}

func geminiUsageToOpenAIUsage(value any) map[string]any {
	usage, ok := value.(map[string]any)
	if !ok || usage == nil {
		return nil
	}
	inputTokens := uintFromAny(usage["promptTokenCount"])
	if inputTokens == 0 {
		inputTokens = uintFromAny(usage["prompt_token_count"])
	}
	outputTokens := uintFromAny(usage["candidatesTokenCount"])
	if outputTokens == 0 {
		outputTokens = uintFromAny(usage["candidates_token_count"])
	}
	totalTokens := uintFromAny(usage["totalTokenCount"])
	if totalTokens == 0 {
		totalTokens = uintFromAny(usage["total_token_count"])
	}
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	return map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"total_tokens":  totalTokens,
	}
}

func openAIUsageToGeminiUsage(value any) map[string]any {
	usage, ok := value.(map[string]any)
	if !ok || usage == nil {
		return nil
	}
	inputTokens := uintFromAny(usage["input_tokens"])
	if inputTokens == 0 {
		inputTokens = uintFromAny(usage["prompt_tokens"])
	}
	outputTokens := uintFromAny(usage["output_tokens"])
	if outputTokens == 0 {
		outputTokens = uintFromAny(usage["completion_tokens"])
	}
	totalTokens := uintFromAny(usage["total_tokens"])
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	return map[string]any{
		"promptTokenCount":     inputTokens,
		"candidatesTokenCount": outputTokens,
		"totalTokenCount":      totalTokens,
	}
}

func normalizeImageMimeType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	case "image/png":
		return "image/png"
	default:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
			return strings.TrimSpace(mimeType)
		}
		return "image/png"
	}
}

func normalizeOutputFormat(outputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func outputFormatFromMimeType(mimeType string) string {
	switch normalizeImageMimeType(mimeType) {
	case "image/jpeg":
		return "jpeg"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}

func mimeTypeFromURL(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	default:
		return "image/png"
	}
}

func parseDataURL(value string) (string, string, bool) {
	metadata, payload, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok {
		return "", "", false
	}
	metadata = strings.TrimPrefix(metadata, "data:")
	if !strings.HasSuffix(metadata, ";base64") {
		return "", "", false
	}
	mimeType := strings.TrimSuffix(metadata, ";base64")
	mimeType = normalizeImageMimeType(mimeType)
	if payload == "" {
		return "", "", false
	}
	return mimeType, payload, true
}

func formatDataURL(mimeType, data string) string {
	mimeType = normalizeImageMimeType(mimeType)
	return "data:" + mimeType + ";base64," + data
}

func textValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		if s := strings.TrimSpace(typed); s != "" {
			return s, true
		}
	case fmt.Stringer:
		if s := strings.TrimSpace(typed.String()); s != "" {
			return s, true
		}
	}
	return "", false
}

func textFromAny(value any) string {
	text, _ := textValue(value)
	return text
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return int(n)
		}
	case string:
		return intFromString(typed)
	}
	return 0
}

func intFromString(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func uintFromAny(value any) uint64 {
	switch typed := value.(type) {
	case uint:
		return uint64(typed)
	case uint64:
		return typed
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case int32:
		if typed > 0 {
			return uint64(typed)
		}
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case float64:
		if typed > 0 {
			return uint64(typed)
		}
	case json.Number:
		if n, err := typed.Int64(); err == nil && n > 0 {
			return uint64(n)
		}
	case string:
		if n, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func isMultipartContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "multipart/form-data"
}

func requestContentType(env corebridge.RequestEnvelope) string {
	if env.Metadata == nil {
		return ""
	}
	if value, ok := env.Metadata[bridgeMetaContentType].(string); ok {
		return value
	}
	return ""
}

func requestPathFromEnvelope(env corebridge.RequestEnvelope) string {
	if env.Metadata == nil {
		return ""
	}
	if value, ok := env.Metadata[bridgeMetaClientPath].(string); ok {
		return value
	}
	return ""
}

func requestImageEditTransport(env corebridge.RequestEnvelope) string {
	if env.Metadata != nil {
		if value, ok := env.Metadata[bridgeMetaImageEditTransport].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return imageedit.TransportMultipart
}

func normalizedImageOutputCount(count int) int {
	if count < 1 {
		return domain.DefaultImageOutputCount
	}
	return count
}

func imageOutputCountFromGenerationConfig(config map[string]any) (int, error) {
	count := domain.DefaultImageOutputCount
	for _, key := range []string{"candidateCount", "candidate_count"} {
		value, ok := config[key]
		if !ok {
			continue
		}
		parsed, valid := integerFromAny(value)
		if !valid || parsed < 1 || parsed > domain.MaxImageOutputCount {
			return 0, unsupportedImageRequestOptionError("candidateCount must be an integer between 1 and 10")
		}
		count = int(parsed)
		break
	}
	return count, nil
}

func textFromGenerationConfig(generationConfig map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := generationConfig[key]; ok {
			if text := strings.TrimSpace(textFromAny(value)); text != "" {
				return text
			}
		}
	}
	return ""
}

func nestedObject(doc map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if value, ok := doc[key]; ok {
			if obj, ok := value.(map[string]any); ok {
				return obj, true
			}
		}
	}
	return nil, false
}

func nestedObjectValue(doc map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if value, ok := doc[key]; ok {
			if obj, ok := value.(map[string]any); ok {
				return obj, true
			}
		}
	}
	return nil, false
}
