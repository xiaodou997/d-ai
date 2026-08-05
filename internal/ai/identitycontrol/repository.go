package identitycontrol

import (
	"context"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

// Repository is the control-plane persistence port for AI API key management.
type Repository = coreidentity.APIKeyRepository

// KeyCache is the optional cache invalidation port. *apikey.Cache satisfies it.
type KeyCache interface {
	Del(ctx context.Context, keyHash string) error
}
