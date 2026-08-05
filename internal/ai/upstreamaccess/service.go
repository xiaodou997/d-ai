package upstreamaccess

import (
	"context"
	"math"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

const (
	ModePublic     = "public"
	ModeRestricted = "restricted"

	KindDirectUpstream = "direct_upstream"
	KindOAuthPool      = "oauth_pool"
)

type ResourceRef struct {
	Kind string `json:"resource_kind"`
	ID   string `json:"resource_id"`
}

type ResourceAccess struct {
	ResourceRef
	InternalName              string
	TenantDisplayName         string
	AccessMode                string
	Status                    string
	AccessGranted             bool
	Allowed                   bool
	DefaultTenantMultiplier   float64
	TenantMultiplierOverride  *float64
	EffectiveTenantMultiplier float64
}

type TenantResourcePolicy struct {
	ResourceRef
	AccessGranted            bool     `json:"access_granted"`
	TenantMultiplierOverride *float64 `json:"tenant_multiplier_override,omitempty"`
}

type Repository interface {
	ListForTenant(context.Context, string) ([]ResourceAccess, error)
	ReplacePolicies(context.Context, string, []TenantResourcePolicy) error
	CanAccess(context.Context, string, ResourceRef) (bool, error)
}

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

func NormalizeMode(value string) (string, error) {
	mode := strings.TrimSpace(value)
	if mode == "" {
		return ModePublic, nil
	}
	switch mode {
	case ModePublic, ModeRestricted:
		return mode, nil
	default:
		return "", domain.NewValidationError("tenant_access_mode", "tenant_access_mode must be public or restricted")
	}
}

func NormalizeDisplayName(internalName, displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		return displayName
	}
	return strings.TrimSpace(internalName)
}

func (s *Service) ListForTenant(ctx context.Context, tenantID string) ([]ResourceAccess, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, domain.NewValidationError("tenant_id", "tenant_id is required")
	}
	return s.repo.ListForTenant(ctx, tenantID)
}

func (s *Service) ReplacePolicies(ctx context.Context, tenantID string, policies []TenantResourcePolicy) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return domain.NewValidationError("tenant_id", "tenant_id is required")
	}
	deduplicated := make([]TenantResourcePolicy, 0, len(policies))
	seen := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		policy.Kind = strings.TrimSpace(policy.Kind)
		policy.ID = strings.TrimSpace(policy.ID)
		if err := validateRef(policy.ResourceRef); err != nil {
			return err
		}
		if value := policy.TenantMultiplierOverride; value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return domain.NewValidationError("tenant_multiplier_override", "tenant_multiplier_override must be a finite number >= 0")
		}
		key := policy.Kind + ":" + policy.ID
		if _, exists := seen[key]; exists {
			return domain.NewValidationError("policies", "each upstream resource may appear only once")
		}
		seen[key] = struct{}{}
		deduplicated = append(deduplicated, policy)
	}
	return s.repo.ReplacePolicies(ctx, tenantID, deduplicated)
}

func (s *Service) CanAccess(ctx context.Context, tenantID string, ref ResourceRef) (bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return false, nil
	}
	if err := validateRef(ref); err != nil {
		return false, err
	}
	return s.repo.CanAccess(ctx, tenantID, ref)
}

func validateRef(ref ResourceRef) error {
	if ref.Kind != KindDirectUpstream && ref.Kind != KindOAuthPool {
		return domain.NewValidationError("resource_kind", "resource_kind must be direct_upstream or oauth_pool")
	}
	if ref.ID == "" {
		return domain.NewValidationError("resource_id", "resource_id is required")
	}
	return nil
}
