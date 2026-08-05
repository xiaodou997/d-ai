package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/core/identity"
)

// ApplicationRuntimeRepo expands persisted invoke keys and app agents into the
// rebuilt application runtime view.
type ApplicationRuntimeRepo struct {
	pool *pgxpool.Pool
}

func NewApplicationRuntimeRepo(pool *pgxpool.Pool) *ApplicationRuntimeRepo {
	return &ApplicationRuntimeRepo{pool: pool}
}

var _ application.RuntimeResolver = (*ApplicationRuntimeRepo)(nil)
var _ application.RuntimeIDResolver = (*ApplicationRuntimeRepo)(nil)

func (r *ApplicationRuntimeRepo) ResolveRuntimeInvocationByKeyHash(ctx context.Context, keyHash string) (application.RuntimeInvocation, error) {
	key, err := NewApplicationInvokeKeyRepo(r.pool).GetInvokeKeyByHash(ctx, keyHash)
	if err != nil {
		return application.RuntimeInvocation{}, err
	}
	return r.resolveRuntimeInvocation(ctx, key)
}

func (r *ApplicationRuntimeRepo) ResolveRuntimeInvocationByID(
	ctx context.Context,
	ownerScope identity.Scope,
	tenantID, userID, keyID string,
) (application.RuntimeInvocation, error) {
	key, err := NewApplicationInvokeKeyRepo(r.pool).GetInvokeKeyByID(
		ctx, string(ownerScope), tenantID, userID, keyID,
	)
	if err != nil {
		return application.RuntimeInvocation{}, err
	}
	return r.resolveRuntimeInvocation(ctx, key)
}

func (r *ApplicationRuntimeRepo) resolveRuntimeInvocation(ctx context.Context, key application.InvokeKey) (application.RuntimeInvocation, error) {
	invocation := application.RuntimeInvocation{InvokeKey: key}

	viewer := AppViewer{TenantID: key.TenantID}
	if key.OwnerScope == identity.ScopeUser {
		viewer.UserID = key.UserID
	}
	agent, err := NewApplicationAppRepo(r.pool).GetVisibleAgentByID(ctx, viewer, key.AppID, []string{
		"chat",
		"image_generation",
		"image_edit",
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.RuntimeInvocation{}, application.ErrRuntimeAppNotVisible
		}
		return application.RuntimeInvocation{}, err
	}
	runtimeApp, err := agentRecordToRuntimeApp(agent)
	if err != nil {
		return application.RuntimeInvocation{}, err
	}
	invocation.App = &runtimeApp
	invocation.BoundModelID = runtimeApp.App.BoundModelID
	invocation.InvokeKey.AppID = runtimeApp.App.ID
	return invocation, nil
}

func agentRecordToRuntimeApp(agent AppAgentRecord) (application.RuntimeApp, error) {
	appType, primaryRole, err := capabilityToRuntime(agent.Capability)
	if err != nil {
		return application.RuntimeApp{}, err
	}
	defaultOptions := map[string]any{}
	if len(agent.DefaultOptions) > 0 {
		_ = json.Unmarshal(agent.DefaultOptions, &defaultOptions)
	}
	ownerScope := identity.ScopeTenant
	switch agent.OwnerType {
	case "platform":
		ownerScope = identity.ScopePlatform
	case "user":
		ownerScope = identity.ScopeUser
	}
	app := application.App{
		ID:             agent.ID,
		OwnerScope:     ownerScope,
		OwnerTenantID:  agent.OwnerTenantID,
		OwnerUserID:    agent.OwnerUserID,
		AppType:        appType,
		PromptStrategy: application.PromptStrategy(agent.PromptStrategy),
		Code:           agent.Name,
		Name:           agent.Name,
		Description:    agent.Description,
		BoundModelID:   agent.ModelCode,
		GroupID:        agent.GroupID,
		DefaultOptions: defaultOptions,
		Status:         application.Status(agent.Status),
	}
	bindings := make([]application.RuntimePromptBinding, 0, len(agent.PromptBindings))
	for _, binding := range agent.PromptBindings {
		var variables []string
		if len(binding.Variables) > 0 {
			_ = json.Unmarshal(binding.Variables, &variables)
		}
		role := application.PromptBindingInputTemplate
		if binding.BindingRole == "primary" {
			role = primaryRole
		}
		bindings = append(bindings, application.RuntimePromptBinding{
			PromptID:       binding.PromptID,
			PromptName:     binding.PromptName,
			PromptRevision: int(binding.CurrentRevision),
			Role:           role,
			BindingOrder:   int(binding.DisplayOrder),
			TemplateText:   binding.TemplateText,
			Variables:      variables,
		})
	}
	return application.RuntimeApp{App: app, PromptBindings: bindings}, nil
}

func capabilityToRuntime(capability string) (application.AppType, application.PromptBindingRole, error) {
	switch strings.TrimSpace(capability) {
	case "chat":
		return application.AppTypeChatAgent, application.PromptBindingSystem, nil
	case "image_generation":
		return application.AppTypeImageGenerationAgent, application.PromptBindingInputTemplate, nil
	case "image_edit":
		return application.AppTypeImageEditAgent, application.PromptBindingInputTemplate, nil
	default:
		return "", "", application.ErrRuntimeAppNotVisible
	}
}
