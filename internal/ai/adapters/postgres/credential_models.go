package postgres

import (
	"context"

	"xiaodou/dai/internal/ai/domain"
)

// ListModelCredentials reads identities only. Discovery never selects or
// touches last_used_at, so management traffic cannot disturb pool rotation.
func (s *OAuthCredentialStore) ListModelCredentials(ctx context.Context, poolID string) ([]domain.CredentialModelIdentity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text,c.pool_id::text,c.provider_type,c.token_version
		FROM ai_provider_oauth_credentials c
		JOIN ai_credential_pools p ON p.id=c.pool_id AND p.status='active'
		WHERE c.pool_id=$1 AND c.status='active' AND c.access_token_ciphertext<>''
		ORDER BY c.id`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CredentialModelIdentity{}
	for rows.Next() {
		var ref domain.CredentialModelIdentity
		if err := rows.Scan(&ref.ID, &ref.PoolID, &ref.ProviderType, &ref.TokenVersion); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
