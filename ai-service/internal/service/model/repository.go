// Package model holds the business logic for model-catalog management (the
// console management plane). Service owns validation and default filling;
// persistence is reached through Repository, defined on the consumer side.
package model

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the model service.
type Repository interface {
	Create(ctx context.Context, w ModelWrite) (domain.ManagedModel, error)
	List(ctx context.Context) ([]domain.ManagedModel, error)
	Update(ctx context.Context, id string, w ModelWrite) (domain.ManagedModel, error)
	UpdateStatus(ctx context.Context, id, status string) (domain.ManagedModel, error)
}

// ModelWrite is the persistence-level payload for create/update. The table has
// no display_name column. DefaultMaxOutputTokens is non-null (the service fills
// the DB default when the request omits it).
type ModelWrite struct {
	ModelCode              string
	CapabilityType         string
	ContextWindow          *int32
	DefaultMaxOutputTokens int32
	MaxOutputTokens        *int32
	Status                 string
}
