package workspace

import (
	"context"
	"fmt"
	"strings"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultRecentLimit = 6
	maxRecentLimit     = 50
)

type Service struct {
	repo          Repository
	catalog       ChatCatalog
	chatWriter    ChatWriter
	runtimeWriter ChatRuntimeWriter
}

type Option func(*Service)

func WithChatCatalog(catalog ChatCatalog) Option {
	return func(s *Service) {
		s.catalog = catalog
	}
}

func WithChatWriter(writer ChatWriter) Option {
	return func(s *Service) {
		s.chatWriter = writer
	}
}

func WithChatRuntimeWriter(writer ChatRuntimeWriter) Option {
	return func(s *Service) {
		s.runtimeWriter = writer
	}
}

func NewService(repo Repository, opts ...Option) *Service {
	svc := &Service{repo: repo}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

func (s *Service) ListChatSessions(ctx context.Context, owner Owner, limit int) ([]ChatSession, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	return s.repo.ListChatSessions(ctx, owner, clampRecentLimit(limit))
}

func (s *Service) GetChatSession(ctx context.Context, owner Owner, sessionID string) (ChatSession, error) {
	if err := validateOwner(owner); err != nil {
		return ChatSession{}, err
	}
	if sessionID == "" {
		return ChatSession{}, domain.NewValidationError("session_id", "session id is required")
	}
	return s.repo.GetChatSession(ctx, owner, sessionID)
}

func (s *Service) ListChatMessages(ctx context.Context, owner Owner, sessionID string) ([]ChatMessage, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, domain.NewValidationError("session_id", "session id is required")
	}
	return s.repo.ListChatMessages(ctx, owner, sessionID)
}

func (s *Service) ListImageJobs(ctx context.Context, owner Owner, limit int) ([]ImageJob, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	return s.repo.ListImageJobs(ctx, owner, clampRecentLimit(limit))
}

func (s *Service) ListChatModels(ctx context.Context, owner Owner) ([]ChatModel, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	if s.catalog == nil {
		return nil, fmt.Errorf("workspace chat catalog is not configured")
	}
	return s.catalog.ListChatModels(ctx, owner)
}

func (s *Service) CreateChatSession(ctx context.Context, owner Owner, input CreateChatSessionInput) (ChatSession, error) {
	if err := validateOwner(owner); err != nil {
		return ChatSession{}, err
	}
	if s.chatWriter == nil {
		return ChatSession{}, fmt.Errorf("workspace chat writer is not configured")
	}
	input.Title = normalizeWorkspaceSessionTitle(input.Title)
	input.ModelCode = strings.TrimSpace(input.ModelCode)
	input.GroupID = strings.TrimSpace(input.GroupID)
	if input.GroupID == "" {
		return ChatSession{}, domain.NewValidationError("group_id", "group_id is required")
	}
	if input.ModelCode == "" {
		return ChatSession{}, domain.NewValidationError("model_code", "model_code is required")
	}
	models, err := s.ListChatModels(ctx, owner)
	if err != nil {
		return ChatSession{}, err
	}
	if !workspaceChatModelVisible(models, input.GroupID, input.ModelCode) {
		return ChatSession{}, domain.ErrForbidden
	}

	sessionID, err := s.chatWriter.CreateChatSession(ctx, owner, input)
	if err != nil {
		return ChatSession{}, err
	}
	return s.repo.GetChatSession(ctx, owner, sessionID)
}

func (s *Service) DeleteChatSession(ctx context.Context, owner Owner, sessionID string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return domain.NewValidationError("session_id", "session id is required")
	}
	if s.chatWriter == nil {
		return fmt.Errorf("workspace chat writer is not configured")
	}
	return s.chatWriter.DeleteChatSession(ctx, owner, sessionID)
}

func (s *Service) CreateChatMessage(ctx context.Context, owner Owner, sessionID string, input ChatMessageWriteInput) (string, error) {
	if err := validateOwner(owner); err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", domain.NewValidationError("session_id", "session id is required")
	}
	if s.runtimeWriter == nil {
		return "", fmt.Errorf("workspace chat runtime writer is not configured")
	}
	return s.runtimeWriter.CreateChatMessage(ctx, owner, sessionID, input)
}

func (s *Service) UpdateChatMessageContent(ctx context.Context, owner Owner, messageID string, content string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	if strings.TrimSpace(messageID) == "" {
		return domain.NewValidationError("message_id", "message id is required")
	}
	if s.runtimeWriter == nil {
		return fmt.Errorf("workspace chat runtime writer is not configured")
	}
	return s.runtimeWriter.UpdateChatMessageContent(ctx, owner, messageID, content)
}

func (s *Service) UpdateChatMessageRoute(ctx context.Context, owner Owner, messageID string, input ChatMessageRouteUpdate) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	if strings.TrimSpace(messageID) == "" {
		return domain.NewValidationError("message_id", "message id is required")
	}
	if s.runtimeWriter == nil {
		return fmt.Errorf("workspace chat runtime writer is not configured")
	}
	return s.runtimeWriter.UpdateChatMessageRoute(ctx, owner, messageID, input)
}

func (s *Service) UpdateChatSessionRoute(ctx context.Context, owner Owner, sessionID string, clientSurface surface.ID, routeID string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return domain.NewValidationError("session_id", "session id is required")
	}
	if s.runtimeWriter == nil {
		return fmt.Errorf("workspace chat runtime writer is not configured")
	}
	return s.runtimeWriter.UpdateChatSessionRoute(ctx, owner, sessionID, clientSurface, routeID)
}

func (s *Service) Overview(ctx context.Context, owner Owner, limit int) (Overview, error) {
	if err := validateOwner(owner); err != nil {
		return Overview{}, err
	}
	limit = clampRecentLimit(limit)
	chatSessions, err := s.repo.ListChatSessions(ctx, owner, limit)
	if err != nil {
		return Overview{}, err
	}
	imageJobs, err := s.repo.ListImageJobs(ctx, owner, limit)
	if err != nil {
		return Overview{}, err
	}
	return Overview{
		RecentChatSessions: chatSessions,
		RecentImageJobs:    imageJobs,
	}, nil
}

func clampRecentLimit(limit int) int {
	if limit <= 0 {
		return defaultRecentLimit
	}
	if limit > maxRecentLimit {
		return maxRecentLimit
	}
	return limit
}

func validateOwner(owner Owner) error {
	if owner.TenantID == "" {
		return domain.NewValidationError("tenant_id", "tenant id is required")
	}
	switch owner.Scope {
	case identity.ScopeTenant:
		return nil
	case identity.ScopeUser:
		if owner.UserID == "" {
			return domain.NewValidationError("user_id", "user id is required for user scope")
		}
		return nil
	default:
		return domain.NewValidationError("scope", "workspace only supports tenant or user scope")
	}
}

func normalizeWorkspaceSessionTitle(value string) string {
	if title := strings.TrimSpace(value); title != "" {
		return title
	}
	return "新对话"
}

func workspaceChatModelVisible(models []ChatModel, groupID string, modelCode string) bool {
	for _, model := range models {
		if model.GroupID == groupID && model.ModelCode == modelCode {
			return true
		}
	}
	return false
}
