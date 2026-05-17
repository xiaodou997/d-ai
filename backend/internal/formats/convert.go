// Package formats provides the format conversion layer for the AI gateway.
// It converts between the gateway's canonical internal format and the wire
// formats expected by each upstream provider protocol.
//
// Conversion flow:
//
//	Client request (any format)
//	  → ParseClientChatRequest()   → canonical.ChatRequest
//	  → ToUpstreamChatRequest()    → provider-specific bytes
//	  → upstream response bytes
//	  → ToCanonicalChatResponse()  → canonical.ChatResponse
//	  → client response (OpenAI format)
package formats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats/canonical"
	"uni-ai-api/backend/internal/formats/claude"
	"uni-ai-api/backend/internal/formats/gemini"
	"uni-ai-api/backend/internal/formats/openai"
	"uni-ai-api/backend/internal/formats/openai/responses"
)

// DetectClientProtocol returns the upstream protocol that matches the client's
// Content-Type and request path. Clients almost always send OpenAI format.
func DetectClientProtocol(r *http.Request) domain.UpstreamProtocol {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/messages"):
			return domain.ProtocolAnthropicMessages
		case strings.Contains(path, "/responses"):
			return domain.ProtocolOpenAIResponses
		case strings.Contains(path, "/embeddings"):
			return domain.ProtocolOpenAIEmbeddings
		case strings.Contains(path, "/images/generations"):
			return domain.ProtocolOpenAIImages
		default:
			return domain.ProtocolOpenAIChat
		}
	}
	return domain.ProtocolOpenAIChat
}

// ============================================================================
// Chat request conversion
// ============================================================================

// ParseClientChatRequest decodes the raw HTTP body into a canonical ChatRequest.
// The clientProtocol is detected from the incoming request path/headers.
func ParseClientChatRequest(body []byte, clientProtocol domain.UpstreamProtocol) (*canonical.ChatRequest, error) {
	switch clientProtocol {
	case domain.ProtocolAnthropicMessages:
		var req claude.MessagesRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode claude request: %w", err)
		}
		return claudeRequestToCanonical(&req)

	default: // ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolOpenAICompletions
		var req openai.ChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode openai request: %w", err)
		}
		return openai.ChatRequestToCanonical(&req)
	}
}

// ToUpstreamChatRequest converts a canonical request to the wire format
// expected by the target upstream protocol. Returns the JSON body bytes.
func ToUpstreamChatRequest(req *canonical.ChatRequest, protocol domain.UpstreamProtocol, upstreamModel string) ([]byte, error) {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		up, err := claude.MessagesRequestFromCanonical(req, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("build claude request: %w", err)
		}
		return json.Marshal(up)

	case domain.ProtocolGeminiGenerate:
		up, err := gemini.GenerateContentRequestFromCanonical(req, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("build gemini request: %w", err)
		}
		return json.Marshal(up)

	default: // openai_chat, openai_responses, openai_completions
		up, err := openai.ChatRequestFromCanonical(req, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("build openai request: %w", err)
		}
		return json.Marshal(up)
	}
}

// ============================================================================
// Chat response conversion
// ============================================================================

// ToCanonicalChatResponse converts an upstream response body to canonical format.
// The requestID is used to populate the canonical response ID for protocols that
// don't return one (e.g. Gemini).
func ToCanonicalChatResponse(body []byte, protocol domain.UpstreamProtocol, requestID, model string) (*canonical.ChatResponse, error) {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		var resp claude.MessagesResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode claude response: %w", err)
		}
		return claude.MessagesResponseToCanonical(&resp), nil

	case domain.ProtocolGeminiGenerate:
		var resp gemini.GenerateContentResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode gemini response: %w", err)
		}
		return gemini.GenerateContentResponseToCanonical(&resp, requestID, model), nil

	case domain.ProtocolOpenAIResponses:
		var resp responses.Response
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode responses api response: %w", err)
		}
		return resp.ToCanonical(), nil

	default: // openai_chat, openai_completions, openai_embeddings
		var resp openai.ChatResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode openai response: %w", err)
		}
		return openai.ChatResponseToCanonical(&resp), nil
	}
}

// CanonicalResponseToOpenAI serialises a canonical response back to the OpenAI
// wire format, which is what the gateway always returns to clients.
func CanonicalResponseToOpenAI(resp *canonical.ChatResponse) ([]byte, error) {
	out := openai.ChatResponseFromCanonical(resp)
	return json.Marshal(out)
}

// ============================================================================
// Streaming frame conversion
// ============================================================================

// ToCanonicalStreamChunk converts a raw SSE data line from an upstream into a
// canonical StreamChunk. Returns (nil, nil) for lines that carry no content
// (e.g. Anthropic's "ping" event).
func ToCanonicalStreamChunk(
	data []byte,
	eventType string, // Anthropic SSE event type field
	protocol domain.UpstreamProtocol,
	responseID, model string,
	index int,
) (*canonical.StreamChunk, error) {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		var event claude.StreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("decode claude stream event: %w", err)
		}
		if eventType != "" {
			event.Type = eventType
		}
		chunk := claude.StreamEventToCanonical(&event, responseID, model, index)
		return chunk, nil // nil is valid (no-op events)

	case domain.ProtocolGeminiGenerate:
		var chunk gemini.GenerateContentResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil, fmt.Errorf("decode gemini stream chunk: %w", err)
		}
		return gemini.StreamChunkToCanonical(&chunk, responseID, model), nil

	case domain.ProtocolOpenAIResponses:
		chunk := responses.StreamEventToCanonical(data, responseID, model, index)
		return chunk, nil // nil = no-op event

	default: // openai_chat, openai_completions
		var chunk openai.StreamChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil, fmt.Errorf("decode openai stream chunk: %w", err)
		}
		return openai.StreamChunkToCanonical(&chunk), nil
	}
}

// CanonicalChunkToOpenAI serialises a canonical StreamChunk back to the OpenAI
// SSE wire format ("data: {...}\n\n").
func CanonicalChunkToOpenAI(chunk *canonical.StreamChunk) ([]byte, error) {
	out := openai.StreamChunkFromCanonical(chunk)
	return json.Marshal(out)
}

// ============================================================================
// Internal helpers
// ============================================================================

// claudeRequestToCanonical converts a decoded Claude MessagesRequest to canonical.
// This handles the case where a client sends native Claude format to our gateway.
func claudeRequestToCanonical(req *claude.MessagesRequest) (*canonical.ChatRequest, error) {
	cr := &canonical.ChatRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
	}
	if req.MaxTokens > 0 {
		cr.MaxTokens = &req.MaxTokens
	}
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		cr.ThinkingBudget = &req.Thinking.BudgetTokens
	}

	// System message → canonical system message
	if req.System != "" {
		cr.Messages = append(cr.Messages, canonical.Message{
			Role:    canonical.RoleSystem,
			Content: req.System,
		})
	}

	for _, m := range req.Messages {
		cm, err := claudeMessageToCanonical(m)
		if err != nil {
			return nil, err
		}
		cr.Messages = append(cr.Messages, cm)
	}

	for _, t := range req.Tools {
		cr.Tools = append(cr.Tools, canonical.Tool{
			Type: "function",
			Function: canonical.FuncSpec{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return cr, nil
}

func claudeMessageToCanonical(m claude.Message) (canonical.Message, error) {
	role := canonical.Role(m.Role)
	if role == "model" {
		role = canonical.RoleAssistant
	}

	cm := canonical.Message{Role: role}

	// Try plain text first
	var text string
	if json.Unmarshal(m.Content, &text) == nil {
		cm.Content = text
		return cm, nil
	}

	// Array of blocks
	var blocks []claude.Block
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return cm, fmt.Errorf("decode claude message content: %w", err)
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			cm.Content += b.Text
		case "image":
			if b.Source != nil {
				url := b.Source.URL
				if url == "" && b.Source.Data != "" {
					url = fmt.Sprintf("data:%s;base64,%s", b.Source.MediaType, b.Source.Data)
				}
				cm.Parts = append(cm.Parts, canonical.ContentPart{
					Type:     "image_url",
					ImageURL: &canonical.ImageURL{URL: url},
				})
			}
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			cm.ToolCalls = append(cm.ToolCalls, canonical.ToolCall{
				ID:   b.ID,
				Type: "function",
				Function: canonical.FuncCall{
					Name:      b.Name,
					Arguments: string(args),
				},
			})
		case "tool_result":
			cm.Role = canonical.RoleTool
			cm.ToolCallID = b.ToolUseID
			var resultText string
			_ = json.Unmarshal(b.Content, &resultText)
			cm.Content = resultText
		}
	}

	return cm, nil
}
