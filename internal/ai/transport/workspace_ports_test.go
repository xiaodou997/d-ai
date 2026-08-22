package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/internal/auth"
)

var (
	_ workspace.OverviewReader     = (*workspace.Service)(nil)
	_ workspace.ChatModelReader    = (*workspace.Service)(nil)
	_ workspace.ChatSessionReader  = (*workspace.Service)(nil)
	_ workspace.ChatSessionManager = (*workspace.Service)(nil)
	_ workspace.ChatMessageManager = (*workspace.Service)(nil)
	_ workspace.ImageJobReader     = (*workspace.Service)(nil)
)

type workspaceOverviewStub struct {
	owner workspace.Owner
	limit int
	value workspace.Overview
}

func (s *workspaceOverviewStub) Overview(_ context.Context, owner workspace.Owner, limit int) (workspace.Overview, error) {
	s.owner, s.limit = owner, limit
	return s.value, nil
}

type workspaceModelReaderStub struct {
	owner  workspace.Owner
	models []workspace.ChatModel
}

func (s *workspaceModelReaderStub) ListChatModels(_ context.Context, owner workspace.Owner) ([]workspace.ChatModel, error) {
	s.owner = owner
	return s.models, nil
}

type workspaceSessionReaderStub struct {
	listOwner workspace.Owner
	listLimit int
	getOwner  workspace.Owner
	getID     string
	messageID string
	sessions  []workspace.ChatSession
	session   workspace.ChatSession
	messages  []workspace.ChatMessage
}

func (s *workspaceSessionReaderStub) ListChatSessions(_ context.Context, owner workspace.Owner, limit int) ([]workspace.ChatSession, error) {
	s.listOwner, s.listLimit = owner, limit
	return s.sessions, nil
}

func (s *workspaceSessionReaderStub) GetChatSession(_ context.Context, owner workspace.Owner, sessionID string) (workspace.ChatSession, error) {
	s.getOwner, s.getID = owner, sessionID
	return s.session, nil
}

func (s *workspaceSessionReaderStub) ListChatMessages(_ context.Context, _ workspace.Owner, sessionID string) ([]workspace.ChatMessage, error) {
	s.messageID = sessionID
	return s.messages, nil
}

type workspaceSessionManagerStub struct {
	createOwner workspace.Owner
	createInput workspace.CreateChatSessionInput
	deleteOwner workspace.Owner
	deleteID    string
	session     workspace.ChatSession
}

func (s *workspaceSessionManagerStub) CreateChatSession(_ context.Context, owner workspace.Owner, input workspace.CreateChatSessionInput) (workspace.ChatSession, error) {
	s.createOwner, s.createInput = owner, input
	return s.session, nil
}

func (s *workspaceSessionManagerStub) DeleteChatSession(_ context.Context, owner workspace.Owner, sessionID string) error {
	s.deleteOwner, s.deleteID = owner, sessionID
	return nil
}

type workspaceImageReaderStub struct {
	owner workspace.Owner
	limit int
	jobs  []workspace.ImageJob
}

func (s *workspaceImageReaderStub) ListImageJobs(_ context.Context, owner workspace.Owner, limit int) ([]workspace.ImageJob, error) {
	s.owner, s.limit = owner, limit
	return s.jobs, nil
}

func TestWorkspaceBuildersUseSeparatedCapabilityPorts(t *testing.T) {
	ctx := context.WithValue(t.Context(), authClaimsContextKey{}, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})
	overview := &workspaceOverviewStub{value: workspace.Overview{
		RecentChatSessions: []workspace.ChatSession{{ID: "session-recent"}},
		RecentImageJobs:    []workspace.ImageJob{{ID: "job-recent"}},
	}}
	models := &workspaceModelReaderStub{models: []workspace.ChatModel{{ModelCode: "gpt-test", GroupID: "group-1"}}}
	sessions := &workspaceSessionReaderStub{
		sessions: []workspace.ChatSession{{ID: "session-1"}},
		session:  workspace.ChatSession{ID: "session-1"},
		messages: []workspace.ChatMessage{{ID: "message-1"}},
	}
	manager := &workspaceSessionManagerStub{session: workspace.ChatSession{ID: "session-created"}}
	images := &workspaceImageReaderStub{jobs: []workspace.ImageJob{{ID: "job-1"}}}
	usage := &usageQueryReaderStub{}
	d := WorkspaceHTTPDeps{
		WorkspaceOverview: overview,
		WorkspaceModels:   models,
		WorkspaceSessions: sessions,
		WorkspaceManager:  manager,
		WorkspaceImages:   images,
		UsageQueries:      usage,
	}

	if _, err := buildUserWorkspaceOverview(ctx, d, &workspaceOverviewInput{LogLimit: 10, ItemLimit: 5}); err != nil {
		t.Fatalf("overview: %v", err)
	}
	if _, err := buildWorkspaceChatModels(ctx, d, identity.ScopeUser); err != nil {
		t.Fatalf("models: %v", err)
	}
	if _, err := buildWorkspaceChatSessions(ctx, d, identity.ScopeUser, &workspaceItemsInput{Limit: 7}); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if _, err := buildWorkspaceChatSessionDetail(ctx, d, identity.ScopeUser, &workspaceSessionDetailInput{SessionID: "session-1"}); err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if _, err := buildWorkspaceChatSessionCreate(ctx, d, identity.ScopeUser, &workspaceChatSessionCreateInput{Body: workspaceChatSessionCreateBody{ModelCode: "gpt-test", GroupID: "group-1", Title: "New"}}); err != nil {
		t.Fatalf("session create: %v", err)
	}
	if _, err := buildWorkspaceChatSessionDelete(ctx, d, identity.ScopeUser, &workspaceSessionDetailInput{SessionID: "session-1"}); err != nil {
		t.Fatalf("session delete: %v", err)
	}
	if _, err := buildWorkspaceImageJobs(ctx, d, identity.ScopeUser, &workspaceItemsInput{Limit: 9}); err != nil {
		t.Fatalf("images: %v", err)
	}

	wantOwner := workspace.Owner{Scope: identity.ScopeUser, TenantID: "tenant-1", UserID: "user-1"}
	if overview.owner != wantOwner || overview.limit != 5 || usage.userTenantID != "tenant-1" || usage.userID != "user-1" {
		t.Fatalf("overview scope = %#v limit %d, usage tenant %q user %q", overview.owner, overview.limit, usage.userTenantID, usage.userID)
	}
	if models.owner != wantOwner || sessions.listOwner != wantOwner || sessions.listLimit != 7 || sessions.getID != "session-1" || sessions.messageID != "session-1" {
		t.Fatalf("read scopes = models %#v sessions %#v/%d detail %q messages %q", models.owner, sessions.listOwner, sessions.listLimit, sessions.getID, sessions.messageID)
	}
	if manager.createOwner != wantOwner || manager.createInput.ModelCode != "gpt-test" || manager.deleteOwner != wantOwner || manager.deleteID != "session-1" {
		t.Fatalf("manager calls = create %#v/%#v delete %#v/%q", manager.createOwner, manager.createInput, manager.deleteOwner, manager.deleteID)
	}
	if images.owner != wantOwner || images.limit != 9 {
		t.Fatalf("image scope = %#v limit %d", images.owner, images.limit)
	}
}

func TestWorkspaceModelPortDoesNotEnableSessionQueries(t *testing.T) {
	ctx := context.WithValue(t.Context(), authClaimsContextKey{}, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})
	d := WorkspaceHTTPDeps{WorkspaceModels: &workspaceModelReaderStub{}}
	if _, err := buildWorkspaceChatModels(ctx, d, identity.ScopeUser); err != nil {
		t.Fatalf("model-only query: %v", err)
	}
	if _, err := buildWorkspaceChatSessions(ctx, d, identity.ScopeUser, &workspaceItemsInput{Limit: 1}); err == nil {
		t.Fatal("session query succeeded without WorkspaceSessions")
	}
}
