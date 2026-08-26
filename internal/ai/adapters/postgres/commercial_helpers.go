package postgres

import (
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/routing"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

func legacyGroupToCommercial(item domain.Group) commercial.Group {
	return commercial.Group{
		ID:                      item.ID,
		TenantID:                item.TenantID,
		Code:                    item.Name,
		Name:                    item.Name,
		Description:             item.Description,
		RetailPriceBookID:       item.RetailPriceBookID,
		DefaultUserMultiplier:   item.DefaultUserMultiplier,
		UserDefaultVisible:      item.UserDefaultVisible,
		AllowProtocolConversion: item.AllowProtocolConversion,
		Status:                  commercial.Status(item.Status),
		SortOrder:               int(item.SortOrder),
		CreatedAt:               item.CreatedAt,
		UpdatedAt:               item.UpdatedAt,
	}
}

func groupTargetBindingToCommercial(item domain.GroupTargetBinding) commercial.GroupTarget {
	return commercial.GroupTarget{
		ID:         item.ID,
		GroupID:    item.GroupID,
		TargetKind: commercial.TargetKind(item.TargetKind),
		TargetID:   item.TargetID,
		Priority:   int(item.Priority),
		Status:     commercial.Status(item.Status),
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}

func groupTargetDetailToCommercial(item domain.GroupTargetDetail) commercial.GroupTargetDetail {
	return commercial.GroupTargetDetail{
		GroupTarget:       groupTargetBindingToCommercial(item.GroupTargetBinding),
		AccountName:       item.AccountName,
		DefaultProtocol:   item.DefaultProtocol,
		PoolName:          item.PoolName,
		FixedProviderType: item.FixedProviderType,
		Available:         item.Available,
		UnavailableReason: item.UnavailableReason,
	}
}

func dispatchRuleToCommercial(item domain.GroupModelDispatchRule) (commercial.DispatchRule, error) {
	return commercial.DispatchRule{
		ID:            item.ID,
		GroupID:       item.GroupID,
		ClientSurface: surface.ID(item.ClientSurface),
		MatchType:     commercial.DispatchMatchType(item.MatchType),
		MatchValue:    item.MatchValue,
		TargetModelID: item.TargetModelCode,
		Priority:      int(item.Priority),
		Status:        commercial.Status(item.Status),
		Notes:         item.Notes,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}, nil
}

func legacyUserBindingToCommercial(item domain.UserGroup) commercial.UserGroupBinding {
	return commercial.UserGroupBinding{
		ID:                     item.ID,
		TenantID:               item.TenantID,
		UserID:                 item.UserID,
		GroupID:                item.GroupID,
		UserMultiplierOverride: item.UserMultiplierOverride,
		CreatedBy:              item.CreatedBy,
		UpdatedBy:              item.CreatedBy,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
}

func routingPolicyFromRow(id pgtype.UUID, scopeKey string, weightsJSON []byte, updatedAt pgtype.Timestamptz) (routing.Policy, error) {
	var weights routing.WeightSet
	if len(weightsJSON) > 0 {
		if err := json.Unmarshal(weightsJSON, &weights); err != nil {
			return routing.Policy{}, fmt.Errorf("unmarshal routing weights for %s: %w", scopeKey, err)
		}
	}
	scopeType, scopeID := decodeRoutingScope(scopeKey)
	return routing.Policy{
		ID:        uuidToString(id),
		ScopeType: scopeType,
		ScopeID:   scopeID,
		Weights:   weights,
		CreatedAt: updatedAt.Time,
		UpdatedAt: updatedAt.Time,
	}, nil
}

func scanCommercialGroupClientSurfaceRow(scanner interface {
	Scan(dest ...any) error
}) (commercial.GroupClientSurface, error) {
	var item commercial.GroupClientSurface
	if err := scanner.Scan(
		&item.ID,
		&item.GroupID,
		&item.Surface,
		&item.BridgeEnabled,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return commercial.GroupClientSurface{}, err
	}
	return item, nil
}

func commercialNameOrCode(name, code string) string {
	return firstNonEmpty(strings.TrimSpace(name), strings.TrimSpace(code))
}

func commercialStatusOrDefault(status commercial.Status) string {
	switch status {
	case commercial.StatusDisabled:
		return string(commercial.StatusDisabled)
	case commercial.StatusActive:
		return string(commercial.StatusActive)
	default:
		return string(commercial.StatusActive)
	}
}

func validateLegacyLimitBridge(in commercial.LimitPolicyWrite) error {
	if strings.TrimSpace(in.ScopeID) == "" {
		return domain.NewValidationError("scope_id", "scope_id is required")
	}
	if in.Capability != "" {
		return domain.NewValidationError("capability", "current postgres commercial adapter only supports subject-scope limit policies")
	}
	if strings.TrimSpace(in.ModelID) != "" {
		return domain.NewValidationError("model_id", "current postgres commercial adapter only supports subject-scope limit policies")
	}
	return nil
}

func providerFamilyToLegacy(raw string) string {
	switch catalog.ProviderFamily(raw) {
	case catalog.ProviderFamilyOpenAICompatible:
		return string(catalog.ProviderFamilyOpenAICompatible)
	case catalog.ProviderFamilyAnthropic:
		return string(catalog.ProviderFamilyAnthropic)
	case catalog.ProviderFamilyGoogle:
		return string(domain.EndpointProtocolGemini)
	default:
		return ""
	}
}

func legacyProviderFamilyToCatalog(raw string) catalog.ProviderFamily {
	switch raw {
	case string(catalog.ProviderFamilyOpenAICompatible):
		return catalog.ProviderFamilyOpenAICompatible
	case string(catalog.ProviderFamilyAnthropic):
		return catalog.ProviderFamilyAnthropic
	case string(catalog.ProviderFamilyGoogle), string(domain.EndpointProtocolGemini):
		return catalog.ProviderFamilyGoogle
	default:
		return ""
	}
}

func encodeRoutingScope(scopeType routing.ScopeType, scopeID string) (string, error) {
	switch scopeType {
	case routing.ScopeGlobal:
		return legacyRoutingScopeGlobal, nil
	case routing.ScopeTenant, routing.ScopeGroup, routing.ScopeUpstream:
		scopeID = strings.TrimSpace(scopeID)
		if scopeID == "" {
			return "", domain.NewValidationError("scope_id", "scope_id is required")
		}
		return string(scopeType) + ":" + scopeID, nil
	default:
		return "", domain.NewValidationError("scope_type", "unsupported scope_type")
	}
}

func decodeRoutingScope(scopeKey string) (routing.ScopeType, string) {
	if scopeKey == legacyRoutingScopeGlobal {
		return routing.ScopeGlobal, legacyRoutingScopeGlobal
	}
	parts := strings.SplitN(scopeKey, ":", 2)
	if len(parts) == 2 {
		switch routing.ScopeType(parts[0]) {
		case routing.ScopeTenant, routing.ScopeGroup, routing.ScopeUpstream:
			return routing.ScopeType(parts[0]), parts[1]
		}
	}
	return routing.ScopeGlobal, scopeKey
}

func intPtrToInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
