package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

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
		$26)`

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
			p.MatchedDispatchRuleID, p.MatchedDispatchRuleSummary,
			p.ResolvedLogicalModel, p.ResolvedProviderFamily,
			p.ProtocolConversionEnabled, p.SelectedUpstreamProtocol, p.SelectedUpstreamModel,
			p.UpstreamModelMappingApplied, p.PublicResponseModel,
			p.RequestMessages, p.RequestParams, p.ResponseMessage, p.MediaRefs,
			p.RequestStatus, p.HTTPStatus, p.ErrorCode,
			p.InternalErrorDetail, p.FailedStep,
			p.AttemptsDetail,
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
