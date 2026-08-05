package bridge

// Usage is the canonical bridge-level token usage projection.
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

// StreamEvent marks a bridge-neutral streaming event kind.
type StreamEvent int

const (
	EvStart StreamEvent = iota
	EvTextDelta
	EvReasoningDelta
	EvReasoningSignature
	EvToolCallStart
	EvToolCallArgsDelta
	EvToolResultDelta
	EvFinish
	EvUnknown
)

// StreamFrame is the bridge-neutral streaming frame used during client/provider
// SSE conversion.
type StreamFrame struct {
	ID    string
	Model string
	Event StreamEvent

	Text string

	ToolIndex int
	CallID    string
	Name      string
	Arguments string

	ToolUseID string
	Content   string

	FinishReason string
	HasFinish    bool
	Usage        *Usage

	Unknown []byte
}

// StreamProvider parses upstream SSE lines into bridge-neutral frames.
type StreamProvider interface {
	PushLine(line []byte) ([]StreamFrame, error)
	Finish() ([]StreamFrame, error)
}

// StreamEmitter emits bridge-neutral frames into client SSE bytes.
type StreamEmitter interface {
	Emit(frame StreamFrame) ([]byte, error)
	Finish() ([]byte, error)
}

// Runtime defines the runtime-kernel bridge entrypoint used by execution.
type Runtime interface {
	ConvertRequest(env RequestEnvelope, body []byte) ([]byte, error)
	ConvertResponse(env ResponseEnvelope, body []byte) ([]byte, error)
	NewStreamProvider(env ResponseEnvelope, fallbackModel string) (StreamProvider, error)
	NewStreamEmitter(env ResponseEnvelope) (StreamEmitter, error)
}
