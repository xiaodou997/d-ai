// Package limit holds the business logic for runtime limit-policy management
// (the console management plane). Service owns validation and default filling;
// persistence is reached through Repository, defined on the consumer side.
package limit

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the limit service. The
// underlying table has no delete query, so the port is create/list/update only.
type Repository interface {
	Create(ctx context.Context, w PolicyWrite) (domain.RuntimeLimitPolicy, error)
	List(ctx context.Context) ([]domain.RuntimeLimitPolicy, error)
	Update(ctx context.Context, id string, w PolicyWrite) (domain.RuntimeLimitPolicy, error)
	UpdateStatus(ctx context.Context, id, status string) (domain.RuntimeLimitPolicy, error)
}

// PolicyWrite is the persistence-level payload for create/update. CreatedBy is
// only consumed on create (Update ignores it).
type PolicyWrite struct {
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
