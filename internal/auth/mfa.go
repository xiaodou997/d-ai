package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/clientsecret"
)

var (
	ErrMFARequired       = errors.New("multi-factor authentication required")
	ErrInvalidMFACode    = errors.New("invalid multi-factor authentication code")
	ErrMFAUnavailable    = errors.New("multi-factor authentication is unavailable")
	ErrMFAAlreadyEnabled = errors.New("multi-factor authentication is already enabled")
)

type MFAService struct {
	pool         *pgxpool.Pool
	redis        *redis.Client
	challengeTTL time.Duration
}

var consumeMFAChallengeScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('DEL', KEYS[1], KEYS[2])
  return 1
end
return 0
`)

type MFAEnrollment struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauthUrl"`
}

type mfaChallenge struct {
	Principal Principal `json:"principal"`
	UserID    string    `json:"userId"`
}

func NewMFAService(pool *pgxpool.Pool, redisClient *redis.Client) *MFAService {
	return &MFAService{pool: pool, redis: redisClient, challengeTTL: 5 * time.Minute}
}

func (s *MFAService) Enabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := s.pool.QueryRow(ctx, `SELECT mfa_enabled FROM iam_accounts WHERE user_id = $1`, userID).Scan(&enabled)
	return enabled, err
}

func (s *MFAService) Enroll(ctx context.Context, userID, username string) (MFAEnrollment, error) {
	var enabled bool
	if err := s.pool.QueryRow(ctx, `SELECT mfa_enabled FROM iam_accounts WHERE user_id = $1 AND user_type IN (1, 2)`, userID).Scan(&enabled); err != nil {
		return MFAEnrollment{}, err
	}
	if enabled {
		return MFAEnrollment{}, ErrMFAAlreadyEnabled
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return MFAEnrollment{}, err
	}
	secret := strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
	encrypted, err := clientsecret.Encrypt(secret)
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("encrypt MFA secret: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE iam_accounts SET mfa_secret_encrypted = $1, mfa_enabled = FALSE, mfa_enrolled_at = now(), updated_at = now()
		WHERE user_id = $2 AND user_type IN (1, 2)
	`, encrypted, userID); err != nil {
		return MFAEnrollment{}, err
	}
	label := url.QueryEscape("D-AI:" + username)
	return MFAEnrollment{Secret: secret, OTPAuthURL: "otpauth://totp/" + label + "?secret=" + secret + "&issuer=D-AI"}, nil
}

func (s *MFAService) ConfirmEnrollment(ctx context.Context, userID, code string) error {
	if !s.VerifyCode(ctx, userID, code) {
		return ErrInvalidMFACode
	}
	_, err := s.pool.Exec(ctx, `UPDATE iam_accounts SET mfa_enabled = TRUE, updated_at = now() WHERE user_id = $1 AND user_type IN (1, 2)`, userID)
	return err
}

func (s *MFAService) VerifyCode(ctx context.Context, userID, code string) bool {
	secret, err := s.secret(ctx, userID)
	if err != nil {
		return false
	}
	return VerifyTOTP(secret, code, time.Now().UTC())
}

func (s *MFAService) CreateChallenge(ctx context.Context, principal Principal) (string, error) {
	if s.redis == nil {
		return "", ErrMFAUnavailable
	}
	secret, err := s.secret(ctx, principal.UserID)
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", ErrMFAUnavailable
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "dai_mfa_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	payload, err := json.Marshal(mfaChallenge{Principal: principal, UserID: principal.UserID})
	if err != nil {
		return "", err
	}
	key := mfaChallengeKey(token)
	if err := s.redis.Set(ctx, key, payload, s.challengeTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *MFAService) VerifyChallenge(ctx context.Context, token, code string) (Principal, error) {
	var zero Principal
	if s.redis == nil {
		return zero, ErrMFAUnavailable
	}
	raw, err := s.redis.Get(ctx, mfaChallengeKey(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return zero, ErrInvalidMFACode
	}
	if err != nil {
		return zero, err
	}
	attemptKey := mfaChallengeKey(token) + ":attempts"
	attempts, err := s.redis.Incr(ctx, attemptKey).Result()
	if err != nil {
		return zero, err
	}
	if attempts == 1 {
		_ = s.redis.Expire(ctx, attemptKey, s.challengeTTL)
	}
	if attempts > 5 {
		_ = s.redis.Del(ctx, mfaChallengeKey(token), attemptKey)
		return zero, ErrInvalidMFACode
	}
	var challenge mfaChallenge
	if err := json.Unmarshal(raw, &challenge); err != nil {
		return zero, ErrInvalidMFACode
	}
	secret, err := s.secret(ctx, challenge.UserID)
	if err != nil {
		return zero, err
	}
	if !VerifyTOTP(secret, code, time.Now().UTC()) {
		return zero, ErrInvalidMFACode
	}
	consumed, err := consumeMFAChallengeScript.Run(ctx, s.redis, []string{mfaChallengeKey(token), attemptKey}, string(raw)).Int64()
	if err != nil {
		return zero, err
	}
	if consumed != 1 {
		return zero, ErrInvalidMFACode
	}
	return challenge.Principal, nil
}

func (s *MFAService) secret(ctx context.Context, userID string) (string, error) {
	var encrypted string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(mfa_secret_encrypted, '') FROM iam_accounts WHERE user_id = $1 AND user_type IN (1, 2)`, userID).Scan(&encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMFAUnavailable
	}
	if err != nil {
		return "", err
	}
	if encrypted == "" {
		return "", ErrMFAUnavailable
	}
	return clientsecret.Decrypt(encrypted)
}

func mfaChallengeKey(token string) string {
	sum := sha256Bytes(token)
	return "dai:auth:mfa:challenge:" + hex.EncodeToString(sum)
}

func sha256Bytes(value string) []byte {
	out := sha256.Sum256([]byte(value))
	return out[:]
}

func VerifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	for offset := int64(-1); offset <= 1; offset++ {
		counter := uint64(now.Unix()/30 + offset)
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, counter)), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func totpCode(secret string, counter uint64) string {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)
	h := hmac.New(sha1.New, decoded)
	_, _ = h.Write(message[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
