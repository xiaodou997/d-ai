package upstreamcompat

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"xiaodou/dai/internal/ai/domain"
)

// DefaultInstructions belongs to requests authored by D-AI (web and tests).
// It is not injected into external API clients' requests.
//
//go:embed prompts/assistant.txt
var DefaultInstructions string

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildChatRequest is the common protocol contract for the console and account
// diagnostics. A zero maxTokens leaves optional output budgets unset.
func BuildChatRequest(protocol domain.UpstreamProtocol, model string, messages []Message, stream bool, maxTokens int) ([]byte, error) {
	body := map[string]any{"model": model}
	var systems []string
	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			if strings.TrimSpace(msg.Content) != "" {
				systems = append(systems, msg.Content)
			}
		}
	}
	system := strings.Join(systems, "\n\n")
	if system == "" {
		system = DefaultInstructions
	}
	switch protocol {
	case domain.ProtocolOpenAIResponses:
		input := make([]any, 0, len(messages))
		for _, msg := range messages {
			if msg.Role == "system" || msg.Role == "developer" {
				continue
			}
			role, kind := "user", "input_text"
			if msg.Role == "assistant" {
				role, kind = "assistant", "output_text"
			}
			input = append(input, map[string]any{"role": role, "content": []any{map[string]any{"type": kind, "text": msg.Content}}})
		}
		body["instructions"], body["input"], body["store"], body["stream"] = system, input, false, stream
		if maxTokens > 0 {
			body["max_output_tokens"] = maxTokens
		}
	case domain.ProtocolOpenAIChat, domain.ProtocolAnthropicMessages:
		out := make([]Message, 0, len(messages)+1)
		if protocol == domain.ProtocolOpenAIChat {
			out = append(out, Message{Role: "system", Content: system})
		} else {
			body["system"] = system
		}
		for _, msg := range messages {
			if msg.Role == "system" || msg.Role == "developer" {
				continue
			}
			role := "user"
			if msg.Role == "assistant" {
				role = "assistant"
			}
			out = append(out, Message{Role: role, Content: msg.Content})
		}
		body["messages"], body["stream"] = out, stream
		if maxTokens > 0 {
			body["max_tokens"] = maxTokens
		} else if protocol == domain.ProtocolAnthropicMessages {
			body["max_tokens"] = 2048
		}
	case domain.ProtocolGeminiGenerate:
		delete(body, "model")
		contents := make([]any, 0, len(messages))
		for _, msg := range messages {
			if msg.Role == "system" || msg.Role == "developer" {
				continue
			}
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			contents = append(contents, map[string]any{"role": role, "parts": []any{map[string]any{"text": msg.Content}}})
		}
		body["contents"] = contents
		body["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": system}}}
		if maxTokens > 0 {
			body["generationConfig"] = map[string]any{"maxOutputTokens": maxTokens}
		}
	default:
		return nil, fmt.Errorf("unsupported chat protocol %q", protocol)
	}
	return json.Marshal(body)
}

type ImageOptions struct {
	N                 int
	Stream            bool
	Size              string
	ResponseFormat    string
	Background        string
	OutputFormat      string
	Moderation        string
	User              string
	OutputCompression *int
}

// BuildImageRequest keeps image options in their own protocol; chat instructions
// must never be appended to an image prompt or sent as unknown image fields.
func BuildImageRequest(protocol domain.UpstreamProtocol, model, prompt string, opts ImageOptions) ([]byte, error) {
	switch protocol {
	case domain.ProtocolOpenAIImages:
		n := opts.N
		if n <= 0 {
			n = domain.DefaultImageOutputCount
		}
		body := map[string]any{"model": model, "prompt": prompt, "n": n, "stream": opts.Stream}
		for key, value := range map[string]string{"size": opts.Size, "response_format": opts.ResponseFormat, "background": opts.Background, "output_format": opts.OutputFormat, "moderation": opts.Moderation, "user": opts.User} {
			if value != "" {
				body[key] = value
			}
		}
		if opts.OutputCompression != nil {
			body["output_compression"] = *opts.OutputCompression
		}
		return json.Marshal(body)
	case domain.ProtocolGeminiGenerate:
		config := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
		if opts.Size != "" {
			config["imageConfig"] = map[string]any{"imageSize": opts.Size}
		}
		return json.Marshal(map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt}}}}, "generationConfig": config})
	default:
		return nil, fmt.Errorf("unsupported image protocol %q", protocol)
	}
}
