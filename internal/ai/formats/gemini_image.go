package formats

import "encoding/json"

// GeminiRequestImageIntent inspects a Gemini generateContent request and
// reports whether it is an image-generation/edit request. The second return
// value is true when the request appears to be image-editing (inline image
// input is present).
func GeminiRequestImageIntent(body []byte) (isImage bool, isEdit bool) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil || doc == nil {
		return false, false
	}
	if !geminiRequestIsImageGenerationBody(doc) {
		return false, false
	}
	return true, geminiRequestHasInlineImageInput(doc)
}

func geminiRequestIsImageGenerationBody(doc map[string]any) bool {
	generationConfig, ok := nestedObject(doc, "generationConfig", "generation_config")
	if !ok {
		return false
	}
	modalityValue, ok := generationConfig["responseModalities"]
	if !ok {
		modalityValue, ok = generationConfig["response_modalities"]
	}
	if !ok {
		return false
	}
	items, ok := modalityValue.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if s, ok := item.(string); ok && s == "IMAGE" {
			return true
		}
	}
	return false
}

func geminiRequestHasInlineImageInput(doc map[string]any) bool {
	contents, ok := doc["contents"].([]any)
	if !ok {
		return false
	}
	for _, item := range contents {
		content, ok := item.(map[string]any)
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
			if _, ok := part["inlineData"]; ok {
				return true
			}
			if _, ok := part["inline_data"]; ok {
				return true
			}
		}
	}
	return false
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
