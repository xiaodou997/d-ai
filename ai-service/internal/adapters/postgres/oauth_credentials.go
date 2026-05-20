package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/secret"
)

// OAuthCredentialStore handles credential pool operations.
type OAuthCredentialStore struct {
	pool      *pgxpool.Pool
	masterKey string
}

func NewOAuthCredentialStore(pool *pgxpool.Pool, masterKey string) *OAuthCredentialStore {
	return &OAuthCredentialStore{pool: pool, masterKey: masterKey}
}

// ============================================================================
// CredentialPool CRUD
// ============================================================================

// CredentialPoolInput is used to create or update a pool.
type CredentialPoolInput struct {
	Name              string
	FixedProviderType string
	OAuthStrategy     string
	Notes             string
	Status            string
}

// CreatePool inserts a new credential pool and returns its ID.
func (s *OAuthCredentialStore) CreatePool(ctx context.Context, in CredentialPoolInput) (string, error) {
	strategy := in.OAuthStrategy
	if strategy == "" {
		strategy = "round_robin"
	}
	status := in.Status
	if status == "" {
		status = "active"
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ai_credential_pools (name, fixed_provider_type, oauth_strategy, notes, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text`,
		in.Name, in.FixedProviderType, strategy, in.Notes, status,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create credential pool: %w", err)
	}
	return id, nil
}

// GetPool fetches a single pool by ID.
func (s *OAuthCredentialStore) GetPool(ctx context.Context, poolID string) (*domain.CredentialPool, error) {
	const q = `
		SELECT id::text, name, fixed_provider_type, oauth_strategy,
		       COALESCE(notes,''), status, created_at, updated_at
		FROM ai_credential_pools WHERE id = $1`
	var p domain.CredentialPool
	err := s.pool.QueryRow(ctx, q, poolID).Scan(
		&p.ID, &p.Name, &p.FixedProviderType, &p.OAuthStrategy,
		&p.Notes, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPools returns all pools ordered by fixed_provider_type, then name.
func (s *OAuthCredentialStore) ListPools(ctx context.Context) ([]domain.CredentialPool, error) {
	const q = `
		SELECT id::text, name, fixed_provider_type, oauth_strategy,
		       COALESCE(notes,''), status, created_at, updated_at
		FROM ai_credential_pools
		ORDER BY fixed_provider_type, name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list pools: %w", err)
	}
	defer rows.Close()

	var out []domain.CredentialPool
	for rows.Next() {
		var p domain.CredentialPool
		if err := rows.Scan(
			&p.ID, &p.Name, &p.FixedProviderType, &p.OAuthStrategy,
			&p.Notes, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pool: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdatePool updates mutable pool fields.
func (s *OAuthCredentialStore) UpdatePool(ctx context.Context, poolID string, in CredentialPoolInput) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ai_credential_pools
		SET name = $2, oauth_strategy = $3, notes = $4, status = $5, updated_at = now()
		WHERE id = $1`,
		poolID, in.Name, in.OAuthStrategy, in.Notes, in.Status,
	)
	return err
}

// DeletePool removes a pool (cascades to credentials).
func (s *OAuthCredentialStore) DeletePool(ctx context.Context, poolID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ai_credential_pools WHERE id = $1`, poolID)
	return err
}

// ============================================================================
// Credential selection (serving path)
// ============================================================================

// OAuthCredentialRow is the raw DB row (ciphertexts, not decrypted).
type OAuthCredentialRow struct {
	ID                     string
	PoolID                 string
	Name                   string
	ProviderType           string
	Email                  string
	AccessTokenCiphertext  string
	RefreshTokenCiphertext string
	TokenType              string
	Scope                  string
	ExpiresAt              *time.Time
	AuthMetadataRaw        []byte
	Weight                 int
	Status                 string
	InvalidReason          string
	LastUsedAt             *time.Time
	LastRefreshedAt        *time.Time
	LastFailedAt           *time.Time
	ConsecutiveFailCount   int
	SuccessCount           int64
	FailCount              int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// SelectCredentialFromPool picks one active credential from the pool.
// strategy: "round_robin" picks the least-recently-used; "weighted" picks by weight.
// Returns domain.OAuthCredential with decrypted tokens.
func (s *OAuthCredentialStore) SelectCredentialFromPool(
	ctx context.Context,
	poolID string,
	strategy string,
) (*domain.OAuthCredential, error) {
	if strategy == "weighted" {
		return s.selectWeighted(ctx, poolID)
	}
	return s.selectRoundRobin(ctx, poolID)
}

// selectRoundRobin atomically picks the least-recently-used credential and
// updates last_used_at in a single statement, preventing concurrent requests
// from selecting the same credential.
func (s *OAuthCredentialStore) selectRoundRobin(ctx context.Context, poolID string) (*domain.OAuthCredential, error) {
	const q = `
		UPDATE ai_provider_oauth_credentials
		SET last_used_at = now(), updated_at = now()
		WHERE id = (
			SELECT id
			FROM ai_provider_oauth_credentials
			WHERE pool_id = $1 AND status = 'active'
			ORDER BY last_used_at ASC NULLS FIRST
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, pool_id, name, provider_type, email,
		          access_token_ciphertext, refresh_token_ciphertext,
		          token_type, scope, expires_at, auth_metadata,
		          weight, status, invalid_reason,
		          last_used_at, last_refreshed_at, last_failed_at,
		          consecutive_fail_count, success_count, fail_count,
		          created_at, updated_at`

	pgRows, err := s.pool.Query(ctx, q, poolID)
	if err != nil {
		return nil, fmt.Errorf("select round-robin credential: %w", err)
	}
	rows, err := pgx.CollectRows(pgRows, pgx.RowToStructByPos[OAuthCredentialRow])
	if err != nil {
		return nil, fmt.Errorf("scan round-robin credential: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no active oauth credentials for pool %s", poolID)
	}
	return s.decryptRow(rows[0])
}

func (s *OAuthCredentialStore) selectWeighted(ctx context.Context, poolID string) (*domain.OAuthCredential, error) {
	rows, err := s.listActiveWeighted(ctx, poolID)
	if err != nil {
		return nil, fmt.Errorf("list oauth credentials: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no active oauth credentials for pool %s", poolID)
	}
	row := weightedSelectOAuth(rows)
	cred, err := s.decryptRow(row)
	if err != nil {
		return nil, err
	}
	// Record usage synchronously so that the next selection sees updated last_used_at.
	s.RecordUsed(ctx, cred.ID)
	return cred, nil
}

func (s *OAuthCredentialStore) listActiveWeighted(ctx context.Context, poolID string) ([]OAuthCredentialRow, error) {
	const q = `
		SELECT id, pool_id, name, provider_type, email,
		       access_token_ciphertext, refresh_token_ciphertext,
		       token_type, scope, expires_at, auth_metadata,
		       weight, status, invalid_reason,
		       last_used_at, last_refreshed_at, last_failed_at,
		       consecutive_fail_count, success_count, fail_count,
		       created_at, updated_at
		FROM ai_provider_oauth_credentials
		WHERE pool_id = $1 AND status = 'active'
		ORDER BY weight DESC`
	return s.scanRows(ctx, q, poolID)
}

func weightedSelectOAuth(rows []OAuthCredentialRow) OAuthCredentialRow {
	total := 0
	for _, r := range rows {
		total += r.Weight
	}
	if total == 0 {
		return rows[0]
	}
	n := rand.Intn(total)
	acc := 0
	for _, r := range rows {
		acc += r.Weight
		if n < acc {
			return r
		}
	}
	return rows[len(rows)-1]
}

// ============================================================================
// Usage tracking (serving path)
// ============================================================================

func (s *OAuthCredentialStore) RecordUsed(ctx context.Context, credID string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE ai_provider_oauth_credentials SET last_used_at = now(), updated_at = now() WHERE id = $1`,
		credID,
	)
}

func (s *OAuthCredentialStore) RecordSuccess(ctx context.Context, credID string) {
	_, _ = s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET success_count = success_count + 1,
		    consecutive_fail_count = 0,
		    updated_at = now()
		WHERE id = $1`, credID)
}

func (s *OAuthCredentialStore) RecordFailure(ctx context.Context, credID string, reason string) {
	_, _ = s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET fail_count             = fail_count + 1,
		    consecutive_fail_count = consecutive_fail_count + 1,
		    last_failed_at         = now(),
		    updated_at             = now(),
		    status       = CASE WHEN consecutive_fail_count + 1 >= 3 AND status = 'active'
		                        THEN 'invalid' ELSE status END,
		    invalid_reason = CASE WHEN consecutive_fail_count + 1 >= 3 AND status = 'active'
		                          THEN $2 ELSE invalid_reason END
		WHERE id = $1`, credID, reason)
}

func (s *OAuthCredentialStore) MarkInvalid(ctx context.Context, credID string, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET status = 'invalid', invalid_reason = $2, updated_at = now()
		WHERE id = $1`, credID, reason)
	return err
}

func (s *OAuthCredentialStore) UpdateTokens(
	ctx context.Context,
	credID string,
	accessToken string,
	refreshToken string,
	expiresAt *time.Time,
) error {
	atCipher, err := secret.EncryptProviderKey(s.masterKey, accessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}

	var rtCipher pgtype.Text
	if refreshToken != "" {
		enc, err := secret.EncryptProviderKey(s.masterKey, refreshToken)
		if err != nil {
			return fmt.Errorf("encrypt refresh token: %w", err)
		}
		rtCipher = pgtype.Text{String: enc, Valid: true}
	}

	var pgExpiry pgtype.Timestamptz
	if expiresAt != nil {
		pgExpiry = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}

	// When refreshToken is empty, preserve the existing refresh_token_ciphertext
	// rather than overwriting it with NULL.
	_, err = s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET access_token_ciphertext  = $2,
		    refresh_token_ciphertext = CASE WHEN $3::text IS NOT NULL THEN $3 ELSE refresh_token_ciphertext END,
		    expires_at               = $4,
		    last_refreshed_at        = now(),
		    status                   = 'active',
		    invalid_reason           = NULL,
		    consecutive_fail_count   = 0,
		    updated_at               = now()
		WHERE id = $1`,
		credID, atCipher, rtCipher, pgExpiry,
	)
	return err
}

// ListExpiring returns active credentials expiring within the given duration.
func (s *OAuthCredentialStore) ListExpiring(ctx context.Context, within time.Duration) ([]OAuthCredentialRow, error) {
	const q = `
		SELECT id, pool_id, name, provider_type, email,
		       access_token_ciphertext, refresh_token_ciphertext,
		       token_type, scope, expires_at, auth_metadata,
		       weight, status, invalid_reason,
		       last_used_at, last_refreshed_at, last_failed_at,
		       consecutive_fail_count, success_count, fail_count,
		       created_at, updated_at
		FROM ai_provider_oauth_credentials
		WHERE status = 'active'
		  AND refresh_token_ciphertext IS NOT NULL
		  AND (expires_at IS NULL OR expires_at < now() + $1::interval)
		ORDER BY expires_at ASC NULLS FIRST`
	interval := pgtype.Interval{Microseconds: int64(within / time.Microsecond), Valid: true}
	return s.scanRows(ctx, q, interval)
}

// ============================================================================
// Pool health summary (admin dashboard)
// ============================================================================

// PoolHealthRow is one pool's credential health summary.
type PoolHealthRow struct {
	PoolID            string
	PoolName          string
	FixedProviderType string
	OAuthStrategy     string
	Total             int
	Active            int
	Invalid           int
	Disabled          int
	ExpiringSoon      int
}

func (s *OAuthCredentialStore) GetPoolHealthSummary(ctx context.Context) ([]PoolHealthRow, error) {
	const q = `
		SELECT
		    p.id::text                AS pool_id,
		    p.name                   AS pool_name,
		    p.fixed_provider_type,
		    p.oauth_strategy,
		    COUNT(*)::int             AS total,
		    COUNT(*) FILTER (WHERE c.status = 'active')::int   AS active,
		    COUNT(*) FILTER (WHERE c.status = 'invalid')::int  AS invalid,
		    COUNT(*) FILTER (WHERE c.status = 'disabled')::int AS disabled,
		    COUNT(*) FILTER (
		        WHERE c.status = 'active'
		          AND c.expires_at IS NOT NULL
		          AND c.expires_at < now() + INTERVAL '1 hour'
		    )::int AS expiring_soon
		FROM ai_credential_pools p
		JOIN ai_provider_oauth_credentials c ON c.pool_id = p.id
		GROUP BY p.id, p.name, p.fixed_provider_type, p.oauth_strategy
		ORDER BY p.fixed_provider_type, p.name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query pool health: %w", err)
	}
	defer rows.Close()

	var out []PoolHealthRow
	for rows.Next() {
		var r PoolHealthRow
		if err := rows.Scan(
			&r.PoolID, &r.PoolName, &r.FixedProviderType, &r.OAuthStrategy,
			&r.Total, &r.Active, &r.Invalid, &r.Disabled, &r.ExpiringSoon,
		); err != nil {
			return nil, fmt.Errorf("scan pool health row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ============================================================================
// Admin CRUD (credentials within a pool)
// ============================================================================

// OAuthCredentialInput is used to import a credential into a pool.
type OAuthCredentialInput struct {
	Name         string
	ProviderType string
	Email        string
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresAt    *time.Time
	AuthMetadata map[string]any
	Weight       int
}

// Create inserts a new credential into the pool.
func (s *OAuthCredentialStore) Create(ctx context.Context, poolID string, in OAuthCredentialInput) (string, error) {
	atCipher, err := secret.EncryptProviderKey(s.masterKey, in.AccessToken)
	if err != nil {
		return "", fmt.Errorf("encrypt access token: %w", err)
	}

	var rtCipher pgtype.Text
	if in.RefreshToken != "" {
		enc, err := secret.EncryptProviderKey(s.masterKey, in.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("encrypt refresh token: %w", err)
		}
		rtCipher = pgtype.Text{String: enc, Valid: true}
	}

	metaRaw, _ := json.Marshal(in.AuthMetadata)

	var pgExpiry pgtype.Timestamptz
	if in.ExpiresAt != nil {
		pgExpiry = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}

	weight := in.Weight
	if weight <= 0 {
		weight = 100
	}
	tokenType := in.TokenType
	if tokenType == "" {
		tokenType = "bearer"
	}

	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO ai_provider_oauth_credentials (
			pool_id, name, provider_type, email,
			access_token_ciphertext, refresh_token_ciphertext,
			token_type, scope, expires_at, auth_metadata, weight
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id::text`,
		poolID, in.Name, in.ProviderType, in.Email,
		atCipher, rtCipher,
		tokenType, in.Scope, pgExpiry, metaRaw, weight,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert oauth credential: %w", err)
	}
	return id, nil
}

// ListForPool returns all credentials for a pool (raw, encrypted).
func (s *OAuthCredentialStore) ListForPool(ctx context.Context, poolID string) ([]OAuthCredentialRow, error) {
	const q = `
		SELECT id, pool_id, name, provider_type, email,
		       access_token_ciphertext, refresh_token_ciphertext,
		       token_type, scope, expires_at, auth_metadata,
		       weight, status, invalid_reason,
		       last_used_at, last_refreshed_at, last_failed_at,
		       consecutive_fail_count, success_count, fail_count,
		       created_at, updated_at
		FROM ai_provider_oauth_credentials
		WHERE pool_id = $1
		ORDER BY created_at ASC`
	return s.scanRows(ctx, q, poolID)
}

func (s *OAuthCredentialStore) UpdateStatus(ctx context.Context, credID string, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ai_provider_oauth_credentials SET status = $2, updated_at = now() WHERE id = $1`,
		credID, status)
	return err
}

func (s *OAuthCredentialStore) UpdateWeight(ctx context.Context, credID string, weight int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ai_provider_oauth_credentials SET weight = $2, updated_at = now() WHERE id = $1`,
		credID, weight)
	return err
}

func (s *OAuthCredentialStore) Delete(ctx context.Context, credID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM ai_provider_oauth_credentials WHERE id = $1`, credID)
	return err
}

func (s *OAuthCredentialStore) GetByID(ctx context.Context, credID string) (*OAuthCredentialRow, error) {
	const q = `
		SELECT id, pool_id, name, provider_type, email,
		       access_token_ciphertext, refresh_token_ciphertext,
		       token_type, scope, expires_at, auth_metadata,
		       weight, status, invalid_reason,
		       last_used_at, last_refreshed_at, last_failed_at,
		       consecutive_fail_count, success_count, fail_count,
		       created_at, updated_at
		FROM ai_provider_oauth_credentials
		WHERE id = $1`
	rows, err := s.scanRows(ctx, q, credID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, pgx.ErrNoRows
	}
	return &rows[0], nil
}

func (s *OAuthCredentialStore) GetDecryptedByID(ctx context.Context, credID string) (*domain.OAuthCredential, error) {
	row, err := s.GetByID(ctx, credID)
	if err != nil {
		return nil, err
	}
	return s.decryptRow(*row)
}

// ============================================================================
// Helpers
// ============================================================================

func (s *OAuthCredentialStore) decryptRow(row OAuthCredentialRow) (*domain.OAuthCredential, error) {
	at, err := secret.DecryptProviderKey(s.masterKey, row.AccessTokenCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt access token for cred %s: %w", row.ID, err)
	}
	var rt string
	if row.RefreshTokenCiphertext != "" {
		rt, err = secret.DecryptProviderKey(s.masterKey, row.RefreshTokenCiphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypt refresh token for cred %s: %w", row.ID, err)
		}
	}

	var meta map[string]any
	if len(row.AuthMetadataRaw) > 0 {
		_ = json.Unmarshal(row.AuthMetadataRaw, &meta)
	}

	return &domain.OAuthCredential{
		ID:           row.ID,
		PoolID:       row.PoolID,
		Name:         row.Name,
		ProviderType: domain.FixedProviderType(row.ProviderType),
		Email:        row.Email,
		AccessToken:  at,
		RefreshToken: rt,
		TokenType:    row.TokenType,
		ExpiresAt:    row.ExpiresAt,
		AuthMetadata: meta,
		Weight:       row.Weight,
		Status:       domain.OAuthCredentialStatus(row.Status),
	}, nil
}

func (s *OAuthCredentialStore) scanRows(ctx context.Context, query string, args ...any) ([]OAuthCredentialRow, error) {
	pgRows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	var out []OAuthCredentialRow
	for pgRows.Next() {
		var r OAuthCredentialRow
		var pgExpiresAt pgtype.Timestamptz
		var pgLastUsed, pgLastRefreshed, pgLastFailed pgtype.Timestamptz
		var pgInvalidReason pgtype.Text
		err := pgRows.Scan(
			&r.ID, &r.PoolID, &r.Name, &r.ProviderType, &r.Email,
			&r.AccessTokenCiphertext, &r.RefreshTokenCiphertext,
			&r.TokenType, &r.Scope, &pgExpiresAt, &r.AuthMetadataRaw,
			&r.Weight, &r.Status, &pgInvalidReason,
			&pgLastUsed, &pgLastRefreshed, &pgLastFailed,
			&r.ConsecutiveFailCount, &r.SuccessCount, &r.FailCount,
			&r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan oauth credential row: %w", err)
		}
		if pgInvalidReason.Valid {
			r.InvalidReason = pgInvalidReason.String
		}
		if pgExpiresAt.Valid {
			r.ExpiresAt = &pgExpiresAt.Time
		}
		if pgLastUsed.Valid {
			r.LastUsedAt = &pgLastUsed.Time
		}
		if pgLastRefreshed.Valid {
			r.LastRefreshedAt = &pgLastRefreshed.Time
		}
		if pgLastFailed.Valid {
			r.LastFailedAt = &pgLastFailed.Time
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}
