package workspace

import (
	"time"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

type ThreadStatus string

const (
	ThreadStatusActive   ThreadStatus = "active"
	ThreadStatusArchived ThreadStatus = "archived"
	ThreadStatusDeleted  ThreadStatus = "deleted"
)

// Thread represents one workspace conversation or image task thread.
type Thread struct {
	ID              string
	OwnerScope      identity.Scope
	TenantID        string
	UserID          string
	TargetModelCode string
	Title           string
	SelectedSurface surface.ID
	Status          ThreadStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

// Message captures one workspace event after bridge/runtime processing.
type Message struct {
	ID              string
	ThreadID        string
	Role            MessageRole
	ContentText     string
	ContentJSON     map[string]any
	ClientSurface   surface.ID
	UpstreamSurface surface.ID
	RouteSnapshot   map[string]any
	Usage           map[string]any
	Error           map[string]any
	CreatedAt       time.Time
}

type ChatMessageWriteInput struct {
	Role          MessageRole
	Content       string
	ClientSurface surface.ID
	RouteID       string
	StreamStatus  ChatStreamStatus
}

type ChatMessageRouteUpdate struct {
	ClientSurface surface.ID
	RouteID       string
	StreamStatus  ChatStreamStatus
}

type ChatStreamStatus string

const (
	ChatStreamStatusStreaming   ChatStreamStatus = "streaming"
	ChatStreamStatusCompleted   ChatStreamStatus = "completed"
	ChatStreamStatusInterrupted ChatStreamStatus = "interrupted"
)
