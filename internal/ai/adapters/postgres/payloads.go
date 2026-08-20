package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/audit"
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
		matched_dispatch_rule_id, matched_dispatch_rule_summary,
		resolved_logical_model, resolved_provider_family,
		protocol_conversion_enabled, selected_upstream_protocol, selected_upstream_model,
		upstream_model_mapping_applied, public_response_model,
		request_messages, request_params, response_message, media_refs,
		request_status, http_status, error_code, internal_error_detail, failed_step,
		attempts_detail
	) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid, NULLIF($9, ''),
		NULLIF($10, ''), NULLIF($11, ''), $12, NULLIF($13, ''), NULLIF($14, ''),
		$15, NULLIF($16, ''), $17, $18, $19, $20, $21, $22, $23, NULLIF($24, ''), NULLIF($25, ''),
		$26)
	ON CONFLICT (request_id) DO NOTHING`

// Enqueue writes the compact audit envelope to the durable inbox.
func (s *AuditStore) Enqueue(ctx context.Context, payload *audit.Payload) error {
	if payload == nil || payload.RequestID == "" {
		return errors.New("audit payload requires a request id")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit enqueue: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := s.EnqueueTx(ctx, tx, payload); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit enqueue: %w", err)
	}
	return nil
}

// EnqueueTx is used by UsageLogger so normal requests commit the audit inbox
// row in the same transaction as the usage and financial facts.
func (s *AuditStore) EnqueueTx(ctx context.Context, tx pgx.Tx, payload *audit.Payload) error {
	if payload == nil || payload.RequestID == "" {
		return errors.New("audit payload requires a request id")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal audit payload: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_audit_inbox (request_id, payload)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (request_id) DO NOTHING
	`, payload.RequestID, raw); err != nil {
		return fmt.Errorf("insert audit inbox: %w", err)
	}
	return nil
}

// Claim leases ready and expired-processing rows. The transaction is committed
// before the payload is processed so a slow blob write never holds row locks.
func (s *AuditStore) Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]audit.Delivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin audit claim: %w", err)
	}
	defer tx.Rollback(ctx)
	leaseBefore := time.Now().UTC().Add(-lease)
	rows, err := tx.Query(ctx, `
		WITH claimed AS (
			SELECT id
			FROM ai_audit_inbox
			WHERE (status = 'pending' AND available_at <= now())
			   OR (status = 'processing' AND locked_at < $3)
			ORDER BY available_at, created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE ai_audit_inbox AS inbox
		SET status = 'processing', attempts = inbox.attempts + 1,
			locked_at = now(), locked_by = $2
		FROM claimed
		WHERE inbox.id = claimed.id
		RETURNING inbox.id, inbox.request_id, inbox.payload, inbox.attempts
	`, limit, workerID, leaseBefore)
	if err != nil {
		return nil, fmt.Errorf("claim audit inbox: %w", err)
	}
	defer rows.Close()
	deliveries := make([]audit.Delivery, 0, limit)
	for rows.Next() {
		var delivery audit.Delivery
		var requestID string
		var raw []byte
		if err := rows.Scan(&delivery.ID, &requestID, &raw, &delivery.Attempts); err != nil {
			return nil, fmt.Errorf("scan audit inbox: %w", err)
		}
		var payload audit.Payload
		if err := json.Unmarshal(raw, &payload); err != nil {
			payload.RequestID = requestID
			delivery.Payload = nil
		} else {
			delivery.Payload = &payload
		}
		delivery.WorkerID = workerID
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read audit inbox: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit audit claim: %w", err)
	}
	return deliveries, nil
}

// Complete materializes the payload and removes the inbox row atomically.
// Both operations are idempotent, which makes crash recovery safe.
func (s *AuditStore) Complete(ctx context.Context, delivery audit.Delivery) error {
	if delivery.Payload == nil {
		return errors.New("audit delivery payload is nil")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit completion: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := insertPayload(ctx, tx, delivery.Payload); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_audit_inbox
		WHERE id = $1 AND request_id = $2 AND status = 'processing' AND locked_by = $3
	`, delivery.ID, delivery.Payload.RequestID, delivery.WorkerID); err != nil {
		return fmt.Errorf("remove completed audit inbox row: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit completion: %w", err)
	}
	return nil
}

func insertPayload(ctx context.Context, tx pgx.Tx, p *audit.Payload) error {
	if _, err := tx.Exec(ctx, insertPayloadSQL,
		p.RequestID, p.ClientProtocol, p.ClientIP, p.UserAgent,
		p.RequestPath, p.AuthMasked, p.RequestModel,
		p.MatchedDispatchRuleID, p.MatchedDispatchRuleSummary,
		p.ResolvedLogicalModel, p.ResolvedProviderFamily,
		p.ProtocolConversionEnabled, p.SelectedUpstreamProtocol, p.SelectedUpstreamModel,
		p.UpstreamModelMappingApplied, p.PublicResponseModel,
		p.RequestMessages, p.RequestParams, p.ResponseMessage, p.MediaRefs,
		p.RequestStatus, p.HTTPStatus, p.ErrorCode,
		p.InternalErrorDetail, p.FailedStep,
		p.AttemptsDetail,
	); err != nil {
		return fmt.Errorf("insert audit payload: %w", err)
	}
	return nil
}

func (s *AuditStore) Retry(ctx context.Context, delivery audit.Delivery, availableAt time.Time, cause error, dead bool) error {
	status := "pending"
	if dead {
		status = "dead"
	}
	message := "audit delivery failed"
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE ai_audit_inbox
		SET status = $1, available_at = $2, last_error = $3,
			dead_at = CASE WHEN $1 = 'dead' THEN now() ELSE NULL END,
			locked_at = NULL, locked_by = NULL
		WHERE id = $4 AND status = 'processing' AND locked_by = $5
	`, status, availableAt, message, delivery.ID, delivery.WorkerID)
	if err != nil {
		return fmt.Errorf("record audit delivery failure: %w", err)
	}
	return nil
}

func (s *AuditStore) Stats(ctx context.Context) (audit.QueueStats, error) {
	var stats audit.QueueStats
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('pending', 'processing')),
			COUNT(*) FILTER (WHERE status = 'dead'),
			COALESCE(EXTRACT(EPOCH FROM now() - MIN(created_at) FILTER (WHERE status IN ('pending', 'processing'))), 0)
		FROM ai_audit_inbox
	`).Scan(&stats.Pending, &stats.Dead, &stats.OldestS); err != nil {
		return audit.QueueStats{}, fmt.Errorf("read audit inbox stats: %w", err)
	}
	return stats, nil
}

// AuditRecord is the combined row returned by GetByRequestID.
// Fields from ai_request_payloads are always populated; fields from
// ai_usage_logs are zero-valued when no usage row exists (early failures).
type AuditRecord struct {
	// from ai_request_payloads
	RequestID                   string
	ClientProtocol              string
	RequestModel                string
	RequestPath                 string
	ClientIP                    string
	UserAgent                   string
	AuthMasked                  string
	RequestMessages             []byte
	RequestParams               []byte
	ResponseMessage             []byte
	MediaRefs                   []byte
	RequestStatus               string
	HTTPStatus                  int
	ErrorCode                   string
	InternalErrorDetail         string
	FailedStep                  string
	AttemptsDetail              []byte
	MatchedDispatchRuleID       string
	MatchedDispatchRuleSummary  string
	ResolvedLogicalModel        string
	ResolvedProviderFamily      string
	ProtocolConversionEnabled   bool
	SelectedUpstreamProtocol    string
	SelectedUpstreamModel       string
	UpstreamModelMappingApplied bool
	PublicResponseModel         string

	// from ai_usage_logs (zero when absent)
	TenantID                      string
	GroupID                       string
	RequestedModel                string
	ResolvedUsageLogicalModel     string
	ResolvedUsageProviderFamily   string
	SelectedUpstreamProtocolUsage string
	UpstreamModelUsage            string
	PromptTokens                  int
	CompletionTokens              int
	LatencyMs                     int
}

// GetByRequestID returns the most-recent audit row for the given request_id,
// LEFT JOIN-ed with ai_usage_logs for token/latency metadata.
// Returns (nil, nil) when not found.
func (s *AuditStore) GetByRequestID(ctx context.Context, requestID string) (*AuditRecord, error) {
	const q = `
		SELECT
			p.request_id, p.client_protocol, p.request_model,
			p.request_path, COALESCE(p.client_ip, '') AS client_ip,
			COALESCE(p.user_agent, '') AS user_agent, COALESCE(p.auth_masked, '') AS auth_masked,
			COALESCE(p.matched_dispatch_rule_id::text, '') AS matched_dispatch_rule_id,
			COALESCE(p.matched_dispatch_rule_summary, '') AS matched_dispatch_rule_summary,
			COALESCE(p.resolved_logical_model, '') AS resolved_logical_model,
			COALESCE(p.resolved_provider_family, '') AS resolved_provider_family,
			p.protocol_conversion_enabled,
			COALESCE(p.selected_upstream_protocol, '') AS selected_upstream_protocol,
			COALESCE(p.selected_upstream_model, '') AS selected_upstream_model,
			p.upstream_model_mapping_applied,
			COALESCE(p.public_response_model, '') AS public_response_model,
			p.request_messages, p.request_params, p.response_message, p.media_refs,
			p.request_status, p.http_status, p.error_code,
			COALESCE(p.internal_error_detail, '') AS internal_error_detail,
			COALESCE(p.failed_step, '')        AS failed_step,
			p.attempts_detail,
			COALESCE(u.tenant_id, '')          AS tenant_id,
			COALESCE(u.group_id::text, '')     AS group_id,
			COALESCE(u.requested_model, '')    AS requested_model,
			COALESCE(u.resolved_logical_model, '') AS resolved_usage_logical_model,
			COALESCE(u.resolved_provider_family, '') AS resolved_usage_provider_family,
			COALESCE(u.provider_format, '')    AS selected_upstream_protocol_usage,
			COALESCE(u.upstream_model, '')     AS upstream_model_usage,
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
		&rec.RequestPath, &rec.ClientIP, &rec.UserAgent, &rec.AuthMasked,
		&rec.MatchedDispatchRuleID, &rec.MatchedDispatchRuleSummary,
		&rec.ResolvedLogicalModel, &rec.ResolvedProviderFamily,
		&rec.ProtocolConversionEnabled, &rec.SelectedUpstreamProtocol, &rec.SelectedUpstreamModel,
		&rec.UpstreamModelMappingApplied, &rec.PublicResponseModel,
		&msgs, &params, &resp, &mediaRefs,
		&rec.RequestStatus, &rec.HTTPStatus, &rec.ErrorCode,
		&rec.InternalErrorDetail, &rec.FailedStep,
		&rec.AttemptsDetail,
		&rec.TenantID, &rec.GroupID, &rec.RequestedModel,
		&rec.ResolvedUsageLogicalModel, &rec.ResolvedUsageProviderFamily,
		&rec.SelectedUpstreamProtocolUsage, &rec.UpstreamModelUsage,
		&rec.PromptTokens, &rec.CompletionTokens, &rec.LatencyMs,
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
