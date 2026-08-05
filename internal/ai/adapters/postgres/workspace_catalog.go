package postgres

import (
	"context"
	"fmt"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/workspace"
)

var workspaceChatProtocolOrder = []domain.UpstreamProtocol{
	domain.ProtocolOpenAIResponses,
	domain.ProtocolOpenAIChat,
	domain.ProtocolAnthropicMessages,
	domain.ProtocolGeminiGenerate,
}

type WorkspaceChatCatalog struct {
	repo *WorkspaceRepo
}

func NewWorkspaceChatCatalog(repo *WorkspaceRepo) *WorkspaceChatCatalog {
	return &WorkspaceChatCatalog{repo: repo}
}

func (c *WorkspaceChatCatalog) ListChatModels(ctx context.Context, owner workspace.Owner) ([]workspace.ChatModel, error) {
	if c == nil || c.repo == nil {
		return []workspace.ChatModel{}, nil
	}
	subject := &coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodJWT,
		RequestSource: coreidentity.RequestSourceWebChat,
		Scope:         owner.Scope,
		TenantID:      owner.TenantID,
		UserID:        owner.UserID,
	}
	groups, err := c.repo.grantChecker.AccessibleGroupIDsForSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []workspace.ChatModel{}, nil
	}
	rows, err := c.repo.listWorkspaceAuthorizedChatModels(ctx, owner, groups)
	if err != nil {
		return nil, err
	}
	out := make([]workspace.ChatModel, 0, len(rows))
	for _, row := range rows {
		protocols, err := c.repo.availableWorkspaceChatProtocols(ctx, row.ModelCode, []string{row.GroupID})
		if err != nil {
			return nil, err
		}
		if len(protocols) == 0 {
			continue
		}
		out = append(out, workspace.ChatModel{
			GroupID:                 row.GroupID,
			GroupName:               row.GroupName,
			EffectiveUserMultiplier: row.EffectiveUserMultiplier,
			BillingGroupLabel:       row.BillingGroupLabel,
			ModelCode:               row.ModelCode,
			CapabilityType:          row.CapabilityType,
			DefaultProtocol:         string(protocols[0]),
			AvailableProtocols:      workspaceProtocolStrings(protocols),
			SupportsStream:          true,
			Status:                  "available",
		})
	}
	return out, nil
}

func (r *WorkspaceRepo) listWorkspaceAuthorizedChatModels(ctx context.Context, owner workspace.Owner, groupIDs []string) ([]workspace.ChatModel, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
		       g.id::text,
		       g.name,
		       COALESCE(ug.user_multiplier_override, g.default_user_multiplier)::float8 AS effective_user_multiplier,
		       um.model_code,
		       um.capability_type
		FROM ai_group_targets gt
		JOIN ai_groups g
		  ON g.id = gt.group_id AND g.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		 AND um.capability_type = 'chat'
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		LEFT JOIN ai_user_groups ug
		  ON ug.group_id = g.id AND ug.tenant_id = $2 AND ug.user_id = $3
		WHERE gt.group_id = ANY($1::uuid[])
		  AND g.tenant_id = $2
		  AND gt.status = 'active'
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		  AND (
		    COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
		    OR EXISTS (
		      SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		      WHERE rg.resource_kind = gt.target_kind AND rg.resource_id = gt.target_id AND rg.tenant_id = $2
		        AND rg.access_granted
		    )
		  )
		GROUP BY g.id,
		         g.name,
		         COALESCE(ug.user_multiplier_override, g.default_user_multiplier),
		         um.model_code,
		         um.capability_type
		ORDER BY array_position($1::uuid[], g.id), um.model_code ASC
	`, groupIDs, owner.TenantID, owner.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]workspace.ChatModel, 0)
	for rows.Next() {
		var item workspace.ChatModel
		if err := rows.Scan(&item.GroupID, &item.GroupName, &item.EffectiveUserMultiplier, &item.ModelCode, &item.CapabilityType); err != nil {
			return nil, err
		}
		item.BillingGroupLabel = workspaceBillingGroupLabel(item.GroupName, item.EffectiveUserMultiplier)
		out = append(out, item)
	}
	return out, rows.Err()
}

func workspaceBillingGroupLabel(groupName string, multiplier float64) string {
	return fmt.Sprintf("%s · %.4gx", groupName, multiplier)
}

func (r *WorkspaceRepo) availableWorkspaceChatProtocols(ctx context.Context, modelCode string, groupIDs []string) ([]domain.UpstreamProtocol, error) {
	out := make([]domain.UpstreamProtocol, 0, len(workspaceChatProtocolOrder))
	for _, protocol := range workspaceChatProtocolOrder {
		ok, err := r.routeInspector.ModelSupportsClientProtocolInGroups(ctx, modelCode, domain.CapabilityChat, groupIDs, protocol, true, false)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, protocol)
		}
	}
	return out, nil
}

func workspaceProtocolStrings(protocols []domain.UpstreamProtocol) []string {
	out := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		out = append(out, string(protocol))
	}
	return out
}
