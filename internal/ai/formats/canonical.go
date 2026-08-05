package formats

import (
	"encoding/json"
	"sort"
)

// ============================================================================
// Canonical IR (中间表示) — 跨协议转换的中枢
// ============================================================================
//
// 设计抄 Aether `aether-ai-formats/src/protocol/canonical.rs`：每个 wire 格式只
// 实现 parse(wire→canonical) 与 emit(canonical→wire) 两个方向，任意两格式的转换
// = parse(源) + emit(目标)。这样 N 个格式只需 2N 套适配，而非 N×N 两两转换器。
//
// 仅覆盖「生成类」协议（openai:chat / openai:responses / claude:messages /
// gemini:generate）。embeddings / images / rerank 不进 IR——它们只做同格式
// passthrough，不跨格式转换（见 matrix.go）。
//
// 注意：Go 无 Rust 的代数枚举，ContentBlock 用「带 Kind 标签的宽结构体」表达
// 联合类型；未用到的字段留零值。Extensions 保留各源格式的厂商私有字段，按命名空间
// 归档，emit 时只回吐目标格式对应命名空间的字段（跨格式时多被丢弃，符合预期）。

// Role 是规范化后的消息角色。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleTool      Role = "tool"
	RoleUnknown   Role = "unknown"
)

// StopReason 是规范化后的停止原因。
type StopReason string

const (
	StopEndTurn         StopReason = "end_turn"
	StopMaxTokens       StopReason = "max_tokens"
	StopStopSequence    StopReason = "stop_sequence"
	StopToolUse         StopReason = "tool_use"
	StopPauseTurn       StopReason = "pause_turn"
	StopRefusal         StopReason = "refusal"
	StopContentFiltered StopReason = "content_filtered"
	StopUnknown         StopReason = "unknown"
)

// BlockKind 标记 ContentBlock 的联合类型分支。
type BlockKind string

const (
	BlockText       BlockKind = "text"
	BlockThinking   BlockKind = "thinking"
	BlockImage      BlockKind = "image"
	BlockFile       BlockKind = "file"
	BlockAudio      BlockKind = "audio"
	BlockToolUse    BlockKind = "tool_use"
	BlockToolResult BlockKind = "tool_result"
	BlockUnknown    BlockKind = "unknown"
)

// Extensions 按命名空间（"openai"/"claude"/"gemini"/"aether"/"openai_responses"）
// 归档各源格式未被规范化的厂商私有字段。命名空间 → 字段名 → 原始 JSON。
type Extensions map[string]map[string]json.RawMessage

// get 返回某命名空间下的字段集（可能为 nil）。
func (e Extensions) get(ns string) map[string]json.RawMessage {
	if e == nil {
		return nil
	}
	return e[ns]
}

// ensure 返回某命名空间下可写的字段集，必要时创建。
func (e *Extensions) ensure(ns string) map[string]json.RawMessage {
	if *e == nil {
		*e = Extensions{}
	}
	bucket := (*e)[ns]
	if bucket == nil {
		bucket = map[string]json.RawMessage{}
		(*e)[ns] = bucket
	}
	return bucket
}

// setField 在命名空间下写入一个字段。
func (e *Extensions) setField(ns, key string, value json.RawMessage) {
	e.ensure(ns)[key] = value
}

// ContentBlock 是消息内容的一个块（联合类型，按 Kind 区分分支）。
type ContentBlock struct {
	Kind BlockKind

	// text / thinking
	Text string
	// thinking
	Signature        string
	EncryptedContent string
	// image / file / audio
	Data      string
	URL       string
	MediaType string
	Detail    string
	FileID    string
	FileURL   string
	Filename  string
	Format    string
	// tool_use
	ID    string
	Name  string
	Input json.RawMessage
	// tool_result
	ToolUseID      string
	Output         json.RawMessage
	HasOutput      bool
	ContentText    string
	HasContentText bool
	IsError        bool
	// unknown
	RawType string
	Payload json.RawMessage

	Extensions Extensions
}

// Instruction 是一条系统/开发者级指令（来自 system 字段或 system/developer 消息）。
type Instruction struct {
	Role       Role
	Text       string
	Extensions Extensions
}

// Message 是一条对话消息。
type Message struct {
	Role       Role
	Content    []ContentBlock
	Extensions Extensions
}

// GenerationConfig 是采样/生成参数。指针字段区分「未设置」与「零值」。
type GenerationConfig struct {
	MaxTokens        *uint64
	Temperature      *float64
	TopP             *float64
	TopK             *uint64
	StopSequences    *[]string
	N                *uint64
	PresencePenalty  *float64
	FrequencyPenalty *float64
	Seed             *int64
	Logprobs         *bool
	TopLogprobs      *uint64
}

// ToolDefinition 是一个工具/函数定义。
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Extensions  Extensions
}

// ToolChoiceKind 标记 ToolChoice 分支。
type ToolChoiceKind string

const (
	ToolChoiceAuto     ToolChoiceKind = "auto"
	ToolChoiceNone     ToolChoiceKind = "none"
	ToolChoiceRequired ToolChoiceKind = "required"
	ToolChoiceTool     ToolChoiceKind = "tool"
)

// ToolChoice 表达工具选择策略；Tool 分支带具体工具名。
type ToolChoice struct {
	Kind ToolChoiceKind
	Name string
}

// ThinkingConfig 是推理/思考开关与预算。
type ThinkingConfig struct {
	Enabled      bool
	BudgetTokens *uint64
	Extensions   Extensions
}

// ResponseFormat 表达结构化输出格式。
type ResponseFormat struct {
	FormatType string
	JSONSchema json.RawMessage
	Extensions Extensions
}

// Usage 是规范化后的 token 用量。
type Usage struct {
	InputTokens                 uint64
	OutputTokens                uint64
	TotalTokens                 uint64
	CacheReadTokens             uint64
	CacheWriteTokens            uint64
	CacheCreationEphemeral5mTok uint64
	CacheCreationEphemeral1hTok uint64
	ReasoningTokens             uint64
}

// Request 是规范化后的生成请求。
type Request struct {
	Model            string
	Instructions     []Instruction
	System           string
	HasSystem        bool
	Messages         []Message
	Generation       GenerationConfig
	Tools            []ToolDefinition
	ToolChoice       *ToolChoice
	Thinking         *ThinkingConfig
	ResponseFormat   *ResponseFormat
	ParallelToolCall *bool
	Metadata         json.RawMessage
	Extensions       Extensions
}

// ResponseOutput 是响应中的一个候选输出（OpenAI choices / Gemini candidates）。
type ResponseOutput struct {
	Index      int
	Role       Role
	Content    []ContentBlock
	StopReason *StopReason
	Extensions Extensions
}

// Response 是规范化后的生成响应。Content/StopReason 镜像首个 output，便于单候选取用。
type Response struct {
	ID         string
	Model      string
	Outputs    []ResponseOutput
	Content    []ContentBlock
	StopReason *StopReason
	Usage      *Usage
	Extensions Extensions
}

// ============================================================================
// 通用 helper
// ============================================================================

// roleFromString 把任意 wire 角色串规范化为 Role（OpenAI/Claude 通用）。
func roleFromString(role string) Role {
	switch normLower(role) {
	case "user":
		return RoleUser
	case "assistant":
		return RoleAssistant
	case "system":
		return RoleSystem
	case "developer":
		return RoleDeveloper
	case "tool":
		return RoleTool
	default:
		return RoleUnknown
	}
}

// collectExtensions 把对象里不在 skip 集合中的字段归档到 ns 命名空间。
func collectExtensions(obj map[string]json.RawMessage, ns string, skip ...string) Extensions {
	skipSet := make(map[string]struct{}, len(skip))
	for _, k := range skip {
		skipSet[k] = struct{}{}
	}
	var bucket map[string]json.RawMessage
	for k, v := range obj {
		if _, ok := skipSet[k]; ok {
			continue
		}
		if bucket == nil {
			bucket = map[string]json.RawMessage{}
		}
		bucket[k] = v
	}
	if bucket == nil {
		return nil
	}
	return Extensions{ns: bucket}
}

// emitExtensions 把 ext 中 ns 命名空间下、尚未出现在 out 里的字段写回 out。
// 对应 Aether 的 namespace_extension_object：跨格式 emit 时只回吐目标格式命名空间。
func emitExtensions(out map[string]any, ext Extensions, ns string) {
	bucket := ext.get(ns)
	if bucket == nil {
		return
	}
	keys := make([]string, 0, len(bucket))
	for k := range bucket {
		keys = append(keys, k)
	}
	sort.Strings(keys) // 确定性输出，便于 golden 测试
	for _, k := range keys {
		if _, exists := out[k]; exists {
			continue
		}
		out[k] = bucket[k]
	}
}
