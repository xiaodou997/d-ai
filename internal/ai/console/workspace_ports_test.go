package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/internal/auth"
)

type workspaceTokenVerifierStub struct{}

func (workspaceTokenVerifierStub) ParseToken(context.Context, string) (*auth.Claims, error) {
	return &auth.Claims{
		PrincipalType: "user",
		TokenUse:      "access",
		SessionID:     "session-auth",
		TenantID:      "tenant-1",
		UserID:        "user-1",
		UserType:      4,
	}, nil
}

func TestConsoleWiresSeparatedWorkspacePorts(t *testing.T) {
	ports := workspace.NewService(nil)
	console := New(Deps{
		Logger:            zap.NewNop(),
		WorkspaceModels:   ports,
		WorkspaceSessions: ports,
		WorkspaceManager:  ports,
		WorkspaceMessages: ports,
		WorkspaceImages:   ports,
	})

	if console.workspaceModels != ports || console.workspaceSessions != ports || console.workspaceManager != ports || console.workspaceMessages != ports || console.workspaceImages != ports {
		t.Fatal("workspace capability ports were not preserved")
	}
}

func TestConsoleChatStreamRequiresMessagePersistencePort(t *testing.T) {
	console := New(Deps{
		Logger:            zap.NewNop(),
		TokenVerifier:     workspaceTokenVerifierStub{},
		WorkspaceSessions: workspace.NewService(nil),
	})
	request := httptest.NewRequest(http.MethodPost, "/runtime/v1/chat/sessions/session-1/messages:stream", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	console.handleConsoleChatStream(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
}
