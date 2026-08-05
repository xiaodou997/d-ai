package workspace

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

type stubRepo struct {
	lastSessionLimit int
	lastImageLimit   int
}

type stubCatalog struct {
	items []ChatModel
}

type stubWriter struct {
	sessionID       string
	lastOwner       Owner
	lastCreateInput CreateChatSessionInput
	lastDeletedID   string
}

type stubRuntimeWriter struct {
	owner     Owner
	sessionID string
	message   ChatMessageWriteInput
	messageID string
}

func (s *stubRuntimeWriter) CreateChatMessage(_ context.Context, owner Owner, sessionID string, input ChatMessageWriteInput) (string, error) {
	s.owner = owner
	s.sessionID = sessionID
	s.message = input
	return s.messageID, nil
}

func (*stubRuntimeWriter) UpdateChatMessageContent(context.Context, Owner, string, string) error {
	return nil
}

func (*stubRuntimeWriter) UpdateChatMessageRoute(context.Context, Owner, string, ChatMessageRouteUpdate) error {
	return nil
}

func (*stubRuntimeWriter) UpdateChatSessionRoute(context.Context, Owner, string, surface.ID, string) error {
	return nil
}

func (s *stubRepo) ListChatSessions(_ context.Context, _ Owner, limit int) ([]ChatSession, error) {
	s.lastSessionLimit = limit
	return []ChatSession{}, nil
}

func (s *stubRepo) GetChatSession(_ context.Context, _ Owner, _ string) (ChatSession, error) {
	return ChatSession{ID: "session-1", ModelCode: "gpt-4.1-mini", Title: "新对话"}, nil
}

func (s *stubRepo) ListChatMessages(_ context.Context, _ Owner, _ string) ([]ChatMessage, error) {
	return []ChatMessage{}, nil
}

func (s *stubRepo) ListImageJobs(_ context.Context, _ Owner, limit int) ([]ImageJob, error) {
	s.lastImageLimit = limit
	return []ImageJob{}, nil
}

func (s *stubCatalog) ListChatModels(_ context.Context, _ Owner) ([]ChatModel, error) {
	return append([]ChatModel(nil), s.items...), nil
}

func (s *stubWriter) CreateChatSession(_ context.Context, owner Owner, input CreateChatSessionInput) (string, error) {
	s.lastOwner = owner
	s.lastCreateInput = input
	if s.sessionID == "" {
		return "session-1", nil
	}
	return s.sessionID, nil
}

func (s *stubWriter) DeleteChatSession(_ context.Context, _ Owner, sessionID string) error {
	s.lastDeletedID = sessionID
	return nil
}

func TestOverviewClampsRecentLimit(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo)
	owner := Owner{Scope: identity.ScopeUser, TenantID: "t1", UserID: "u1"}

	if _, err := svc.Overview(context.Background(), owner, 999); err != nil {
		t.Fatalf("overview error = %v", err)
	}
	if repo.lastSessionLimit != maxRecentLimit {
		t.Fatalf("session limit = %d, want %d", repo.lastSessionLimit, maxRecentLimit)
	}
	if repo.lastImageLimit != maxRecentLimit {
		t.Fatalf("image limit = %d, want %d", repo.lastImageLimit, maxRecentLimit)
	}
}

func TestOverviewRejectsInvalidOwner(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo)

	if _, err := svc.Overview(context.Background(), Owner{Scope: identity.ScopeUser, TenantID: "t1"}, 6); err == nil {
		t.Fatal("expected error for missing user id")
	}
}

func TestListChatModelsUsesCatalog(t *testing.T) {
	repo := &stubRepo{}
	catalog := &stubCatalog{
		items: []ChatModel{{GroupID: "group-1", ModelCode: "gpt-4.1-mini", DefaultProtocol: "openai_responses"}},
	}
	svc := NewService(repo, WithChatCatalog(catalog))

	items, err := svc.ListChatModels(context.Background(), Owner{Scope: identity.ScopeTenant, TenantID: "t1"})
	if err != nil {
		t.Fatalf("list chat models error = %v", err)
	}
	if len(items) != 1 || items[0].ModelCode != "gpt-4.1-mini" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestCreateChatSessionUsesWriter(t *testing.T) {
	repo := &stubRepo{}
	writer := &stubWriter{}
	catalog := &stubCatalog{
		items: []ChatModel{{GroupID: "group-1", ModelCode: "gpt-4.1-mini", DefaultProtocol: "openai_responses"}},
	}
	svc := NewService(repo, WithChatCatalog(catalog), WithChatWriter(writer))

	session, err := svc.CreateChatSession(context.Background(), Owner{Scope: identity.ScopeTenant, TenantID: "t1"}, CreateChatSessionInput{
		ModelCode: "gpt-4.1-mini",
		GroupID:   "group-1",
	})
	if err != nil {
		t.Fatalf("create chat session error = %v", err)
	}
	if writer.lastCreateInput.Title != "新对话" {
		t.Fatalf("title = %q, want 新对话", writer.lastCreateInput.Title)
	}
	if session.ID != "session-1" {
		t.Fatalf("session = %+v", session)
	}
}

func TestDeleteChatSessionUsesWriter(t *testing.T) {
	repo := &stubRepo{}
	writer := &stubWriter{}
	svc := NewService(repo, WithChatWriter(writer))

	if err := svc.DeleteChatSession(context.Background(), Owner{Scope: identity.ScopeTenant, TenantID: "t1"}, "session-1"); err != nil {
		t.Fatalf("delete chat session error = %v", err)
	}
	if writer.lastDeletedID != "session-1" {
		t.Fatalf("deleted session id = %q, want session-1", writer.lastDeletedID)
	}
}

func TestCreateChatMessageUsesRuntimeWriter(t *testing.T) {
	writer := &stubRuntimeWriter{messageID: "message-1"}
	svc := NewService(&stubRepo{}, WithChatRuntimeWriter(writer))
	owner := Owner{Scope: identity.ScopeUser, TenantID: "tenant-1", UserID: "user-1"}

	messageID, err := svc.CreateChatMessage(context.Background(), owner, "session-1", ChatMessageWriteInput{
		Role:          MessageRoleAssistant,
		Content:       "partial reply",
		ClientSurface: surface.OpenAIChat,
	})
	if err != nil {
		t.Fatalf("create chat message error = %v", err)
	}
	if messageID != "message-1" {
		t.Fatalf("message id = %q, want message-1", messageID)
	}
	if writer.owner != owner || writer.sessionID != "session-1" || writer.message.Content != "partial reply" {
		t.Fatalf("runtime writer input = %+v, owner = %+v, session = %q", writer.message, writer.owner, writer.sessionID)
	}
}
