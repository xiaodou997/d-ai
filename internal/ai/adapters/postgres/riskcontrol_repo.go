package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/riskcontrol"
)

// optionalBool converts a *bool filter to pgtype.Bool (NULL when nil, i.e.
// "no filter" rather than "false").
func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

// optionalTimestamptz converts a *time.Time filter to pgtype.Timestamptz
// (NULL when nil).
func optionalTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// pgTimestamptz converts a non-optional time.Time to pgtype.Timestamptz.
func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// RiskControlRepo implements riskcontrol.SettingRepository,
// riskcontrol.LogRepository and riskcontrol.EventRepository on top of sqlc.
// Config reads/writes reuse the generic ai_settings GetSetting/UpsertSetting
// queries used by the pricing control plane.
type RiskControlRepo struct {
	q *dbgen.Queries
}

func NewRiskControlRepo(q *dbgen.Queries) *RiskControlRepo {
	return &RiskControlRepo{q: q}
}

var (
	_ riskcontrol.SettingRepository = (*RiskControlRepo)(nil)
	_ riskcontrol.LogRepository     = (*RiskControlRepo)(nil)
	_ riskcontrol.EventRepository   = (*RiskControlRepo)(nil)
)

// ---- settings ----

func (r *RiskControlRepo) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	row, err := r.q.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return json.RawMessage(row.Value), nil
}

func (r *RiskControlRepo) UpsertSetting(ctx context.Context, key string, value json.RawMessage) error {
	return r.q.UpsertSetting(ctx, dbgen.UpsertSettingParams{Key: key, Value: []byte(value)})
}

// ---- content moderation logs ----

func (r *RiskControlRepo) InsertLog(ctx context.Context, log domain.ContentModerationLog) (string, time.Time, error) {
	row, err := r.q.InsertContentModerationLog(ctx, dbgen.InsertContentModerationLogParams{
		RequestID:         nullableText(log.RequestID),
		TenantID:          nullableText(log.TenantID),
		UserID:            nullableText(log.UserID),
		ApiKeyID:          nullableUUID(log.APIKeyID),
		ModelCode:         nullableText(log.ModelCode),
		CapabilityType:    nullableText(log.CapabilityType),
		Mode:              log.Mode,
		Action:            log.Action,
		Flagged:           log.Flagged,
		MatchedKeyword:    nullableText(log.MatchedKeyword),
		HighestCategory:   nullableText(log.HighestCategory),
		HighestScore:      floatPtrToNumeric(log.HighestScore),
		CategoryScores:    log.CategoryScores,
		ThresholdSnapshot: log.ThresholdSnapshot,
		InputExcerpt:      nullableText(log.InputExcerpt),
		UpstreamLatencyMs: int32PtrToInt4(log.UpstreamLatencyMs),
		Error:             nullableText(log.Error),
		HitLayer:          nullableText(log.HitLayer),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return uuidToString(row.ID), row.CreatedAt.Time, nil
}

func (r *RiskControlRepo) ListLogs(ctx context.Context, f domain.ContentModerationLogFilter, limit, offset int32) ([]domain.ContentModerationLog, error) {
	rows, err := r.q.ListContentModerationLogs(ctx, dbgen.ListContentModerationLogsParams{
		TenantID: nullableText(f.TenantID),
		UserID:   nullableText(f.UserID),
		Mode:     nullableText(f.Mode),
		Action:   nullableText(f.Action),
		Flagged:  optionalBool(f.Flagged),
		HitLayer: nullableText(f.HitLayer),
		DateFrom: optionalTimestamptz(f.DateFrom),
		DateTo:   optionalTimestamptz(f.DateTo),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ContentModerationLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, moderationLogFromRow(row))
	}
	return out, nil
}

func (r *RiskControlRepo) CountLogs(ctx context.Context, f domain.ContentModerationLogFilter) (int64, error) {
	return r.q.CountContentModerationLogs(ctx, dbgen.CountContentModerationLogsParams{
		TenantID: nullableText(f.TenantID),
		UserID:   nullableText(f.UserID),
		Mode:     nullableText(f.Mode),
		Action:   nullableText(f.Action),
		Flagged:  optionalBool(f.Flagged),
		HitLayer: nullableText(f.HitLayer),
		DateFrom: optionalTimestamptz(f.DateFrom),
		DateTo:   optionalTimestamptz(f.DateTo),
	})
}

func (r *RiskControlRepo) CountFlaggedSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	return r.q.CountFlaggedModerationLogsSince(ctx, dbgen.CountFlaggedModerationLogsSinceParams{
		UserID:    nullableText(userID),
		CreatedAt: pgTimestamptz(since),
	})
}

func moderationLogFromRow(row dbgen.AiContentModerationLog) domain.ContentModerationLog {
	return domain.ContentModerationLog{
		ID:                uuidToString(row.ID),
		RequestID:         row.RequestID.String,
		TenantID:          row.TenantID.String,
		UserID:            row.UserID.String,
		APIKeyID:          uuidToString(row.ApiKeyID),
		ModelCode:         row.ModelCode.String,
		CapabilityType:    row.CapabilityType.String,
		Mode:              row.Mode,
		Action:            row.Action,
		Flagged:           row.Flagged,
		MatchedKeyword:    row.MatchedKeyword.String,
		HighestCategory:   row.HighestCategory.String,
		HighestScore:      numericToFloatPtr(row.HighestScore),
		CategoryScores:    row.CategoryScores,
		ThresholdSnapshot: row.ThresholdSnapshot,
		InputExcerpt:      row.InputExcerpt.String,
		UpstreamLatencyMs: akInt4StrPtr(row.UpstreamLatencyMs),
		Error:             row.Error.String,
		HitLayer:          row.HitLayer.String,
		CreatedAt:         row.CreatedAt.Time,
	}
}

// ---- risk events ----

func (r *RiskControlRepo) InsertEvent(ctx context.Context, ev domain.RiskEvent) (string, error) {
	row, err := r.q.InsertRiskEvent(ctx, dbgen.InsertRiskEventParams{
		EventType:   ev.EventType,
		Severity:    ev.Severity,
		TenantID:    nullableText(ev.TenantID),
		UserID:      nullableText(ev.UserID),
		SourceLogID: nullableUUID(ev.SourceLogID),
		Summary:     ev.Summary,
		Detail:      ev.Detail,
	})
	if err != nil {
		return "", err
	}
	return uuidToString(row.ID), nil
}

func (r *RiskControlRepo) ListEvents(ctx context.Context, f domain.RiskEventFilter, limit, offset int32) ([]domain.RiskEvent, error) {
	rows, err := r.q.ListRiskEvents(ctx, dbgen.ListRiskEventsParams{
		Status:   nullableText(f.Status),
		TenantID: nullableText(f.TenantID),
		UserID:   nullableText(f.UserID),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.RiskEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, riskEventFromRow(row))
	}
	return out, nil
}

func (r *RiskControlRepo) CountEvents(ctx context.Context, f domain.RiskEventFilter) (int64, error) {
	return r.q.CountRiskEvents(ctx, dbgen.CountRiskEventsParams{
		Status:   nullableText(f.Status),
		TenantID: nullableText(f.TenantID),
		UserID:   nullableText(f.UserID),
	})
}

func (r *RiskControlRepo) ResolveEvent(ctx context.Context, id, status, resolvedBy, note string) (domain.RiskEvent, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.RiskEvent{}, domain.ErrNotFound
	}
	row, err := r.q.ResolveRiskEvent(ctx, dbgen.ResolveRiskEventParams{
		ID:             uid,
		Status:         status,
		ResolvedBy:     nullableText(resolvedBy),
		ResolutionNote: nullableText(note),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RiskEvent{}, domain.ErrNotFound
		}
		return domain.RiskEvent{}, err
	}
	return riskEventFromRow(row), nil
}

func riskEventFromRow(row dbgen.AiRiskEvent) domain.RiskEvent {
	return domain.RiskEvent{
		ID:             uuidToString(row.ID),
		EventType:      row.EventType,
		Severity:       row.Severity,
		TenantID:       row.TenantID.String,
		UserID:         row.UserID.String,
		SourceLogID:    uuidToString(row.SourceLogID),
		Summary:        row.Summary,
		Detail:         row.Detail,
		Status:         row.Status,
		ResolvedBy:     row.ResolvedBy.String,
		ResolvedAt:     akTimePtr(row.ResolvedAt),
		ResolutionNote: row.ResolutionNote.String,
		CreatedAt:      row.CreatedAt.Time,
	}
}
