package riskcontrol

import (
	"context"

	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultEventLimit int32 = 50
	maxEventLimit     int32 = 200
)

// EventRepository persists and queries ai_risk_events, the human-in-the-loop
// queue. Resolution never mutates account status; identity administration owns it.
type EventRepository interface {
	InsertEvent(ctx context.Context, ev domain.RiskEvent) (id string, err error)
	ListEvents(ctx context.Context, f domain.RiskEventFilter, limit, offset int32) ([]domain.RiskEvent, error)
	CountEvents(ctx context.Context, f domain.RiskEventFilter) (int64, error)
	ResolveEvent(ctx context.Context, id, status, resolvedBy, note string) (domain.RiskEvent, error)
}

type EventPage = domain.RiskEventPage

type EventService struct {
	repo EventRepository
}

func NewEventService(repo EventRepository) *EventService {
	return &EventService{repo: repo}
}

// Create raises a new risk event. Called by the moderation Checker when a
// user's rolling-window violation count crosses the configured threshold.
func (s *EventService) Create(ctx context.Context, ev domain.RiskEvent) (string, error) {
	return s.repo.InsertEvent(ctx, ev)
}

func (s *EventService) List(ctx context.Context, f domain.RiskEventFilter, limit, offset int32) (EventPage, error) {
	if limit <= 0 {
		limit = defaultEventLimit
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.repo.ListEvents(ctx, f, limit, offset)
	if err != nil {
		return EventPage{}, err
	}
	total, err := s.repo.CountEvents(ctx, f)
	if err != nil {
		return EventPage{}, err
	}
	return EventPage{Items: items, Total: total}, nil
}

func (s *EventService) Resolve(ctx context.Context, id, status, resolvedBy, note string) (domain.RiskEvent, error) {
	switch status {
	case domain.RiskEventStatusAcknowledged, domain.RiskEventStatusResolved, domain.RiskEventStatusDismissed:
	default:
		return domain.RiskEvent{}, domain.NewValidationError("status", "must be one of acknowledged, resolved, dismissed")
	}
	return s.repo.ResolveEvent(ctx, id, status, resolvedBy, note)
}
