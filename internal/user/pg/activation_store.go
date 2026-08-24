package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
	"xiaodou/dai/internal/auth"
)

// activationCredentialStore is the narrow auth capability required by
// account repositories to persist a pending activation within their own
// PostgreSQL transaction.
type activationCredentialStore interface {
	Store(ctx context.Context, tx pgx.Tx, userID, purpose string, credential auth.ActivationCredential) error
}

type activationService interface {
	activationCredentialStore
	Reset(ctx context.Context, userID string) (auth.ActivationResult, error)
}
