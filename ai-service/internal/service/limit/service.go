package limit

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Default capability when the request omits one, mirroring the previous
// handler-layer constant.
const defaultCapability = "chat"

var (
	validScopeTypes = map[string]bool{
		"tenant": true, "user": true, "api_key": true, "provider": true, "endpoint": true,
	}
	validCapabilities = map[string]bool{
		"chat": true, "image": true, "video": true, "embedding": true, "audio": true, "rerank": true,
	}
	validStatuses = map[string]bool{
		"active": true, "inactive": true, "disabled": true,
	}
)

// Service implements runtime limit-policy management business logic.
type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// PolicyInput is the decoded create/update request. Empty CapabilityType /
// Status are defaulted; throttle limits are nil when omitted.
type PolicyInput struct {
	ScopeType        string
	ScopeID          string
	CapabilityType   string
	ModelCode        string
	RpmLimit         *int32
	TpmLimit         *int32
	ConcurrencyLimit *int32
	Status           string
	CreatedBy        string
}

func (s *Service) Create(ctx context.Context, in PolicyInput) (domain.RuntimeLimitPolicy, error) {
	w, err := validateAndNormalize(in)
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	return s.repo.Create(ctx, w)
}

func (s *Service) List(ctx context.Context) ([]domain.RuntimeLimitPolicy, error) {
	return s.repo.List(ctx)
}

func (s *Service) Update(ctx context.Context, id string, in PolicyInput) (domain.RuntimeLimitPolicy, error) {
	w, err := validateAndNormalize(in)
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	return s.repo.Update(ctx, id, w)
}

func (s *Service) UpdateStatus(ctx context.Context, id, status string) (domain.RuntimeLimitPolicy, error) {
	if status == "" {
		return domain.RuntimeLimitPolicy{}, domain.NewValidationError("status", "status is required")
	}
	return s.repo.UpdateStatus(ctx, id, status)
}

// validateAndNormalize enforces the same rules the old handler did: scope/
// capability/status whitelists, default capability+status, at-least-one limit,
// and positive limits. It returns the normalized persistence payload.
func validateAndNormalize(in PolicyInput) (PolicyWrite, error) {
	if in.ScopeType == "" || in.ScopeID == "" {
		return PolicyWrite{}, domain.NewValidationError("", "scope_type and scope_id are required")
	}
	if !validScopeTypes[in.ScopeType] {
		return PolicyWrite{}, domain.NewValidationError("scope_type", "invalid scope_type")
	}
	capability := in.CapabilityType
	if capability == "" {
		capability = defaultCapability
	}
	if !validCapabilities[capability] {
		return PolicyWrite{}, domain.NewValidationError("capability_type", "invalid capability_type")
	}
	status := in.Status
	if status == "" {
		status = domain.APIKeyStatusActive
	}
	if !validStatuses[status] {
		return PolicyWrite{}, domain.NewValidationError("status", "invalid status")
	}
	if in.RpmLimit == nil && in.TpmLimit == nil && in.ConcurrencyLimit == nil {
		return PolicyWrite{}, domain.NewValidationError("", "at least one limit is required")
	}
	if !positiveOptional(in.RpmLimit) || !positiveOptional(in.TpmLimit) || !positiveOptional(in.ConcurrencyLimit) {
		return PolicyWrite{}, domain.NewValidationError("", "limits must be positive")
	}
	return PolicyWrite{
		ScopeType:        in.ScopeType,
		ScopeID:          in.ScopeID,
		CapabilityType:   capability,
		ModelCode:        in.ModelCode,
		RpmLimit:         in.RpmLimit,
		TpmLimit:         in.TpmLimit,
		ConcurrencyLimit: in.ConcurrencyLimit,
		Status:           status,
		CreatedBy:        in.CreatedBy,
	}, nil
}

func positiveOptional(v *int32) bool {
	return v == nil || *v > 0
}
