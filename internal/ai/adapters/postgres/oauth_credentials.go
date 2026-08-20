package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/secret"
	"xiaodou/dai/internal/clientsecret"
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
	TenantDisplayName string
	TenantAccessMode  string
	FixedProviderType string
	OAuthStrategy     string
	Notes             string
	Status            string
	PriceBookID       string
	TenantMultiplier  *float64
}

// CreatePool inserts a new credential pool and returns its ID.
func (s *OAuthCredentialStore) CreatePool(ctx context.Context, in CredentialPoolInput) (string, error) {
	strategy := in.OAuthStrategy
	if strategy == "" {
		strategy = "round_robin"
	}
	status := in.Status
	if status == "" {
		status = "disabled"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := requirePlatformPriceBook(ctx, tx, in.PriceBookID); err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_credential_pools (
			name, tenant_display_name, tenant_access_mode, fixed_provider_type, oauth_strategy, notes, status,
			price_book_id, tenant_multiplier)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id::text`,
		in.Name, in.TenantDisplayName, in.TenantAccessMode, in.FixedProviderType, strategy, in.Notes, status,
		nullableUUID(in.PriceBookID), floatPtrToNumeric(in.TenantMultiplier),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create credential pool: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// credentialPoolColumns is the shared projection for GetPool/ListPools, ordered
// to match scanCredentialPool.
const credentialPoolColumns = `
	id::text, name, tenant_display_name, tenant_access_mode, fixed_provider_type, oauth_strategy,
		COALESCE(notes,''), status,
		price_book_id::text, COALESCE(tenant_multiplier, 1),
	created_at, updated_at`

// scanCredentialPool scans one row (the credentialPoolColumns projection) into a pool.
func scanCredentialPool(scan func(dest ...any) error) (domain.CredentialPool, error) {
	var p domain.CredentialPool
	var priceBookID pgtype.Text
	if err := scan(
		&p.ID, &p.Name, &p.TenantDisplayName, &p.TenantAccessMode, &p.FixedProviderType, &p.OAuthStrategy,
		&p.Notes, &p.Status,
		&priceBookID, &p.TenantMultiplier,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return domain.CredentialPool{}, err
	}
	if priceBookID.Valid {
		p.PriceBookID = priceBookID.String
	}
	return p, nil
}

// GetPool fetches a single pool by ID.
func (s *OAuthCredentialStore) GetPool(ctx context.Context, poolID string) (*domain.CredentialPool, error) {
	q := `SELECT ` + credentialPoolColumns + ` FROM ai_credential_pools WHERE id = $1`
	p, err := scanCredentialPool(s.pool.QueryRow(ctx, q, poolID).Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// ListPools returns all pools ordered by fixed_provider_type, then name.
func (s *OAuthCredentialStore) ListPools(ctx context.Context) ([]domain.CredentialPool, error) {
	q := `SELECT ` + credentialPoolColumns + ` FROM ai_credential_pools ORDER BY fixed_provider_type, name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list pools: %w", err)
	}
	defer rows.Close()

	var out []domain.CredentialPool
	for rows.Next() {
		p, err := scanCredentialPool(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan pool: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdatePool updates mutable pool fields.
func (s *OAuthCredentialStore) UpdatePool(ctx context.Context, poolID string, in CredentialPoolInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := requirePlatformPriceBook(ctx, tx, in.PriceBookID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE ai_credential_pools
		SET name = $2, tenant_display_name = $3, tenant_access_mode = $4,
		    oauth_strategy = $5, notes = $6, status = $7,
		    price_book_id = $8, tenant_multiplier = $9,
		    updated_at = now()
		WHERE id = $1`,
		poolID, in.Name, in.TenantDisplayName, in.TenantAccessMode, in.OAuthStrategy, in.Notes, in.Status,
		nullableUUID(in.PriceBookID), floatPtrToNumeric(in.TenantMultiplier),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if in.TenantAccessMode == "public" {
		if _, err := tx.Exec(ctx, `
			UPDATE ai_upstream_resource_tenant_policies
			SET access_granted = false, updated_at = now()
			WHERE resource_kind = 'oauth_pool' AND resource_id = $1
		`, poolID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return err
}

func (s *OAuthCredentialStore) UpdatePoolStatus(ctx context.Context, poolID, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_credential_pools
		SET status = $2, updated_at = now()
		WHERE id = $1
	`, poolID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func requirePlatformPriceBook(ctx context.Context, tx pgx.Tx, priceBookID string) error {
	if priceBookID == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM ai_price_books WHERE id = $1::uuid AND owner_type = 'platform' AND status = 'active')
	`, priceBookID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.NewValidationError("price_book_id", "active platform price book is required")
	}
	return nil
}

// DeletePool removes a pool (cascades to credentials).
func (s *OAuthCredentialStore) DeletePool(ctx context.Context, poolID string) error {
	refs, err := countOne(ctx, s.pool, `
		SELECT
			(SELECT COUNT(*) FROM ai_provider_oauth_credentials WHERE pool_id = $1) +
			(SELECT COUNT(*) FROM ai_group_targets WHERE target_kind = 'oauth_pool' AND target_id = $1)
	`, poolID)
	if err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("credential pool is still referenced by %d resource(s), clear credentials/bindings before deleting: %w", refs, domain.ErrConflict)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_upstream_resource_tenant_policies
		WHERE resource_kind = 'oauth_pool' AND resource_id = $1
	`, poolID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ai_credential_pools WHERE id = $1`, poolID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
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
	TokenVersion           int64
	AuthMetadataRaw        []byte
	Weight                 int
	Status                 string
	InvalidReason          string
	CooldownUntil          *time.Time
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
			  AND (cooldown_until IS NULL OR cooldown_until <= now())
			ORDER BY last_used_at ASC NULLS FIRST
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, pool_id, name, provider_type, email,
		          access_token_ciphertext, refresh_token_ciphertext,
		          token_type, scope, expires_at, token_version, auth_metadata,
		          weight, status, invalid_reason, cooldown_until,
		          last_used_at, last_refreshed_at, last_failed_at,
		          consecutive_fail_count, success_count, fail_count,
		          created_at, updated_at`

	// Same column list as every other credential read, so it goes through
	// scanRows too: positional struct mapping cannot handle the nullable TEXT
	// columns that the row struct models as plain strings.
	rows, err := s.scanRows(ctx, q, poolID)
	if err != nil {
		return nil, fmt.Errorf("select round-robin credential: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no active oauth credentials for pool %s", poolID)
	}
	return s.decryptRow(ctx, rows[0])
}

// SelectPinnedCredential returns one specific credential of a pool, used by
// sticky routing to keep a conversation on the same upstream account. It
// touches last_used_at like the strategy-based selectors so pinned traffic
// still counts as usage for round-robin ordering. Errors when the credential
// left the pool or is no longer active, letting the caller fail over.
func (s *OAuthCredentialStore) SelectPinnedCredential(
	ctx context.Context,
	poolID string,
	credID string,
) (*domain.OAuthCredential, error) {
	const q = `
		UPDATE ai_provider_oauth_credentials
		SET last_used_at = now(), updated_at = now()
		WHERE id = $1 AND pool_id = $2 AND status = 'active'
		  AND (cooldown_until IS NULL OR cooldown_until <= now())
		RETURNING id, pool_id, name, provider_type, email,
		          access_token_ciphertext, refresh_token_ciphertext,
		          token_type, scope, expires_at, token_version, auth_metadata,
		          weight, status, invalid_reason, cooldown_until,
		          last_used_at, last_refreshed_at, last_failed_at,
		          consecutive_fail_count, success_count, fail_count,
		          created_at, updated_at`
	rows, err := s.scanRows(ctx, q, credID, poolID)
	if err != nil {
		return nil, fmt.Errorf("select pinned credential: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("credential %s is not an active member of pool %s", credID, poolID)
	}
	return s.decryptRow(ctx, rows[0])
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
	cred, err := s.decryptRow(ctx, row)
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
		       token_type, scope, expires_at, token_version, auth_metadata,
		       weight, status, invalid_reason, cooldown_until,
		       last_used_at, last_refreshed_at, last_failed_at,
		       consecutive_fail_count, success_count, fail_count,
		       created_at, updated_at
		FROM ai_provider_oauth_credentials
		WHERE pool_id = $1 AND status = 'active'
		  AND (cooldown_until IS NULL OR cooldown_until <= now())
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
		    cooldown_until = NULL,
		    updated_at = now()
		WHERE id = $1`, credID)
}

func (s *OAuthCredentialStore) MarkInvalid(ctx context.Context, credID string, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET status = 'invalid', invalid_reason = $2, updated_at = now()
		WHERE id = $1`, credID, reason)
	return err
}

func (s *OAuthCredentialStore) MarkCooldown(ctx context.Context, credID string, until time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET cooldown_until = GREATEST(COALESCE(cooldown_until, $2), $2),
		    last_failed_at = now(),
		    consecutive_fail_count = consecutive_fail_count + 1,
		    fail_count = fail_count + 1,
		    updated_at = now()
		WHERE id = $1 AND status = 'active'`,
		credID, until.UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *OAuthCredentialStore) UpdateTokens(
	ctx context.Context,
	credID string,
	accessToken string,
	refreshToken string,
	expiresAt *time.Time,
	expectedVersion int64,
) (int64, error) {
	atCipher, err := secret.EncryptProviderKey(s.masterKey, accessToken)
	if err != nil {
		return 0, fmt.Errorf("encrypt access token: %w", err)
	}

	var rtCipher pgtype.Text
	if refreshToken != "" {
		enc, err := secret.EncryptProviderKey(s.masterKey, refreshToken)
		if err != nil {
			return 0, fmt.Errorf("encrypt refresh token: %w", err)
		}
		rtCipher = pgtype.Text{String: enc, Valid: true}
	}

	var pgExpiry pgtype.Timestamptz
	if expiresAt != nil {
		pgExpiry = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}

	// When refreshToken is empty, preserve the existing refresh_token_ciphertext
	// rather than overwriting it with NULL.
	var nextVersion int64
	err = s.pool.QueryRow(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET access_token_ciphertext  = $2,
		    refresh_token_ciphertext = CASE WHEN $3::text IS NOT NULL THEN $3 ELSE refresh_token_ciphertext END,
		    expires_at               = $4,
		    token_version            = token_version + 1,
		    last_refreshed_at        = now(),
		    status                   = 'active',
		    invalid_reason           = NULL,
		    cooldown_until           = NULL,
		    consecutive_fail_count   = 0,
		    updated_at               = now()
		WHERE id = $1 AND token_version = $5
		RETURNING token_version`,
		credID, atCipher, rtCipher, pgExpiry, expectedVersion,
	).Scan(&nextVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("credential token version changed: %w", domain.ErrConflict)
	}
	if err != nil {
		return 0, err
	}
	return nextVersion, nil
}

// ListExpiring returns active credentials expiring within the given duration.
func (s *OAuthCredentialStore) ListExpiring(ctx context.Context, within time.Duration) ([]OAuthCredentialRow, error) {
	const q = `
		SELECT id, pool_id, name, provider_type, email,
		       access_token_ciphertext, refresh_token_ciphertext,
		       token_type, scope, expires_at, token_version, auth_metadata,
		       weight, status, invalid_reason, cooldown_until,
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
	CoolingDown       int
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
		        WHERE c.status = 'active' AND c.cooldown_until > now()
		    )::int AS cooling_down,
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
			&r.Total, &r.Active, &r.Invalid, &r.Disabled, &r.CoolingDown, &r.ExpiringSoon,
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
		       token_type, scope, expires_at, token_version, auth_metadata,
		       weight, status, invalid_reason, cooldown_until,
		       last_used_at, last_refreshed_at, last_failed_at,
		       consecutive_fail_count, success_count, fail_count,
		       created_at, updated_at
		FROM ai_provider_oauth_credentials
		WHERE pool_id = $1
		ORDER BY created_at ASC`
	return s.scanRows(ctx, q, poolID)
}

func (s *OAuthCredentialStore) UpdateStatus(ctx context.Context, credID string, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE ai_provider_oauth_credentials
		 SET status = $2,
		     cooldown_until = CASE WHEN $2 = 'active' THEN NULL ELSE cooldown_until END,
		     updated_at = now()
		 WHERE id = $1`,
		credID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *OAuthCredentialStore) UpdateWeight(ctx context.Context, credID string, weight int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE ai_provider_oauth_credentials SET weight = $2, updated_at = now() WHERE id = $1`,
		credID, weight)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *OAuthCredentialStore) Delete(ctx context.Context, credID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM ai_provider_oauth_credentials WHERE id = $1`, credID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *OAuthCredentialStore) GetByID(ctx context.Context, credID string) (*OAuthCredentialRow, error) {
	const q = `
		SELECT id, pool_id, name, provider_type, email,
		       access_token_ciphertext, refresh_token_ciphertext,
		       token_type, scope, expires_at, token_version, auth_metadata,
		       weight, status, invalid_reason, cooldown_until,
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
	return s.decryptRow(ctx, *row)
}

// ============================================================================
// Helpers
// ============================================================================

func (s *OAuthCredentialStore) decryptRow(ctx context.Context, row OAuthCredentialRow) (*domain.OAuthCredential, error) {
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
	s.maybeReencrypt(ctx, row, at, rt)

	var meta map[string]any
	if len(row.AuthMetadataRaw) > 0 {
		_ = json.Unmarshal(row.AuthMetadataRaw, &meta)
	}

	return &domain.OAuthCredential{
		ID:            row.ID,
		PoolID:        row.PoolID,
		Name:          row.Name,
		ProviderType:  domain.FixedProviderType(row.ProviderType),
		Email:         row.Email,
		AccessToken:   at,
		RefreshToken:  rt,
		TokenType:     row.TokenType,
		ExpiresAt:     row.ExpiresAt,
		TokenVersion:  row.TokenVersion,
		AuthMetadata:  meta,
		Weight:        row.Weight,
		Status:        domain.OAuthCredentialStatus(row.Status),
		CooldownUntil: row.CooldownUntil,
	}, nil
}

// maybeReencrypt upgrades legacy or previous-key ciphertext on the serving
// path. The update intentionally does not bump token_version: it changes only
// the at-rest representation and must not invalidate an in-flight refresh.
func (s *OAuthCredentialStore) maybeReencrypt(ctx context.Context, row OAuthCredentialRow, accessToken, refreshToken string) {
	if !clientsecret.IsConfigured() {
		return
	}
	accessCipher := row.AccessTokenCiphertext
	refreshCipher := row.RefreshTokenCiphertext
	changed := false
	if clientsecret.NeedsReencrypt(accessCipher) {
		if encrypted, err := secret.EncryptProviderKey(s.masterKey, accessToken); err == nil {
			accessCipher, changed = encrypted, true
		}
	}
	if refreshCipher != "" && clientsecret.NeedsReencrypt(refreshCipher) {
		if encrypted, err := secret.EncryptProviderKey(s.masterKey, refreshToken); err == nil {
			refreshCipher, changed = encrypted, true
		}
	}
	if !changed {
		return
	}
	_, _ = s.pool.Exec(ctx, `
		UPDATE ai_provider_oauth_credentials
		SET access_token_ciphertext = $2,
		    refresh_token_ciphertext = NULLIF($3, ''),
		    updated_at = now()
		WHERE id = $1
	`, row.ID, accessCipher, refreshCipher)
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
		var pgCooldown, pgLastUsed, pgLastRefreshed, pgLastFailed pgtype.Timestamptz
		// Every nullable TEXT column goes through pgtype.Text: the row struct
		// models "absent" as the empty string, and pgx cannot scan NULL into a
		// plain string. Create writes a NULL refresh token whenever a credential
		// has none, so scanning it directly fails for those rows.
		var pgInvalidReason, pgRefreshToken, pgEmail, pgScope pgtype.Text
		err := pgRows.Scan(
			&r.ID, &r.PoolID, &r.Name, &r.ProviderType, &pgEmail,
			&r.AccessTokenCiphertext, &pgRefreshToken,
			&r.TokenType, &pgScope, &pgExpiresAt, &r.TokenVersion, &r.AuthMetadataRaw,
			&r.Weight, &r.Status, &pgInvalidReason, &pgCooldown,
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
		if pgRefreshToken.Valid {
			r.RefreshTokenCiphertext = pgRefreshToken.String
		}
		if pgEmail.Valid {
			r.Email = pgEmail.String
		}
		if pgScope.Valid {
			r.Scope = pgScope.String
		}
		if pgExpiresAt.Valid {
			r.ExpiresAt = &pgExpiresAt.Time
		}
		if pgCooldown.Valid {
			r.CooldownUntil = &pgCooldown.Time
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
