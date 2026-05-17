// Package gemini implements Google Gemini GenerateContent API types and their
// bidirectional conversion to/from the gateway's canonical format.
package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"uni-ai-api/backend/internal/formats/canonical"
)

// ============================================================================
// Gemini GenerateContent API wire types
// ============================================================================

type GenerateContentRequest struct {
	Contents          []Content         `json:"contents"`
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Tools             []GeminiTool      `json:"tools,omitempty"`
	ToolConfig        *ToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings    []SafetySetting   `json:"safetySettings,omitempty"`
}

type Content struct {
	Role  string `json:"role,omitempty"` // "user" | "model"
	Parts []Part `json:"parts"`
}

type Part struct {
	Text         string        `json:"text,omitempty"`
	InlineData   *InlineData   `json:"inlineData,omitempty"`
	FileData     *FileData     `json:"fileData,omitempty"`
	FunctionCall *FunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

type InlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"` // base64-encoded
}

type FileData struct {
	MIMEType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type FunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type FunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type GeminiTool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type FunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type FunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // "AUTO"|"ANY"|"NONE"
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GenerationConfig struct {
	MaxOutputTokens  *int     `json:"maxOutputTokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	TopK             *int     `json:"topK,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	CandidateCount   *int     `json:"candidateCount,omitempty"`
	ResponseMIMEType string   `json:"responseMimeType,omitempty"`
	ThinkingConfig   *ThinkingConfig `json:"thinkingConfig,omitempty"`
}

type ThinkingConfig struct {
	ThinkingBudget int  `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// ============================================================================
// Gemini response types
// ============================================================================

type GenerateContentResponse struct {
	Candidates    []Candidate   `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string        `json:"modelVersion,omitempty"`
}

type Candidate struct {
	Index        int     `json:"index"`
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason"`
	SafetyRatings []SafetyRating `json:"safetyRatings,omitempty"`
}

type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type UsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
}

// Stream chunk for Gemini (each SSE data: line is a full GenerateContentResponse)
type StreamChunk = GenerateContentResponse

// ============================================================================
// Conversion: canonical → Gemini (for forwarding to Google upstream)
// ============================================================================

func GenerateContentRequestFromCanonical(req *canonical.ChatRequest, upstreamModel string) (*GenerateContentRequest, error) {
	out := &GenerateContentRequest{
		GenerationConfig: &GenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			StopSequences:   req.Stop,
		},
	}

	// Thinking / reasoning
	if req.ThinkingBudget != nil && *req.ThinkingBudget > 0 {
		out.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			ThinkingBudget: *req.ThinkingBudget,
		}
	} else if req.ReasoningEffort != nil {
		budget := reasoningEffortToBudget(*req.ReasoningEffort)
		out.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			ThinkingBudget: budget,
		}
	}

	// Tools
	if len(req.Tools) > 0 {
		tool := GeminiTool{}
		for _, t := range req.Tools {
			tool.FunctionDeclarations = append(tool.FunctionDeclarations, FunctionDeclaration{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		out.Tools = []GeminiTool{tool}
	}

	// Build callID→funcName map from all assistant messages for tool result lookup
	callIDToName := make(map[string]string)
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			callIDToName[tc.ID] = tc.Function.Name
		}
	}

	// Messages — extract system message and convert the rest
	for _, m := range req.Messages {
		if m.Role == canonical.RoleSystem {
			out.SystemInstruction = &Content{
				Parts: []Part{{Text: m.TextContent()}},
			}
			continue
		}
		gc, err := contentFromCanonical(m, callIDToName)
		if err != nil {
			return nil, err
		}
		out.Contents = append(out.Contents, gc)
	}

	return out, nil
}

func contentFromCanonical(m canonical.Message, callIDToName map[string]string) (Content, error) {
	role := canonicalRoleToGemini(m.Role)

	// Tool call (assistant → model)
	if len(m.ToolCalls) > 0 {
		var parts []Part
		if m.Content != "" {
			parts = append(parts, Part{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			var args json.RawMessage
			if tc.Function.Arguments != "" {
				args = json.RawMessage(tc.Function.Arguments)
			} else {
				args = json.RawMessage("{}")
			}
			parts = append(parts, Part{
				FunctionCall: &FunctionCall{
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		return Content{Role: role, Parts: parts}, nil
	}

	// Tool result (tool → user with function response)
	if m.Role == canonical.RoleTool && m.ToolCallID != "" {
		funcName := callIDToName[m.ToolCallID]
		if funcName == "" {
			funcName = m.ToolCallID
		}
		respJSON, _ := json.Marshal(map[string]string{"result": m.Content})
		return Content{
			Role: "user",
			Parts: []Part{{
				FunctionResponse: &FunctionResponse{
					Name:     funcName,
					Response: respJSON,
				},
			}},
		}, nil
	}

	// Multi-modal
	if m.IsMultiModal() {
		var parts []Part
		for _, p := range m.Parts {
			switch p.Type {
			case "text":
				parts = append(parts, Part{Text: p.Text})
			case "image_url":
				if p.ImageURL != nil {
					if strings.HasPrefix(p.ImageURL.URL, "data:") {
						// base64 data URI
						mtype, data, ok := parseDataURI(p.ImageURL.URL)
						if ok {
							parts = append(parts, Part{InlineData: &InlineData{MIMEType: mtype, Data: data}})
						}
					} else {
						parts = append(parts, Part{FileData: &FileData{FileURI: p.ImageURL.URL}})
					}
				}
			}
		}
		return Content{Role: role, Parts: parts}, nil
	}

	return Content{Role: role, Parts: []Part{{Text: m.Content}}}, nil
}

func canonicalRoleToGemini(role canonical.Role) string {
	switch role {
	case canonical.RoleAssistant:
		return "model"
	default:
		return "user"
	}
}

func reasoningEffortToBudget(effort string) int {
	switch effort {
	case "high":
		return 8192
	case "medium":
		return 4096
	case "low":
		return 1024
	default:
		return 4096
	}
}

// parseDataURI splits a data URI into (mimeType, base64Data, ok).
func parseDataURI(uri string) (string, string, bool) {
	// data:image/jpeg;base64,<data>
	rest := strings.TrimPrefix(uri, "data:")
	semi := strings.Index(rest, ";")
	if semi < 0 {
		return "", "", false
	}
	mtype := rest[:semi]
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return "", "", false
	}
	data := rest[comma+1:]
	return mtype, data, true
}

// ============================================================================
// Conversion: Gemini response → canonical
// ============================================================================

func GenerateContentResponseToCanonical(resp *GenerateContentResponse, requestID, model string) *canonical.ChatResponse {
	cr := &canonical.ChatResponse{
		ID:     requestID,
		Object: "chat.completion",
		Model:  model,
	}

	for _, cand := range resp.Candidates {
		choice := canonical.Choice{
			Index:        cand.Index,
			FinishReason: geminiFinishReasonToOpenAI(cand.FinishReason),
		}
		msg := canonical.Message{Role: canonical.RoleAssistant}
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				msg.Content += part.Text
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				msg.ToolCalls = append(msg.ToolCalls, canonical.ToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: canonical.FuncCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(args),
					},
				})
				choice.FinishReason = "tool_calls"
			}
		}
		choice.Message = msg
		cr.Choices = append(cr.Choices, choice)
	}

	if resp.UsageMetadata != nil {
		cr.Usage = &canonical.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
			CacheReadTokens:  resp.UsageMetadata.CachedContentTokenCount,
			ReasoningTokens:  resp.UsageMetadata.ThoughtsTokenCount,
		}
	}

	return cr
}

func geminiFinishReasonToOpenAI(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	case "RECITATION":
		return "content_filter"
	default:
		return strings.ToLower(reason)
	}
}

// StreamChunkToCanonical converts a Gemini SSE chunk to a canonical stream chunk.
func StreamChunkToCanonical(chunk *GenerateContentResponse, responseID, model string) *canonical.StreamChunk {
	cc := &canonical.StreamChunk{
		ID:     responseID,
		Object: "chat.completion.chunk",
		Model:  model,
	}
	for _, cand := range chunk.Candidates {
		choice := canonical.Choice{
			Index:        cand.Index,
			FinishReason: geminiFinishReasonToOpenAI(cand.FinishReason),
		}
		delta := canonical.Message{Role: canonical.RoleAssistant}
		for _, part := range cand.Content.Parts {
			delta.Content += part.Text
		}
		choice.Delta = delta
		cc.Choices = append(cc.Choices, choice)
	}
	if chunk.UsageMetadata != nil {
		cc.Usage = &canonical.Usage{
			PromptTokens:     chunk.UsageMetadata.PromptTokenCount,
			CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
			CacheReadTokens:  chunk.UsageMetadata.CachedContentTokenCount,
			ReasoningTokens:  chunk.UsageMetadata.ThoughtsTokenCount,
		}
	}
	return cc
}
