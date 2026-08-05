package workspace

import (
	"context"

	"xiaodou/dai/internal/ai/core/surface"
)

type ChatCatalog interface {
	ListChatModels(ctx context.Context, owner Owner) ([]ChatModel, error)
}

type ChatWriter interface {
	CreateChatSession(ctx context.Context, owner Owner, input CreateChatSessionInput) (string, error)
	DeleteChatSession(ctx context.Context, owner Owner, sessionID string) error
}

type ChatRuntimeWriter interface {
	CreateChatMessage(ctx context.Context, owner Owner, sessionID string, input ChatMessageWriteInput) (string, error)
	UpdateChatMessageContent(ctx context.Context, owner Owner, messageID string, content string) error
	UpdateChatMessageRoute(ctx context.Context, owner Owner, messageID string, input ChatMessageRouteUpdate) error
	UpdateChatSessionRoute(ctx context.Context, owner Owner, sessionID string, clientSurface surface.ID, routeID string) error
}

type Repository interface {
	ListChatSessions(ctx context.Context, owner Owner, limit int) ([]ChatSession, error)
	GetChatSession(ctx context.Context, owner Owner, sessionID string) (ChatSession, error)
	ListChatMessages(ctx context.Context, owner Owner, sessionID string) ([]ChatMessage, error)
	ListImageJobs(ctx context.Context, owner Owner, limit int) ([]ImageJob, error)
}
