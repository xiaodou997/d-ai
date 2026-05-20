package postgres

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	mrand "math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/serving"
)

const (
	payloadTTL        = 7 * 24 * time.Hour
	payloadCleanupGap = 1 * time.Hour
	defaultSamplePct  = 0.005 // 0.5%
)

// payloadAttempt is the JSON shape stored in route_attempts column.
type payloadAttempt struct {
	RouteID   string  `json:"route_id"`
	Score     float64 `json:"score,omitempty"`
	Outcome   string  `json:"outcome"`
	HTTP      int     `json:"http,omitempty"`
	LatencyMs int     `json:"latency_ms"`
}

// PayloadRecord is a decrypted payload row (used by the replay handler).
type PayloadRecord struct {
	ID             string
	UsageLogID     string
	RawClientBody  []byte
	UpstreamBody   []byte // decrypted upstream request body (nil if not captured)
	RouteAttempts  []payloadAttempt
	Sampled        bool
	ClientProtocol string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// PayloadStore saves and retrieves encrypted request payloads.
type PayloadStore struct {
	pool      *pgxpool.Pool
	masterKey string
	samplePct float64
}

// NewPayloadStore creates a PayloadStore. masterKey is the AES-GCM key master
// (same as ProviderKeyMaster). SamplePct defaults to 0.5% for successful requests.
func NewPayloadStore(pool *pgxpool.Pool, masterKey string) *PayloadStore {
	return &PayloadStore{pool: pool, masterKey: masterKey, samplePct: defaultSamplePct}
}

// Save asynchronously persists the payload. Called from UsageLogger after the
// usage log row is committed so the foreign key is satisfied.
// For failed requests: always saved.
// For successful requests: saved at samplePct probability.
func (s *PayloadStore) Save(ctx context.Context, usageLogID pgtype.UUID, req *serving.Request, clientBody []byte) {
	failed := req.RequestStatus == domain.RequestFailed
	sampled := !failed && mrand.Float64() < s.samplePct //nolint:gosec // non-cryptographic sampling
	if !failed && !sampled {
		return
	}

	attempts := buildAttempts(req)
	attJSON, err := json.Marshal(attempts)
	if err != nil {
		slog.WarnContext(ctx, "payload: marshal attempts failed", "error", err)
		return
	}

	encBody, err := s.encrypt(clientBody)
	if err != nil {
		slog.WarnContext(ctx, "payload: encrypt client body failed", "error", err)
		return
	}

	encUpstream, err := s.encrypt(req.UpstreamBody)
	if err != nil {
		slog.WarnContext(ctx, "payload: encrypt upstream body failed", "error", err)
		return
	}

	encResponse, err := s.encrypt(req.UpstreamResponseBody)
	if err != nil {
		slog.WarnContext(ctx, "payload: encrypt upstream response failed", "error", err)
		return
	}

	const q = `
		INSERT INTO ai_request_payloads
		  (usage_log_id, upstream_body, upstream_response, raw_client_body, route_attempts, sampled, client_protocol, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	expires := time.Now().Add(payloadTTL)
	_, err = s.pool.Exec(ctx, q,
		usageLogID,
		encUpstream,
		encResponse,
		encBody,
		attJSON,
		sampled,
		string(req.ClientProtocol),
		expires,
	)
	if err != nil {
		slog.WarnContext(ctx, "payload: insert failed", "error", err, "usage_log_id", usageLogID)
	}
}

// GetByUsageLogID returns the decrypted payload for a given usage log ID.
// Returns (nil, nil) when not found.
func (s *PayloadStore) GetByUsageLogID(ctx context.Context, usageLogID string) (*PayloadRecord, error) {
	const q = `
		SELECT id, usage_log_id, upstream_body, upstream_response, raw_client_body,
		       route_attempts, sampled, client_protocol, created_at, expires_at
		FROM ai_request_payloads
		WHERE usage_log_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, usageLogID)
	var rec PayloadRecord
	var encUpstream, encResponse, rawBody []byte
	var attJSON []byte
	var usageLogUUID pgtype.UUID
	var id pgtype.UUID

	err := row.Scan(
		&id,
		&usageLogUUID,
		&encUpstream,
		&encResponse,
		&rawBody,
		&attJSON,
		&rec.Sampled,
		&rec.ClientProtocol,
		&rec.CreatedAt,
		&rec.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.ID = uuidToString(id)
	rec.UsageLogID = uuidToString(usageLogUUID)

	if len(encUpstream) > 0 {
		if dec, err := s.decrypt(encUpstream); err == nil {
			rec.UpstreamBody = dec
		} else {
			slog.WarnContext(ctx, "payload: decrypt upstream body failed", "error", err)
		}
	}
	if len(encResponse) > 0 {
		// upstream_response intentionally ignored for now (large; accessed via replay handler separately)
		_ = encResponse
	}
	if len(rawBody) > 0 {
		decrypted, err := s.decrypt(rawBody)
		if err != nil {
			slog.WarnContext(ctx, "payload: decrypt failed", "error", err)
		} else {
			rec.RawClientBody = decrypted
		}
	}

	if len(attJSON) > 0 {
		_ = json.Unmarshal(attJSON, &rec.RouteAttempts)
	}
	return &rec, nil
}

// StartCleanupJob begins a background goroutine that deletes expired rows
// every hour. It exits when ctx is cancelled (typically on server shutdown).
func (s *PayloadStore) StartCleanupJob(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(payloadCleanupGap)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.cleanup(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *PayloadStore) cleanup(ctx context.Context) {
	const q = `DELETE FROM ai_request_payloads WHERE expires_at < now()`
	res, err := s.pool.Exec(ctx, q)
	if err != nil {
		slog.WarnContext(ctx, "payload cleanup failed", "error", err)
		return
	}
	if res.RowsAffected() > 0 {
		slog.InfoContext(ctx, "payload cleanup", "deleted", res.RowsAffected())
	}
}

// ============================================================================
// Encryption helpers
// ============================================================================

func (s *PayloadStore) aead() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(s.masterKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *PayloadStore) encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	aead, err := s.aead()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (s *PayloadStore) decrypt(data []byte) ([]byte, error) {
	aead, err := s.aead()
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(data) < nonceSize {
		return nil, nil
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return aead.Open(nil, nonce, ciphertext, nil)
}

// ============================================================================
// Helpers
// ============================================================================

func buildAttempts(req *serving.Request) []payloadAttempt {
	out := make([]payloadAttempt, 0, len(req.Attempts))
	for _, a := range req.Attempts {
		out = append(out, payloadAttempt{
			RouteID:   a.RouteID,
			Score:     a.Score,
			Outcome:   a.Outcome.String(),
			HTTP:      a.HTTPStatus,
			LatencyMs: a.LatencyMs,
		})
	}
	return out
}

