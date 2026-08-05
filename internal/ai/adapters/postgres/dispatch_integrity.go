package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/dai/internal/ai/domain"
)

type lockedGroup struct {
	ID          string
	TenantID    string
	Name        string
	PriceBookID string
	Status      string
}

func lockGroupForTenant(ctx context.Context, tx pgx.Tx, tenantID string, groupID pgtype.UUID) (lockedGroup, error) {
	var group lockedGroup
	err := tx.QueryRow(ctx, `
		SELECT id::text, tenant_id, name, retail_price_book_id::text, status
		FROM ai_groups
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE
	`, groupID, tenantID).Scan(&group.ID, &group.TenantID, &group.Name, &group.PriceBookID, &group.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedGroup{}, domain.ErrNotFound
	}
	return group, err
}

func validateVisibleActivePriceBook(ctx context.Context, tx pgx.Tx, tenantID string, priceBookID pgtype.UUID, groupID, groupName string) error {
	var ownerType, ownerTenantID, status string
	err := tx.QueryRow(ctx, `
		SELECT owner_type, owner_tenant_id, status
		FROM ai_price_books
		WHERE id = $1
		FOR SHARE
	`, priceBookID).Scan(&ownerType, &ownerTenantID, &status)
	if err == nil && status == "active" && (ownerType == "platform" || (ownerType == "tenant" && ownerTenantID == tenantID)) {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return &domain.DispatchRulePriceConflictError{Conflicts: []domain.DispatchRulePriceConflict{{
		GroupID:   groupID,
		GroupName: groupName,
	}}}
}

func dispatchRequiredCapability(apiFormat string) string {
	switch apiFormat {
	case "openai_embeddings", "gemini_embeddings":
		return "embedding"
	case "openai_images", "gemini_images":
		return "image"
	default:
		return "chat"
	}
}

func validateRulePrice(ctx context.Context, tx pgx.Tx, group lockedGroup, ruleID, apiFormat, matchValue, targetModel string) error {
	capability := dispatchRequiredCapability(apiFormat)
	var priced bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM ai_price_book_entries
			WHERE price_book_id = $1::uuid
			  AND model_code = $2
			  AND capability_type = $3
		)
	`, group.PriceBookID, targetModel, capability).Scan(&priced); err != nil {
		return err
	}
	if priced {
		return nil
	}
	return &domain.DispatchRulePriceConflictError{Conflicts: []domain.DispatchRulePriceConflict{{
		GroupID:            group.ID,
		GroupName:          group.Name,
		RuleID:             ruleID,
		APIFormat:          apiFormat,
		MatchValue:         matchValue,
		TargetModel:        targetModel,
		RequiredCapability: capability,
	}}}
}

func validateActiveRulesAgainstBook(ctx context.Context, tx pgx.Tx, group lockedGroup, priceBookID pgtype.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT r.id::text, r.client_surface, r.match_value, r.target_model_code
		FROM ai_group_model_dispatch_rules r
		WHERE r.group_id = $1::uuid
		  AND r.status = 'active'
		  AND NOT EXISTS (
			SELECT 1
			FROM ai_price_book_entries e
			WHERE e.price_book_id = $2
			  AND e.model_code = r.target_model_code
			  AND e.capability_type = CASE
				WHEN r.client_surface IN ('openai_embeddings', 'gemini_embeddings') THEN 'embedding'
				WHEN r.client_surface IN ('openai_images', 'gemini_images') THEN 'image'
				ELSE 'chat'
			  END
		  )
		ORDER BY r.priority, r.created_at, r.id
	`, group.ID, priceBookID)
	if err != nil {
		return err
	}
	defer rows.Close()

	conflicts := make([]domain.DispatchRulePriceConflict, 0)
	for rows.Next() {
		var item domain.DispatchRulePriceConflict
		if err := rows.Scan(&item.RuleID, &item.APIFormat, &item.MatchValue, &item.TargetModel); err != nil {
			return err
		}
		item.GroupID = group.ID
		item.GroupName = group.Name
		item.RequiredCapability = dispatchRequiredCapability(item.APIFormat)
		conflicts = append(conflicts, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return &domain.DispatchRulePriceConflictError{Conflicts: conflicts}
	}
	return nil
}

func lockGroupsForPriceBook(ctx context.Context, tx pgx.Tx, priceBookID pgtype.UUID) ([]lockedGroup, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, tenant_id, name, retail_price_book_id::text, status
		FROM ai_groups
		WHERE retail_price_book_id = $1
		ORDER BY id
		FOR UPDATE
	`, priceBookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]lockedGroup, 0)
	for rows.Next() {
		var group lockedGroup
		if err := rows.Scan(&group.ID, &group.TenantID, &group.Name, &group.PriceBookID, &group.Status); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func priceBookReferenceConflicts(groups []lockedGroup) error {
	if len(groups) == 0 {
		return nil
	}
	conflicts := make([]domain.DispatchRulePriceConflict, 0, len(groups))
	for _, group := range groups {
		conflicts = append(conflicts, domain.DispatchRulePriceConflict{
			GroupID:   group.ID,
			GroupName: group.Name,
		})
	}
	return &domain.DispatchRulePriceConflictError{Conflicts: conflicts}
}

func priceEntryReferenceConflicts(ctx context.Context, tx pgx.Tx, priceBookID pgtype.UUID, modelCode, capability string) error {
	rows, err := tx.Query(ctx, `
		SELECT g.id::text, g.name, r.id::text, r.client_surface, r.match_value, r.target_model_code
		FROM ai_groups g
		JOIN ai_group_model_dispatch_rules r ON r.group_id = g.id AND r.status = 'active'
		WHERE g.retail_price_book_id = $1
		  AND r.target_model_code = $2
		  AND CASE
			WHEN r.client_surface IN ('openai_embeddings', 'gemini_embeddings') THEN 'embedding'
			WHEN r.client_surface IN ('openai_images', 'gemini_images') THEN 'image'
			ELSE 'chat'
		  END = $3
		ORDER BY g.id, r.priority, r.created_at, r.id
	`, priceBookID, modelCode, capability)
	if err != nil {
		return err
	}
	defer rows.Close()

	conflicts := make([]domain.DispatchRulePriceConflict, 0)
	for rows.Next() {
		var item domain.DispatchRulePriceConflict
		if err := rows.Scan(&item.GroupID, &item.GroupName, &item.RuleID, &item.APIFormat, &item.MatchValue, &item.TargetModel); err != nil {
			return err
		}
		item.RequiredCapability = capability
		conflicts = append(conflicts, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return &domain.DispatchRulePriceConflictError{Conflicts: conflicts}
	}
	return nil
}
