package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	ActivationPurposeAccount = "account_activation"
	ActivationPurposeReset   = "password_reset"
)

var (
	ErrInvalidActivationToken = errors.New("invalid activation token")
	ErrExpiredActivationToken = errors.New("activation token expired")
	ErrUsedActivationToken    = errors.New("activation token already used")
)

type ActivationCredential struct {
	Token        string
	TokenHash    []byte
	PasswordHash string
	ExpiresAt    time.Time
}

type ActivationResult struct {
	Token     string
	ExpiresIn int64
}

type ActivationService struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewActivationService(pool *pgxpool.Pool, ttl time.Duration) *ActivationService {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &ActivationService{pool: pool, ttl: ttl}
}

func (s *ActivationService) NewCredential() (ActivationCredential, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return ActivationCredential{}, fmt.Errorf("generate activation token: %w", err)
	}
	placeholderBytes := make([]byte, 32)
	if _, err := rand.Read(placeholderBytes); err != nil {
		return ActivationCredential{}, fmt.Errorf("generate pending credential: %w", err)
	}
	token := "dai_act_" + base64.RawURLEncoding.EncodeToString(tokenBytes)
	placeholder := base64.RawURLEncoding.EncodeToString(placeholderBytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(placeholder), bcrypt.DefaultCost)
	if err != nil {
		return ActivationCredential{}, fmt.Errorf("hash pending credential: %w", err)
	}
	sum := sha256.Sum256([]byte(token))
	return ActivationCredential{
		Token: token, TokenHash: sum[:], PasswordHash: string(hash),
		ExpiresAt: time.Now().UTC().Add(s.ttl),
	}, nil
}

func (s *ActivationService) Store(ctx context.Context, tx pgx.Tx, userID, purpose string, credential ActivationCredential) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_activation_tokens SET consumed_at = now()
		WHERE user_id = $1 AND consumed_at IS NULL
	`, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO auth_activation_tokens (token_hash, user_id, purpose, expires_at)
		VALUES ($1, $2, $3, $4)
	`, credential.TokenHash, userID, purpose, credential.ExpiresAt)
	return err
}

func (s *ActivationService) Reset(ctx context.Context, userID string) (ActivationResult, error) {
	credential, err := s.NewCredential()
	if err != nil {
		return ActivationResult{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// All activation mutations take the per-account lock before the account row
	// lock so reset and activate cannot deadlock each other.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return ActivationResult{}, err
	}
	result, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET password_hash = $1, credential_state = 'pending_activation',
		    credential_version = credential_version + 1, updated_at = now()
		WHERE user_id = $2 AND status <> 'deleted'
	`, credential.PasswordHash, userID)
	if err != nil {
		return ActivationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return ActivationResult{}, pgx.ErrNoRows
	}
	if err := s.Store(ctx, tx, userID, ActivationPurposeReset, credential); err != nil {
		return ActivationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ActivationResult{}, err
	}
	return activationResult(credential), nil
}

func (s *ActivationService) Activate(ctx context.Context, rawToken, password string) error {
	tokenHash := activationTokenHash(rawToken)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	if err := tx.QueryRow(ctx, `SELECT user_id FROM auth_activation_tokens WHERE token_hash = $1`, tokenHash).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidActivationToken
		}
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return err
	}
	var expiresAt time.Time
	var consumedAt *time.Time
	var username, accountStatus, credentialState string
	err = tx.QueryRow(ctx, `
		SELECT tok.expires_at, tok.consumed_at, a.username, a.status,
		       a.credential_state
		FROM auth_activation_tokens tok
		JOIN iam_accounts a ON a.user_id = tok.user_id
		WHERE tok.token_hash = $1
		FOR UPDATE OF tok, a
	`, tokenHash).Scan(&expiresAt, &consumedAt, &username, &accountStatus, &credentialState)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidActivationToken
	}
	if err != nil {
		return err
	}
	if consumedAt != nil {
		return ErrUsedActivationToken
	}
	if !time.Now().UTC().Before(expiresAt) {
		return ErrExpiredActivationToken
	}
	// Setting a credential does not enable the account. Disabled or inherited-
	// disabled accounts may activate, but login/session checks still reject them.
	if accountStatus == "deleted" || credentialState != "pending_activation" {
		return ErrInvalidActivationToken
	}
	if err := ValidatePassword(password, username); err != nil {
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET password_hash = $1, credential_state = 'active',
		    credential_version = credential_version + 1, updated_at = now()
		WHERE user_id = $2
	`, string(passwordHash), userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_activation_tokens SET consumed_at = now()
		WHERE user_id = $1 AND consumed_at IS NULL
	`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteExpired removes credentials that can no longer be used. Recently
// consumed rows are retained briefly so repeated submissions report "used".
func (s *ActivationService) DeleteExpired(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 5000
	}
	result, err := s.pool.Exec(ctx, `
		DELETE FROM auth_activation_tokens
		WHERE token_hash IN (
			SELECT token_hash FROM auth_activation_tokens
			WHERE expires_at < now() - interval '24 hours'
			   OR consumed_at < now() - interval '24 hours'
			ORDER BY created_at
			LIMIT $1
		)
	`, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func activationTokenHash(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func activationResult(credential ActivationCredential) ActivationResult {
	remaining := time.Until(credential.ExpiresAt)
	if remaining < 0 {
		remaining = 0
	}
	return ActivationResult{Token: credential.Token, ExpiresIn: int64(remaining.Seconds())}
}
