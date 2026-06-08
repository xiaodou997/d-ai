package model

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// defaultMaxOutputTokens mirrors the DB default applied by the old handler when
// the request omits default_max_output_tokens.
const defaultMaxOutputTokens int32 = 2048

// Service implements model-catalog management business logic.
type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// ModelInput is the decoded create/update request. An empty CapabilityType
// defaults to "chat"; an empty Status defaults to active; a nil
// DefaultMaxOutputTokens falls back to the DB default.
type ModelInput struct {
	ModelCode              string
	CapabilityType         string
	ContextWindow          *int32
	DefaultMaxOutputTokens *int32
	MaxOutputTokens        *int32
	Status                 string
}

func (s *Service) Create(ctx context.Context, in ModelInput) (domain.ManagedModel, error) {
	w, err := normalize(in)
	if err != nil {
		return domain.ManagedModel{}, err
	}
	return s.repo.Create(ctx, w)
}

func (s *Service) List(ctx context.Context) ([]domain.ManagedModel, error) {
	return s.repo.List(ctx)
}

func (s *Service) Update(ctx context.Context, id string, in ModelInput) (domain.ManagedModel, error) {
	w, err := normalize(in)
	if err != nil {
		return domain.ManagedModel{}, err
	}
	return s.repo.Update(ctx, id, w)
}

func (s *Service) UpdateStatus(ctx context.Context, id, status string) (domain.ManagedModel, error) {
	if status == "" {
		return domain.ManagedModel{}, domain.NewValidationError("status", "status is required")
	}
	return s.repo.UpdateStatus(ctx, id, status)
}

func normalize(in ModelInput) (ModelWrite, error) {
	if in.ModelCode == "" {
		return ModelWrite{}, domain.NewValidationError("model_code", "model_code is required")
	}
	capability := in.CapabilityType
	if capability == "" {
		capability = "chat"
	}
	status := in.Status
	if status == "" {
		status = domain.APIKeyStatusActive
	}
	maxOut := defaultMaxOutputTokens
	if in.DefaultMaxOutputTokens != nil {
		maxOut = *in.DefaultMaxOutputTokens
	}
	return ModelWrite{
		ModelCode:              in.ModelCode,
		CapabilityType:         capability,
		ContextWindow:          in.ContextWindow,
		DefaultMaxOutputTokens: maxOut,
		MaxOutputTokens:        in.MaxOutputTokens,
		Status:                 status,
	}, nil
}
