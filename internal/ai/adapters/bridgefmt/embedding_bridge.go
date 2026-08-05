package bridgefmt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/serving"
)

type embeddingInputKind string

const (
	embeddingInputString          embeddingInputKind = "string"
	embeddingInputStringArray     embeddingInputKind = "string_array"
	embeddingInputTokenArray      embeddingInputKind = "token_array"
	embeddingInputTokenArrayArray embeddingInputKind = "token_array_array"
)

type parsedEmbeddingRequest struct {
	inputs               []string
	inputKind            embeddingInputKind
	outputDimensionality *uint64
}

func (r *Runtime) registerEmbeddingBridges() {
	if r == nil {
		return
	}
	r.registerBridge(
		corebridge.Definition{Kind: corebridge.IRKindEmbedding, Source: surface.OpenAIEmbeddings, Target: surface.GeminiEmbeddings},
		func(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
			return buildGeminiEmbeddingRequestFromOpenAI(env, body)
		},
		func(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
			return buildGeminiEmbeddingResponseFromOpenAI(env, body)
		},
		nil,
		nil,
	)
	r.registerBridge(
		corebridge.Definition{Kind: corebridge.IRKindEmbedding, Source: surface.GeminiEmbeddings, Target: surface.OpenAIEmbeddings},
		func(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
			return buildOpenAIEmbeddingRequestFromGemini(env, body)
		},
		func(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
			return buildOpenAIEmbeddingResponseFromGemini(env, body)
		},
		nil,
		nil,
	)
}

func buildGeminiEmbeddingRequestFromOpenAI(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
	req, err := parseOpenAIEmbeddingRequest(body)
	if err != nil {
		return nil, err
	}
	if req.inputKind == embeddingInputTokenArray || req.inputKind == embeddingInputTokenArrayArray {
		return nil, &serving.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request_error",
			Message: "Gemini embeddings bridge only supports string input; token-array input cannot be converted.",
		}
	}
	if len(req.inputs) == 0 {
		return nil, fmt.Errorf("bridgefmt: empty openai embedding request")
	}
	out := map[string]any{
		"model": env.TargetModel,
	}
	if req.outputDimensionality != nil {
		out["outputDimensionality"] = *req.outputDimensionality
	}
	if len(req.inputs) == 1 {
		out["content"] = map[string]any{
			"parts": []any{
				map[string]any{"text": req.inputs[0]},
			},
		}
	} else {
		requests := make([]any, 0, len(req.inputs))
		for _, text := range req.inputs {
			requests = append(requests, map[string]any{
				"model": env.TargetModel,
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": text},
					},
				},
			})
		}
		out["requests"] = requests
	}
	return json.Marshal(out)
}

func buildOpenAIEmbeddingRequestFromGemini(env corebridge.RequestEnvelope, body []byte) ([]byte, error) {
	req, err := parseGeminiEmbeddingRequest(body)
	if err != nil {
		return nil, err
	}
	if len(req.inputs) == 0 {
		return nil, fmt.Errorf("bridgefmt: empty gemini embedding request")
	}
	out := map[string]any{
		"model": env.TargetModel,
	}
	if req.outputDimensionality != nil {
		out["dimensions"] = *req.outputDimensionality
	}
	if len(req.inputs) == 1 {
		out["input"] = req.inputs[0]
	} else {
		inputs := make([]string, len(req.inputs))
		copy(inputs, req.inputs)
		out["input"] = inputs
	}
	return json.Marshal(out)
}

func buildOpenAIEmbeddingResponseFromGemini(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
	doc, err := decodeObject(body, "bridgefmt: parse gemini embedding response")
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty gemini embedding response")
	}
	vectors, err := geminiEmbeddingVectors(doc)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("bridgefmt: gemini embedding response contains no vectors")
	}
	data := make([]map[string]any, 0, len(vectors))
	for i, vector := range vectors {
		data = append(data, map[string]any{
			"object":    "embedding",
			"index":     i,
			"embedding": vector,
		})
	}
	out := map[string]any{
		"object": "list",
		"model":  env.Model,
		"data":   data,
	}
	if usage := geminiUsageToOpenAIUsage(doc["usageMetadata"]); usage != nil {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func buildGeminiEmbeddingResponseFromOpenAI(env corebridge.ResponseEnvelope, body []byte) ([]byte, error) {
	doc, err := decodeObject(body, "bridgefmt: parse openai embedding response")
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty openai embedding response")
	}
	vectors, err := openAIEmbeddingVectors(doc)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("bridgefmt: openai embedding response contains no vectors")
	}
	out := map[string]any{
		"modelVersion": env.Model,
	}
	if len(vectors) == 1 {
		out["embedding"] = map[string]any{"values": vectors[0]}
	} else {
		items := make([]map[string]any, 0, len(vectors))
		for _, vector := range vectors {
			items = append(items, map[string]any{"values": vector})
		}
		out["embeddings"] = items
	}
	if usage := openAIUsageToGeminiUsage(doc["usage"]); usage != nil {
		out["usageMetadata"] = usage
	}
	return json.Marshal(out)
}

func parseOpenAIEmbeddingRequest(body []byte) (*parsedEmbeddingRequest, error) {
	doc, err := decodeObject(body, "bridgefmt: parse openai embedding request")
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty openai embedding request")
	}
	if model := strings.TrimSpace(textFromAny(doc["model"])); model == "" {
		return nil, fmt.Errorf("bridgefmt: openai embedding request missing model")
	}
	inputKind, inputs, err := parseOpenAIEmbeddingInput(doc["input"])
	if err != nil {
		return nil, err
	}
	if enc := strings.TrimSpace(textFromAny(doc["encoding_format"])); enc != "" && enc != "float" {
		return nil, fmt.Errorf("bridgefmt: unsupported openai embedding encoding_format %q", enc)
	}
	out := &parsedEmbeddingRequest{inputs: inputs, inputKind: inputKind}
	if dimensions := uintFromAny(doc["dimensions"]); dimensions > 0 {
		out.outputDimensionality = &dimensions
	}
	return out, nil
}

func parseGeminiEmbeddingRequest(body []byte) (*parsedEmbeddingRequest, error) {
	doc, err := decodeObject(body, "bridgefmt: parse gemini embedding request")
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("bridgefmt: empty gemini embedding request")
	}
	inputs, err := geminiEmbeddingInputs(doc)
	if err != nil {
		return nil, err
	}
	out := &parsedEmbeddingRequest{inputs: inputs}
	dimensions := uintFromAny(doc["outputDimensionality"])
	if dimensions == 0 {
		dimensions = uintFromAny(doc["output_dimensionality"])
	}
	if dimensions > 0 {
		out.outputDimensionality = &dimensions
	}
	return out, nil
}

func parseOpenAIEmbeddingInput(value any) (embeddingInputKind, []string, error) {
	switch v := value.(type) {
	case string:
		if text := strings.TrimSpace(v); text != "" {
			return embeddingInputString, []string{text}, nil
		}
		return "", nil, fmt.Errorf("bridgefmt: openai embedding input is empty")
	case []any:
		if isTokenArray(v) {
			return embeddingInputTokenArray, nil, nil
		}
		if isTokenArrayArray(v) {
			return embeddingInputTokenArrayArray, nil, nil
		}
		inputs := make([]string, 0, len(v))
		for _, item := range v {
			text, ok := textValue(item)
			if !ok || strings.TrimSpace(text) == "" {
				return "", nil, fmt.Errorf("bridgefmt: openai embedding input must be string, string array, token array, or token array array")
			}
			inputs = append(inputs, strings.TrimSpace(text))
		}
		if len(inputs) == 0 {
			return "", nil, fmt.Errorf("bridgefmt: openai embedding input is empty")
		}
		return embeddingInputStringArray, inputs, nil
	case nil:
		return "", nil, fmt.Errorf("bridgefmt: openai embedding input missing")
	default:
		text, ok := textValue(v)
		if !ok || strings.TrimSpace(text) == "" {
			return "", nil, fmt.Errorf("bridgefmt: openai embedding input must be string, string array, token array, or token array array")
		}
		return embeddingInputString, []string{strings.TrimSpace(text)}, nil
	}
}

func isTokenArray(values []any) bool {
	if len(values) == 0 {
		return false
	}
	for _, item := range values {
		if _, ok := integerFromAny(item); !ok {
			return false
		}
	}
	return true
}

func isTokenArrayArray(values []any) bool {
	if len(values) == 0 {
		return false
	}
	for _, item := range values {
		inner, ok := item.([]any)
		if !ok || !isTokenArray(inner) {
			return false
		}
	}
	return true
}

func geminiEmbeddingInputs(doc map[string]any) ([]string, error) {
	if reqs, ok := doc["requests"].([]any); ok && len(reqs) > 0 {
		inputs := make([]string, 0, len(reqs))
		for _, req := range reqs {
			text, err := geminiEmbeddingText(req)
			if err != nil {
				return nil, err
			}
			inputs = append(inputs, text)
		}
		return inputs, nil
	}
	text, err := geminiEmbeddingText(doc["content"])
	if err != nil {
		return nil, err
	}
	return []string{text}, nil
}

func geminiEmbeddingText(value any) (string, error) {
	switch v := value.(type) {
	case string:
		if text := strings.TrimSpace(v); text != "" {
			return text, nil
		}
	case map[string]any:
		if text, err := geminiEmbeddingText(v["content"]); err == nil {
			return text, nil
		}
		if text, err := geminiEmbeddingTextFromContent(v["parts"]); err == nil {
			return text, nil
		}
		if text, err := geminiEmbeddingTextFromContent(v); err == nil {
			return text, nil
		}
	case []any:
		if text, err := geminiEmbeddingTextFromContent(v); err == nil {
			return text, nil
		}
	}
	return "", fmt.Errorf("bridgefmt: gemini embedding request must contain text content")
}

func geminiEmbeddingTextFromContent(value any) (string, error) {
	parts, ok := value.([]any)
	if !ok {
		if text, ok := textValue(value); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
		return "", fmt.Errorf("bridgefmt: gemini embedding content missing parts")
	}
	texts := make([]string, 0, len(parts))
	for _, partValue := range parts {
		part, ok := partValue.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := textValue(part["text"]); ok && strings.TrimSpace(text) != "" {
			texts = append(texts, strings.TrimSpace(text))
			continue
		}
		if text, ok := textValue(part["input_text"]); ok && strings.TrimSpace(text) != "" {
			texts = append(texts, strings.TrimSpace(text))
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("bridgefmt: gemini embedding content has no text parts")
	}
	return strings.Join(texts, "\n"), nil
}

func openAIEmbeddingVectors(doc map[string]any) ([][]float64, error) {
	data, ok := doc["data"].([]any)
	if !ok || len(data) == 0 {
		return nil, fmt.Errorf("bridgefmt: openai embedding response missing data")
	}
	vectors := make([][]float64, 0, len(data))
	for _, itemValue := range data {
		item, ok := itemValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("bridgefmt: openai embedding response item must be object")
		}
		vector, err := floatSliceFromAny(item["embedding"])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func geminiEmbeddingVectors(doc map[string]any) ([][]float64, error) {
	if item, ok := doc["embedding"].(map[string]any); ok {
		vector, err := geminiEmbeddingVectorFromObject(item)
		if err != nil {
			return nil, err
		}
		return [][]float64{vector}, nil
	}
	if items, ok := doc["embeddings"].([]any); ok && len(items) > 0 {
		vectors := make([][]float64, 0, len(items))
		for _, itemValue := range items {
			item, ok := itemValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("bridgefmt: gemini embedding response item must be object")
			}
			vector, err := geminiEmbeddingVectorFromObject(item)
			if err != nil {
				return nil, err
			}
			vectors = append(vectors, vector)
		}
		return vectors, nil
	}
	return nil, fmt.Errorf("bridgefmt: gemini embedding response missing embedding data")
}

func geminiEmbeddingVectorFromObject(item map[string]any) ([]float64, error) {
	if values, err := floatSliceFromAny(item["values"]); err == nil {
		return values, nil
	}
	if values, err := floatSliceFromAny(item["embedding"]); err == nil {
		return values, nil
	}
	if embedding, ok := item["embedding"].(map[string]any); ok {
		if values, err := floatSliceFromAny(embedding["values"]); err == nil {
			return values, nil
		}
	}
	return nil, fmt.Errorf("bridgefmt: gemini embedding vector missing values")
}

func floatSliceFromAny(value any) ([]float64, error) {
	switch v := value.(type) {
	case []any:
		out := make([]float64, 0, len(v))
		for _, item := range v {
			f, ok := floatFromAny(item)
			if !ok {
				return nil, fmt.Errorf("bridgefmt: embedding vector contains non-numeric value")
			}
			out = append(out, f)
		}
		return out, nil
	default:
		if value == nil {
			return nil, fmt.Errorf("bridgefmt: embedding vector missing")
		}
		return nil, fmt.Errorf("bridgefmt: embedding vector has unsupported type %T", value)
	}
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func integerFromAny(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
		return 0, false
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func decodeObject(body []byte, prefix string) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", prefix, err)
	}
	return doc, nil
}
