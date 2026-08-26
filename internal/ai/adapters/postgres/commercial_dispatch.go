package postgres

import (
	"context"
	"errors"
	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

func (r *CommercialRepo) AddDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, in commercial.DispatchRuleWrite) (commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := validateRulePrice(ctx, tx, group, "", string(in.ClientSurface), in.MatchValue, in.TargetModelID); err != nil {
		return commercial.DispatchRule{}, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO ai_group_model_dispatch_rules (
			group_id,
			client_surface,
			match_type,
			match_value,
			target_model_code,
			priority,
			status,
			notes
		) VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
		RETURNING id::text, group_id::text, client_surface, match_type, match_value, target_model_code,
		          priority, status, notes, created_at, updated_at
	`, gid, string(in.ClientSurface), string(in.MatchType), in.MatchValue, in.TargetModelID, in.Priority, in.Notes)
	item, err := scanCommercialDispatchRuleRow(row)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := setDispatchRulePriceMetadata(ctx, tx, group.PriceBookID, &item); err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.DispatchRule{}, err
	}
	return item, nil
}

func (r *CommercialRepo) ListDispatchRules(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return nil, domain.NewValidationError("group_id", "invalid group_id")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.id::text, r.group_id::text, r.client_surface, r.match_type, r.match_value, r.target_model_code,
		       r.priority, r.status, r.notes, r.created_at, r.updated_at,
		       CASE
		         WHEN r.client_surface IN ('openai_embeddings', 'gemini_embeddings') THEN 'embedding'
		         WHEN r.client_surface IN ('openai_images', 'gemini_images') THEN 'image'
		         ELSE 'chat'
		       END AS required_capability,
		       EXISTS (
		         SELECT 1 FROM ai_price_book_entries e
		         WHERE e.price_book_id = g.retail_price_book_id
		           AND e.model_code = r.target_model_code
		           AND e.capability_type = CASE
		             WHEN r.client_surface IN ('openai_embeddings', 'gemini_embeddings') THEN 'embedding'
		             WHEN r.client_surface IN ('openai_images', 'gemini_images') THEN 'image'
		             ELSE 'chat'
		           END
		       ) AS priced
		FROM ai_group_model_dispatch_rules r
		JOIN ai_groups g ON g.id = r.group_id
		WHERE r.group_id = $1 AND g.tenant_id = $2
		ORDER BY r.priority ASC, r.created_at ASC
	`, gid, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]commercial.DispatchRule, 0)
	for rows.Next() {
		item, scanErr := scanCommercialDispatchRuleWithPriceRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) ListDispatchModels(ctx context.Context, scope commercial.TenantGroupScope, clientSurface surface.ID) ([]commercial.DispatchModel, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return nil, domain.NewValidationError("group_id", "invalid group_id")
	}
	capability := dispatchRequiredCapability(string(clientSurface))
	rows, err := r.pool.Query(ctx, `
		SELECT e.model_code,
		       e.capability_type,
		       COUNT(DISTINCT CASE WHEN gt.status = 'active' AND um.status = 'active' THEN gt.target_id END)::int
		FROM ai_groups g
		JOIN ai_price_book_entries e ON e.price_book_id = g.retail_price_book_id
		LEFT JOIN ai_group_targets gt ON gt.group_id = g.id
		LEFT JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.model_code = e.model_code
		 AND um.capability_type = e.capability_type
		WHERE g.id = $1 AND g.tenant_id = $2 AND e.capability_type = $3
		GROUP BY e.model_code, e.capability_type
		ORDER BY e.model_code ASC
	`, gid, scope.TenantID, capability)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]commercial.DispatchModel, 0)
	for rows.Next() {
		var item commercial.DispatchModel
		if err := rows.Scan(&item.ModelCode, &item.Capability, &item.AvailableTargets); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) UpdateDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, id string, in commercial.DispatchRuleWrite) (commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	rid, err := akUUID(id)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2`, rid, gid).Scan(&currentStatus); err != nil {
		return commercial.DispatchRule{}, err
	}
	if currentStatus == string(commercial.StatusActive) {
		if err := validateRulePrice(ctx, tx, group, id, string(in.ClientSurface), in.MatchValue, in.TargetModelID); err != nil {
			return commercial.DispatchRule{}, err
		}
	}
	row := tx.QueryRow(ctx, `
		UPDATE ai_group_model_dispatch_rules
		SET client_surface = $1,
		    match_type = $2,
		    match_value = $3,
		    target_model_code = $4,
		    priority = $5,
		    notes = $6,
		    updated_at = now()
		WHERE id = $7 AND group_id = $8
		RETURNING id::text, group_id::text, client_surface, match_type, match_value, target_model_code,
		          priority, status, notes, created_at, updated_at
	`, string(in.ClientSurface), string(in.MatchType), in.MatchValue, in.TargetModelID, in.Priority, in.Notes, rid, gid)
	item, err := scanCommercialDispatchRuleRow(row)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := setDispatchRulePriceMetadata(ctx, tx, group.PriceBookID, &item); err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.DispatchRule{}, err
	}
	return item, nil
}

func (r *CommercialRepo) UpdateDispatchRuleStatus(ctx context.Context, scope commercial.TenantGroupScope, id string, status commercial.Status) (commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	rid, err := akUUID(id)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	var clientSurface, matchValue, targetModel string
	if err := tx.QueryRow(ctx, `SELECT client_surface, match_value, target_model_code FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2`, rid, gid).Scan(&clientSurface, &matchValue, &targetModel); err != nil {
		return commercial.DispatchRule{}, err
	}
	if status == commercial.StatusActive {
		if err := validateRulePrice(ctx, tx, group, id, clientSurface, matchValue, targetModel); err != nil {
			return commercial.DispatchRule{}, err
		}
	}
	row := tx.QueryRow(ctx, `
		UPDATE ai_group_model_dispatch_rules SET status = $1, updated_at = now()
		WHERE id = $2 AND group_id = $3
		RETURNING id::text, group_id::text, client_surface, match_type, match_value, target_model_code,
		          priority, status, notes, created_at, updated_at
	`, string(status), rid, gid)
	item, err := scanCommercialDispatchRuleRow(row)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := setDispatchRulePriceMetadata(ctx, tx, group.PriceBookID, &item); err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.DispatchRule{}, err
	}
	return item, nil
}

func (r *CommercialRepo) DeleteDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, id string) error {
	rid, err := akUUID(id)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var groupID string
	if err := tx.QueryRow(ctx, `
		SELECT r.group_id::text
		FROM ai_group_model_dispatch_rules r
		JOIN ai_groups g ON g.id = r.group_id
		WHERE r.id = $1 AND g.tenant_id = $2
	`, rid, scope.TenantID).Scan(&groupID); err != nil {
		return err
	}
	if groupID != scope.GroupID {
		return domain.ErrNotFound
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1::uuid AND tenant_id = $2 FOR UPDATE`, groupID, scope.TenantID).Scan(&lockedID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2`, rid, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *CommercialRepo) PreviewDispatch(ctx context.Context, scope commercial.TenantGroupScope, requestedModel string, clientSurface surface.ID) (commercial.DispatchPreview, error) {
	if r.runtimeResolver == nil {
		return commercial.DispatchPreview{}, errors.New("runtime resolver is not configured")
	}
	return r.previewWithRuntime(ctx, scope.TenantID, scope.GroupID, requestedModel, string(clientSurface))
}

func (r *CommercialRepo) previewWithRuntime(ctx context.Context, tenantID, groupID, requestedModel, clientSurface string) (commercial.DispatchPreview, error) {
	surfaceID := surface.ID(clientSurface)
	capability := catalog.CapabilityChat
	if surfaceID == surface.OpenAIEmbeddings || surfaceID == surface.GeminiEmbeddings {
		capability = catalog.CapabilityEmbedding
	} else if surfaceID == surface.OpenAIImages || surfaceID == surface.GeminiImages {
		capability = catalog.CapabilityImageGeneration
	}
	inspection, err := r.runtimeResolver.Inspect(ctx, identity.Subject{TenantID: tenantID, ForcedGroupID: groupID}, coreruntime.Request{
		RequestedModel: requestedModel, ClientSurface: surfaceID, Capability: capability, ForcedGroupID: groupID,
	})
	if err != nil {
		return commercial.DispatchPreview{}, err
	}
	out := commercial.DispatchPreview{
		RequestedModel:     requestedModel,
		ClientSurface:      clientSurface,
		ResolvedModelID:    requestedModel,
		CandidateUpstreams: make([]commercial.DispatchPreviewCandidate, 0, len(inspection.Candidates)),
		RejectedCandidates: make([]commercial.DispatchPreviewRejection, 0, len(inspection.RejectedCandidates)),
	}
	for _, candidate := range inspection.Candidates {
		if out.ResolvedModelID == requestedModel {
			out.ResolvedModelID = candidate.ModelID
		}
		if candidate.MatchedRule != nil && out.MatchedRule == nil {
			rule := *candidate.MatchedRule
			out.MatchedRule = &rule
		}
		binding := candidate.Binding
		item := commercial.DispatchPreviewCandidate{DisplayName: binding.Upstream.Name, ProviderFamily: string(binding.Upstream.ProviderFamily), UpstreamModel: binding.ModelBinding.UpstreamModelName, ProtocolConversion: binding.ModelBinding.RequestSurface != surfaceID, Priority: candidate.Target.Priority}
		if candidate.Target.TargetKind == commercial.TargetKindOAuthPool {
			item.TargetType = "pool"
			item.CredentialPoolID = binding.Upstream.ID
		} else {
			item.TargetType = "account"
			item.AccountID = binding.Upstream.ID
		}
		if protocol, protocolErr := dispatchProtocolFromSurface(binding.ModelBinding.RequestSurface); protocolErr == nil {
			item.SelectedProtocol = string(protocol)
		}
		out.CandidateUpstreams = append(out.CandidateUpstreams, item)
	}
	for _, rejected := range inspection.RejectedCandidates {
		if out.ResolvedModelID == requestedModel {
			out.ResolvedModelID = rejected.ModelID
		}
		if rejected.MatchedRule != nil && out.MatchedRule == nil {
			rule := *rejected.MatchedRule
			out.MatchedRule = &rule
		}
		item := commercial.DispatchPreviewRejection{
			TargetID:        rejected.Target.TargetID,
			ResolvedModelID: rejected.ModelID,
			ReasonCode:      string(rejected.Code),
			ReasonDetail:    rejected.Detail,
			Priority:        rejected.Target.Priority,
		}
		switch rejected.Target.TargetKind {
		case commercial.TargetKindOAuthPool:
			item.TargetType = "pool"
		case commercial.TargetKindDirectUpstream:
			item.TargetType = "account"
		}
		out.RejectedCandidates = append(out.RejectedCandidates, item)
	}
	return out, nil
}
