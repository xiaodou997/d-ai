package clientcredentials

import (
	"context"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
)

type Store interface {
	GetDecryptedByID(ctx context.Context, credentialID string) (*domain.OAuthCredential, error)
}

type Refresher interface {
	RefreshByID(ctx context.Context, credentialID string) error
}

// Adapter joins the encrypted credential store and OAuth refresher at the
// client runtime's credential-refresh seam.
type Adapter struct {
	store     Store
	refresher Refresher
}

func New(store Store, refresher Refresher) *Adapter {
	return &Adapter{store: store, refresher: refresher}
}

func (a *Adapter) Refresh(ctx context.Context, credentialID string) (clientruntime.Credential, error) {
	if err := a.refresher.RefreshByID(ctx, credentialID); err != nil {
		return clientruntime.Credential{}, err
	}
	credential, err := a.store.GetDecryptedByID(ctx, credentialID)
	if err != nil {
		return clientruntime.Credential{}, err
	}
	return clientruntime.SnapshotCredential(credential), nil
}
