package riskcontrol

import (
	"context"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultLogLimit int32 = 50
	maxLogLimit     int32 = 200
)

// LogRepository persists and queries ai_content_moderation_logs.
type LogRepository interface {
	InsertLog(ctx context.Context, log domain.ContentModerationLog) (id string, createdAt time.Time, err error)
	ListLogs(ctx context.Context, f domain.ContentModerationLogFilter, limit, offset int32) ([]domain.ContentModerationLog, error)
	CountLogs(ctx context.Context, f domain.ContentModerationLogFilter) (int64, error)
	CountFlaggedSince(ctx context.Context, userID string, since time.Time) (int64, error)
}

type LogPage = domain.ContentModerationLogPage

type LogService struct {
	repo LogRepository
}

func NewLogService(repo LogRepository) *LogService {
	return &LogService{repo: repo}
}

// Insert persists a single detection outcome. Returns the generated id so
// callers can link a follow-up risk event back to the log row.
func (s *LogService) Insert(ctx context.Context, log domain.ContentModerationLog) (string, time.Time, error) {
	return s.repo.InsertLog(ctx, log)
}

// CountFlaggedSince counts flagged logs for a user since a point in time —
// the rolling-window violation count that drives risk-event creation.
func (s *LogService) CountFlaggedSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	return s.repo.CountFlaggedSince(ctx, userID, since)
}

func (s *LogService) List(ctx context.Context, f domain.ContentModerationLogFilter, limit, offset int32) (LogPage, error) {
	if limit <= 0 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.repo.ListLogs(ctx, f, limit, offset)
	if err != nil {
		return LogPage{}, err
	}
	total, err := s.repo.CountLogs(ctx, f)
	if err != nil {
		return LogPage{}, err
	}
	return LogPage{Items: items, Total: total}, nil
}
