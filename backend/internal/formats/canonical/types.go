// Package canonical defines the gateway's internal representation of AI requests
// and responses. All format-specific structs convert to/from these types.
// The canonical format is intentionally close to OpenAI's Chat format since
// that is what clients send most often.
package canonical

import "encoding/json"

// ============================================================================
// Shared message building blocks
// ============================================================================

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart represents one element in a multi-modal message.
type ContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *ImageURL       `json:"image_url,omitempty"`
	// Tool result fields
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Raw JSON for provider-specific extensions
	Raw json.RawMessage `json:"-"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto" | "low" | "high"
}

// Message is a single turn in the conversation.
type Message struct {
	Role       Role          `json:"role"`
	Content    string        `json:"content,omitempty"`
	Parts      []ContentPart `json:"parts,omitempty"` // non-nil when multi-modal
	Name       string        `json:"name,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"` // for role=tool
}

// IsMultiModal reports whether this message has content parts (images, etc.)
func (m Message) IsMultiModal() bool { return len(m.Parts) > 0 }

// TextContent returns the text content regardless of whether it's a string or parts.
func (m Message) TextContent() string {
	if m.Content != "" {
		return m.Content
	}
	for _, p := range m.Parts {
		if p.Type == "text" {
			return p.Text
		}
	}
	return ""
}

// ============================================================================
// Tool / function calling
// ============================================================================

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // "function"
	Function FuncCall `json:"function"`
	Index    int      `json:"index,omitempty"`
}

type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type Tool struct {
	Type     string   `json:"type"` // "function"
	Function FuncSpec `json:"function"`
}

type FuncSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
	Strict      bool            `json:"strict,omitempty"`
}

// ============================================================================
// Chat request (the main gateway request type)
// ============================================================================

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	N           *int      `json:"n,omitempty"`
	Stream      bool      `json:"stream"`
	Stop        []string  `json:"stop,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
	// Reasoning / extended thinking
	ReasoningEffort *string `json:"reasoning_effort,omitempty"` // "low"|"medium"|"high"
	ThinkingBudget  *int    `json:"thinking_budget,omitempty"`  // max thinking tokens
	// Pass-through for provider-specific parameters
	Extra map[string]json.RawMessage `json:"-"`
}

// StreamOptions carries stream-specific config attached to the request.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ============================================================================
// Chat response
// ============================================================================

type ChatResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // "chat.completion"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Delta        Message `json:"delta,omitempty"`
	FinishReason string  `json:"finish_reason"`
	Logprobs     any     `json:"logprobs,omitempty"`
}

type Usage struct {
	PromptTokens            int            `json:"prompt_tokens"`
	CompletionTokens        int            `json:"completion_tokens"`
	TotalTokens             int            `json:"total_tokens"`
	CacheWriteTokens        int            `json:"cache_write_tokens,omitempty"`
	CacheReadTokens         int            `json:"cache_read_tokens,omitempty"`
	ReasoningTokens         int            `json:"reasoning_tokens,omitempty"`
	CompletionTokensDetails map[string]int `json:"completion_tokens_details,omitempty"`
	PromptTokensDetails     map[string]int `json:"prompt_tokens_details,omitempty"`
}

// ============================================================================
// Stream chunk (SSE frame body)
// ============================================================================

type StreamChunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // "chat.completion.chunk"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// ============================================================================
// Embedding request / response
// ============================================================================

type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"` // normalised: always a slice
	EncodingFormat string   `json:"encoding_format,omitempty"` // "float" | "base64"
	Dimensions     *int     `json:"dimensions,omitempty"`
}

type EmbeddingResponse struct {
	Object string      `json:"object"` // "list"
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  EmbedUsage  `json:"usage"`
}

type Embedding struct {
	Object    string    `json:"object"` // "embedding"
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type EmbedUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ============================================================================
// Image generation request / response
// ============================================================================

type ImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              *int   `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // "url" | "b64_json"
	User           string `json:"user,omitempty"`
}

type ImageResponse struct {
	Created int64         `json:"created"`
	Data    []ImageObject `json:"data"`
}

type ImageObject struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
	Revised string `json:"revised_prompt,omitempty"`
}
