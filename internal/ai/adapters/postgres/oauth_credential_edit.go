package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/secret"
)

func (s *OAuthCredentialStore) rejectDuplicate(ctx context.Context, poolID, exceptID string, in domain.OAuthCredentialCreate) error {
	rows, err := s.listForPoolRows(ctx, poolID)
	if err != nil {
		return err
	}
	identity := domain.OAuthAccountIdentity(string(in.ProviderType), in.Email, in.AuthMetadata)
	for _, row := range rows {
		if row.ID == exceptID {
			continue
		}
		var metadata map[string]any
		_ = json.Unmarshal(row.AuthMetadataRaw, &metadata)
		other := domain.OAuthAccountIdentity(row.ProviderType, row.Email, metadata)
		duplicate := identity != "" && identity == other
		// Explicit, different workspace identities must remain separate even when
		// the exporting client shares an OAuth login token between workspaces.
		if !duplicate && (identity == "" || other == "") {
			credential, err := s.decryptRow(ctx, row)
			if err != nil {
				return err
			}
			duplicate = (in.AccessToken != "" && in.AccessToken == credential.AccessToken) || (in.RefreshToken != "" && in.RefreshToken == credential.RefreshToken)
		}
		if duplicate {
			return fmt.Errorf("account already exists in this pool; edit the existing account to update credentials: %w", domain.ErrConflict)
		}
	}
	return nil
}

// UpdateCredential saves metadata, status and optional replacement tokens in
// one transaction. Concurrent refresh results are fenced by token_version.
func (s *OAuthCredentialStore) UpdateCredential(ctx context.Context, credID string, in domain.OAuthCredentialUpdate) error {
	if in.Weight != nil && *in.Weight < 0 {
		return domain.NewValidationError("weight", "weight must be nonnegative")
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		return domain.NewValidationError("status", "status must be active or disabled")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var poolID string
	if err = tx.QueryRow(ctx, `SELECT pool_id::text FROM ai_provider_oauth_credentials WHERE id=$1`, credID).Scan(&poolID); err != nil {
		return err
	}
	// Pool lock order matches Create and avoids identity-edit/import races.
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_credential_pools WHERE id=$1 FOR UPDATE`, poolID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_provider_oauth_credentials WHERE id=$1 FOR UPDATE`, credID); err != nil {
		return err
	}
	scoped := *s
	scoped.pool = tx
	row, err := scoped.GetByID(ctx, credID)
	if err != nil {
		return err
	}
	metadata := map[string]any{}
	_ = json.Unmarshal(row.AuthMetadataRaw, &metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	previousIdentity := domain.OAuthAccountIdentity(row.ProviderType, row.Email, metadata)
	for key, value := range in.AuthMetadata {
		metadata[key] = value
	}
	email, name, status, weight := row.Email, row.Name, row.Status, row.Weight
	if in.Email != nil {
		email = strings.TrimSpace(*in.Email)
	}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return domain.NewValidationError("name", "name is required")
		}
	}
	if in.Status != nil {
		status = *in.Status
	}
	if in.Weight != nil {
		weight = *in.Weight
	}
	access, refresh := strings.TrimSpace(in.AccessToken), strings.TrimSpace(in.RefreshToken)
	tokensChanged := access != "" || refresh != ""
	identityChanged := previousIdentity != domain.OAuthAccountIdentity(row.ProviderType, email, metadata)
	if tokensChanged || identityChanged {
		if err = scoped.rejectDuplicate(ctx, poolID, credID, domain.OAuthCredentialCreate{ProviderType: domain.FixedProviderType(row.ProviderType), Email: email, AuthMetadata: metadata, AccessToken: access, RefreshToken: refresh}); err != nil {
			return err
		}
	}
	atCipher, rtCipher := row.AccessTokenCiphertext, row.RefreshTokenCiphertext
	if access != "" {
		atCipher, err = secret.EncryptProviderKey(s.masterKey, access)
		if err != nil {
			return err
		}
	}
	if refresh != "" {
		rtCipher, err = secret.EncryptProviderKey(s.masterKey, refresh)
		if err != nil {
			return err
		}
	}
	expiry := row.ExpiresAt
	if access != "" || in.ExpiresAt != nil {
		expiry = in.ExpiresAt
	}
	tokenType, scope := row.TokenType, row.Scope
	if in.TokenType != nil {
		tokenType = strings.TrimSpace(*in.TokenType)
	}
	if in.Scope != nil {
		scope = strings.TrimSpace(*in.Scope)
	}
	metadataRaw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	resetAuth := (tokensChanged || identityChanged) && status == "active"
	_, err = tx.Exec(ctx, `UPDATE ai_provider_oauth_credentials SET
  name=$2,email=$3,status=$4,weight=$5,access_token_ciphertext=$6,refresh_token_ciphertext=NULLIF($7,''),
  expires_at=$8,auth_metadata=$9,token_type=$10,scope=$11,
  token_version=token_version+CASE WHEN $12 THEN 1 ELSE 0 END,
  invalid_reason=CASE WHEN $13 THEN NULL ELSE invalid_reason END,
  cooldown_until=CASE WHEN $13 OR $14 THEN NULL ELSE cooldown_until END,
  updated_at=now() WHERE id=$1`, credID, name, email, status, weight, atCipher, rtCipher, expiry, metadataRaw, tokenType, scope, tokensChanged || identityChanged || in.Status != nil, resetAuth, in.Status != nil && status == "active")
	if err != nil {
		return err
	}
	if resetAuth || in.Status != nil && status == "active" {
		if err = scoped.resetCredentialAvailability(ctx, credID, in.Status == nil); err != nil {
			return err
		}
	}
	if identityChanged && s.availability != nil {
		if err = s.availability.ClearCredentialQuota(ctx, credID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ObserveCredentialQuota fences observations from credentials that were edited
// or refreshed while an upstream request was in flight.
func (s *OAuthCredentialStore) ObserveCredentialQuota(ctx context.Context, id string, version int64, quota domain.CredentialQuota) error {
	if s.availability == nil {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var currentVersion int64
	// Hold a shared row lock through the Redis write: an identity edit cannot
	// clear old quota and then be overtaken by a late observation from that ID.
	err = tx.QueryRow(ctx, `SELECT token_version FROM ai_provider_oauth_credentials WHERE id=$1 AND status='active' FOR SHARE`, id).Scan(&currentVersion)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if currentVersion != version {
		return nil
	}
	if err = s.availability.ObserveCredentialQuota(ctx, id, quota); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *OAuthCredentialStore) CredentialQuotaEligibility(ctx context.Context, id, model string) (bool, int64, error) {
	if s.availability == nil {
		return true, 0, nil
	}
	return s.availability.CredentialQuotaEligibility(ctx, id, model)
}

func (s *OAuthCredentialStore) ReadCredentialQuotas(ctx context.Context, ids []string) (map[string][]domain.CredentialQuota, error) {
	if s.availability == nil {
		return map[string][]domain.CredentialQuota{}, nil
	}
	return s.availability.CredentialQuotas(ctx, ids)
}
