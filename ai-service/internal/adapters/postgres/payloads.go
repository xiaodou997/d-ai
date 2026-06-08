package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/unihub/ai-service/internal/audit"
)

// AuditStore implements audit.Store backed by PostgreSQL.
type AuditStore struct {
	pool *pgxpool.Pool
}

// NewAuditStore creates an AuditStore backed by pool.
func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

const insertPayloadSQL = `
	INSERT INTO ai_request_payloads (
		request_id, client_protocol, client_ip, user_agent,
		request_path, auth_masked, request_model,
		request_messages, request_params, response_message, media_refs,
		request_status, http_status, error_code
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

// InsertBatch writes a batch of audit payload rows using a pgx pipeline.
// Individual row errors are logged and skipped; the batch is best-effort.
func (s *AuditStore) InsertBatch(ctx context.Context, payloads []*audit.Payload) error {
	if len(payloads) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, p := range payloads {
		batch.Queue(insertPayloadSQL,
			p.RequestID, p.ClientProtocol, p.ClientIP, p.UserAgent,
			p.RequestPath, p.AuthMasked, p.RequestModel,
			p.RequestMessages, p.RequestParams, p.ResponseMessage, p.MediaRefs,
			p.RequestStatus, p.HTTPStatus, p.ErrorCode,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for _, p := range payloads {
		if _, err := br.Exec(); err != nil {
			zap.L().Warn("audit: insert row failed",
				zap.Error(err),
				zap.String("request_id", p.RequestID),
			)
		}
	}
	return nil
}

// AuditRecord is the combined row returned by GetByRequestID.
// Fields from ai_request_payloads are always populated; fields from
// ai_usage_logs are zero-valued when no usage row exists (early failures).
type AuditRecord struct {
	// from ai_request_payloads
	RequestID       string
	ClientProtocol  string
	RequestModel    string
	RequestMessages []byte
	RequestParams   []byte
	ResponseMessage []byte
	MediaRefs       []byte
	RequestStatus   string
	HTTPStatus      int
	ErrorCode       string

	// from ai_usage_logs (zero when absent)
	TenantID         string
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int
}

// GetByRequestID returns the most-recent audit row for the given request_id,
// LEFT JOIN-ed with ai_usage_logs for token/latency metadata.
// Returns (nil, nil) when not found.
func (s *AuditStore) GetByRequestID(ctx context.Context, requestID string) (*AuditRecord, error) {
	const q = `
		SELECT
			p.request_id, p.client_protocol, p.request_model,
			p.request_messages, p.request_params, p.response_message, p.media_refs,
			p.request_status, p.http_status, p.error_code,
			COALESCE(u.tenant_id, '')          AS tenant_id,
			COALESCE(u.prompt_tokens, 0)       AS prompt_tokens,
			COALESCE(u.completion_tokens, 0)   AS completion_tokens,
			COALESCE(u.latency_ms, 0)          AS latency_ms
		FROM ai_request_payloads p
		LEFT JOIN ai_usage_logs u ON u.request_id = p.request_id
		WHERE p.request_id = $1
		ORDER BY p.created_at DESC
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, requestID)
	var rec AuditRecord
	var msgs, params, resp, mediaRefs []byte
	if err := row.Scan(
		&rec.RequestID, &rec.ClientProtocol, &rec.RequestModel,
		&msgs, &params, &resp, &mediaRefs,
		&rec.RequestStatus, &rec.HTTPStatus, &rec.ErrorCode,
		&rec.TenantID, &rec.PromptTokens, &rec.CompletionTokens, &rec.LatencyMs,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec.RequestMessages = msgs
	rec.RequestParams = params
	rec.ResponseMessage = resp
	rec.MediaRefs = mediaRefs
	return &rec, nil
}
