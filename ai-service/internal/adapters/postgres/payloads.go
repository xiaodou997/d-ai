package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/uni-ai-api/internal/audit"
)

// AuditStore implements audit.Store backed by PostgreSQL.
type AuditStore struct {
	pool *pgxpool.Pool
}

// NewAuditStore creates an AuditStore backed by pool.
func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

// InsertPayload writes a single audit payload row. Errors are non-fatal:
// the caller (audit.Worker) logs and discards failures.
func (s *AuditStore) InsertPayload(ctx context.Context, p *audit.Payload) error {
	const q = `
		INSERT INTO ai_request_payloads (
			request_id, tenant_id, api_key_id, capability_type, client_protocol,
			client_ip, user_agent, request_path, auth_masked, request_model,
			request_messages, request_params, route_id, upstream_provider,
			upstream_model, upstream_endpoint, response_message, response_model,
			prompt_tokens, completion_tokens, request_status, http_status,
			error_code, latency_ms, first_token_ms
		) VALUES (
			$1,  $2,       $3::uuid,   $4,  $5,
			$6,  $7,       $8,         $9,  $10,
			$11, $12,      $13,        $14,
			$15, $16,      $17,        $18,
			$19, $20,      $21,        $22,
			$23, $24,      $25
		)`
	_, err := s.pool.Exec(ctx, q,
		p.RequestID, p.TenantID, nilUUID(p.APIKeyID), p.CapabilityType, p.ClientProtocol,
		p.ClientIP, p.UserAgent, p.RequestPath, p.AuthMasked, p.RequestModel,
		p.RequestMessages, p.RequestParams, p.RouteID, p.UpstreamProvider,
		p.UpstreamModel, p.UpstreamEndpoint, p.ResponseMessage, p.ResponseModel,
		p.PromptTokens, p.CompletionTokens, p.RequestStatus, p.HTTPStatus,
		p.ErrorCode, p.LatencyMs, p.FirstTokenMs,
	)
	return err
}

// AuditRecord is a single audit row returned by query methods.
type AuditRecord struct {
	RequestID       string
	ClientProtocol  string
	RequestModel    string
	RequestMessages []byte
	RequestParams   []byte
	ResponseMessage []byte
	RequestStatus   string
	HTTPStatus      int
	ErrorCode       string
	LatencyMs       int
}

// GetByRequestID returns the most-recent audit row for the given request_id.
// Returns (nil, nil) when not found.
func (s *AuditStore) GetByRequestID(ctx context.Context, requestID string) (*AuditRecord, error) {
	const q = `
		SELECT request_id, client_protocol, request_model,
		       request_messages, request_params, response_message,
		       request_status, http_status, error_code, latency_ms
		FROM ai_request_payloads
		WHERE request_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, requestID)
	var rec AuditRecord
	var msgs, params, resp []byte
	if err := row.Scan(
		&rec.RequestID, &rec.ClientProtocol, &rec.RequestModel,
		&msgs, &params, &resp,
		&rec.RequestStatus, &rec.HTTPStatus, &rec.ErrorCode, &rec.LatencyMs,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec.RequestMessages = msgs
	rec.RequestParams = params
	rec.ResponseMessage = resp
	return &rec, nil
}

// nilUUID converts an empty string to nil so pgx treats it as SQL NULL.
func nilUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
