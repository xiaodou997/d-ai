package workspace

import (
	"context"

	"xiaodou/dai/internal/ai/core/surface"
)

// OverviewReader returns the recent activity projection used by workspace
// landing pages.
type OverviewReader interface {
	Overview(ctx context.Context, owner Owner, limit int) (Overview, error)
}

// ChatModelReader exposes the chat models visible to a workspace owner.
type ChatModelReader interface {
	ListChatModels(ctx context.Context, owner Owner) ([]ChatModel, error)
}

// ChatSessionReader contains non-mutating session and message queries.
type ChatSessionReader interface {
	ListChatSessions(ctx context.Context, owner Owner, limit int) ([]ChatSession, error)
	GetChatSession(ctx context.Context, owner Owner, sessionID string) (ChatSession, error)
	ListChatMessages(ctx context.Context, owner Owner, sessionID string) ([]ChatMessage, error)
}

// ChatSessionManager owns user-driven session creation and deletion.
type ChatSessionManager interface {
	CreateChatSession(ctx context.Context, owner Owner, input CreateChatSessionInput) (ChatSession, error)
	DeleteChatSession(ctx context.Context, owner Owner, sessionID string) error
}

// ChatMessageManager persists runtime messages and their selected routes.
type ChatMessageManager interface {
	CreateChatMessage(ctx context.Context, owner Owner, sessionID string, input ChatMessageWriteInput) (string, error)
	UpdateChatMessageContent(ctx context.Context, owner Owner, messageID string, content string) error
	UpdateChatMessageRoute(ctx context.Context, owner Owner, messageID string, input ChatMessageRouteUpdate) error
	UpdateChatSessionRoute(ctx context.Context, owner Owner, sessionID string, clientSurface surface.ID, routeID string) error
}

// ImageJobReader returns image jobs owned by a workspace subject.
type ImageJobReader interface {
	ListImageJobs(ctx context.Context, owner Owner, limit int) ([]ImageJob, error)
}
