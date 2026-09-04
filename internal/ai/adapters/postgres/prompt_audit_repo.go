package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/promptaudit"
)

type PromptAuditRepo struct{ pool *pgxpool.Pool }

func NewPromptAuditRepo(pool *pgxpool.Pool) *PromptAuditRepo { return &PromptAuditRepo{pool: pool} }

func (r *PromptAuditRepo) InsertPromptAuditEvent(ctx context.Context, event promptaudit.Event) error {
	if r == nil || r.pool == nil {
		return nil
	}
	result := event.Result
	decision, riskLevel, action := result.Decision, result.RiskLevel, result.Action
	if event.ErrorCode != "" {
		decision, riskLevel, action = "error", "unknown", "Error"
	}
	categories, _ := json.Marshal(nonNilStrings(result.Categories))
	matched, _ := json.Marshal(nonNilStrings(result.MatchedScanners))
	unknown, _ := json.Marshal(nonNilStrings(result.UnknownCategories))
	scores := result.ScannerScores
	if scores == nil {
		scores = map[string]float64{}
	}
	scoreJSON, _ := json.Marshal(scores)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ai_prompt_audit_events (
			request_id, tenant_id, user_id, api_key_id, model_code, capability_type, protocol,
			prompt_hash, redacted_preview, prompt_length, message_count,
			decision, risk_level, action, safety, categories, matched_scanners, scanner_scores,
			unknown_categories, scanner_version, guard_endpoint_id, config_revision,
			chunk_total, latency_ms, error_code, created_at
		) VALUES (
			NULLIF($1,''), NULLIF($2,''), NULLIF($3,''), NULLIF($4,'')::uuid, NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
			$8, $9, $10, $11, $12, $13, $14, NULLIF($15,''), $16::jsonb, $17::jsonb, $18::jsonb,
			$19::jsonb, NULLIF($20,''), NULLIF($21,''), $22, $23, $24, NULLIF($25,''), $26
		)`, event.Snapshot.RequestID, event.Snapshot.TenantID, event.Snapshot.UserID, event.Snapshot.APIKeyID,
		event.Snapshot.ModelCode, event.Snapshot.CapabilityType, event.Snapshot.Protocol,
		event.Snapshot.PromptHash, event.Snapshot.RedactedPreview, event.Snapshot.PromptLength, event.Snapshot.MessageCount,
		decision, riskLevel, action, result.Safety, categories, matched, scoreJSON, unknown,
		result.ScannerVersion, result.EndpointID, event.ConfigRevision, result.ChunkTotal, result.LatencyMS,
		event.ErrorCode, event.CreatedAt)
	return err
}

func (r *PromptAuditRepo) ListPromptAuditEvents(ctx context.Context, filter promptaudit.EventFilter) (promptaudit.EventPage, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	where := ` WHERE ($1 = '' OR tenant_id = $1) AND ($2 = '' OR user_id = $2) AND ($3 = '' OR decision = $3)`
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_prompt_audit_events`+where, filter.TenantID, filter.UserID, filter.Decision).Scan(&total); err != nil {
		return promptaudit.EventPage{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, COALESCE(request_id,''), COALESCE(tenant_id,''), COALESCE(user_id,''), COALESCE(api_key_id::text,''), COALESCE(model_code,''), COALESCE(capability_type,''), COALESCE(protocol,''), prompt_hash, redacted_preview, prompt_length, message_count, decision, risk_level, action, COALESCE(safety,''), categories, matched_scanners, scanner_scores, COALESCE(scanner_version,''), COALESCE(guard_endpoint_id,''), config_revision, chunk_total, latency_ms, COALESCE(error_code,''), created_at FROM ai_prompt_audit_events`+where+` ORDER BY created_at DESC, id DESC LIMIT $4 OFFSET $5`, filter.TenantID, filter.UserID, filter.Decision, filter.Limit, filter.Offset)
	if err != nil {
		return promptaudit.EventPage{}, err
	}
	defer rows.Close()
	items := make([]promptaudit.StoredEvent, 0, filter.Limit)
	for rows.Next() {
		var item promptaudit.StoredEvent
		var categories, matched, scores []byte
		err := rows.Scan(&item.ID, &item.Snapshot.RequestID, &item.Snapshot.TenantID, &item.Snapshot.UserID, &item.Snapshot.APIKeyID, &item.Snapshot.ModelCode, &item.Snapshot.CapabilityType, &item.Snapshot.Protocol, &item.Snapshot.PromptHash, &item.Snapshot.RedactedPreview, &item.Snapshot.PromptLength, &item.Snapshot.MessageCount, &item.Decision, &item.RiskLevel, &item.Action, &item.Safety, &categories, &matched, &scores, &item.ScannerVersion, &item.EndpointID, &item.ConfigRevision, &item.ChunkTotal, &item.LatencyMS, &item.ErrorCode, &item.CreatedAt)
		if err != nil {
			return promptaudit.EventPage{}, fmt.Errorf("scan prompt audit event: %w", err)
		}
		_ = json.Unmarshal(categories, &item.Categories)
		_ = json.Unmarshal(matched, &item.MatchedScanners)
		_ = json.Unmarshal(scores, &item.ScannerScores)
		items = append(items, item)
	}
	return promptaudit.EventPage{Items: items, Total: total}, rows.Err()
}

func (r *PromptAuditRepo) DeletePromptAuditEvent(ctx context.Context, id string) (bool, error) {
	result, err := r.pool.Exec(ctx, `DELETE FROM ai_prompt_audit_events WHERE id = $1::uuid`, id)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
