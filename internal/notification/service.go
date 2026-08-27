package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/auth"
)

var (
	ErrInvalidChannel = errors.New("notification channel is invalid")
	ErrInvalidInput   = errors.New("notification input is invalid")
	ErrInvalidActor   = errors.New("notification actor is invalid")
)

type Input struct {
	EventKey        string
	Channel         string
	RecipientUserID string
	RecipientType   int
	TenantID        string
	Title           string
	Body            string
	Payload         map[string]any
	IdempotencyKey  string
	WebhookURL      string
}

type Delivery struct {
	ID              string         `json:"id"`
	EventKey        string         `json:"eventKey"`
	Channel         string         `json:"channel"`
	RecipientUserID string         `json:"recipientUserId,omitempty"`
	RecipientType   int            `json:"recipientUserType,omitempty"`
	TenantID        string         `json:"tenantId,omitempty"`
	Title           string         `json:"title"`
	Body            string         `json:"body"`
	Payload         map[string]any `json:"payload"`
	Status          string         `json:"status"`
	Attempts        int            `json:"attempts"`
	LastError       string         `json:"lastError,omitempty"`
	SentAt          *time.Time     `json:"sentAt,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
}

// HTTPService is the narrow notification application surface used by the
// transport module. Persistence and delivery implementation details remain
// behind the service.
type HTTPService interface {
	ListForActor(ctx context.Context, actor auth.Actor, limit int) ([]Delivery, error)
	Send(ctx context.Context, input Input) (Delivery, error)
}

type Service struct {
	pool *pgxpool.Pool
	http *http.Client
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, http: &http.Client{Timeout: 8 * time.Second}}
}

// Send dispatches one notification through the requested channel. Channel
// selection belongs to the application command so every caller receives the
// same validation and delivery semantics.
func (s *Service) Send(ctx context.Context, input Input) (Delivery, error) {
	switch input.Channel {
	case "in_app":
		return s.CreateInApp(ctx, input)
	case "webhook":
		return s.SendWebhook(ctx, input)
	default:
		return Delivery{}, ErrInvalidChannel
	}
}

func (s *Service) CreateInApp(ctx context.Context, input Input) (Delivery, error) {
	input.Channel = "in_app"
	return s.create(ctx, input, "sent", "")
}

func (s *Service) SendWebhook(ctx context.Context, input Input) (Delivery, error) {
	input.Channel = "webhook"
	if err := validateWebhookURL(input.WebhookURL); err != nil {
		return Delivery{}, err
	}
	payload, err := json.Marshal(map[string]any{"event": input.EventKey, "title": input.Title, "body": input.Body, "payload": input.Payload})
	if err != nil {
		return Delivery{}, err
	}
	delivery, err := s.create(ctx, input, "pending", "")
	if err != nil {
		return Delivery{}, err
	}
	if input.IdempotencyKey != "" && delivery.Status == "sent" {
		return delivery, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, input.WebhookURL, strings.NewReader(string(payload)))
	if err != nil {
		return s.markFailed(ctx, delivery.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return s.markFailed(ctx, delivery.ID, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.markFailed(ctx, delivery.ID, fmt.Errorf("webhook returned HTTP %d", resp.StatusCode))
	}
	return s.markSent(ctx, delivery.ID)
}

// ListForActor returns only notifications addressed to the authenticated
// actor. Authorization is repeated here so callers cannot accidentally widen
// the query by passing an arbitrary user/tenant tuple.
func (s *Service) ListForActor(ctx context.Context, actor auth.Actor, limit int) ([]Delivery, error) {
	if actor.UserID == "" || (!actor.Has(auth.CapabilitySuperAdmin) &&
		!actor.Has(auth.CapabilityPlatformAdmin) &&
		!actor.Has(auth.CapabilityTenantSelf) &&
		!actor.Has(auth.CapabilityCustomerSelf)) {
		return nil, ErrInvalidActor
	}
	userID, userType, tenantID := string(actor.UserID), int(actor.UserType), string(actor.TenantID)
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_key, channel, COALESCE(recipient_user_id, ''), COALESCE(recipient_user_type, 0),
		       COALESCE(tenant_id, ''), title, body, payload, status, attempts, COALESCE(last_error, ''), sent_at, created_at
		FROM sys_notification_deliveries
		WHERE recipient_user_id = $1 AND (recipient_user_type = $2 OR recipient_user_type IS NULL)
		  AND (tenant_id = $3 OR tenant_id IS NULL)
		ORDER BY created_at DESC LIMIT $4`, userID, userType, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Delivery, 0)
	for rows.Next() {
		var item Delivery
		var raw []byte
		if err := rows.Scan(&item.ID, &item.EventKey, &item.Channel, &item.RecipientUserID, &item.RecipientType,
			&item.TenantID, &item.Title, &item.Body, &raw, &item.Status, &item.Attempts, &item.LastError, &item.SentAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &item.Payload)
		}
		if item.Payload == nil {
			item.Payload = map[string]any{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) create(ctx context.Context, input Input, status, lastError string) (Delivery, error) {
	if input.Channel != "in_app" && input.Channel != "webhook" {
		return Delivery{}, ErrInvalidChannel
	}
	if strings.TrimSpace(input.EventKey) == "" || strings.TrimSpace(input.Title) == "" {
		return Delivery{}, fmt.Errorf("%w: eventKey and title are required", ErrInvalidInput)
	}
	if input.Payload == nil {
		input.Payload = map[string]any{}
	}
	raw, err := json.Marshal(input.Payload)
	if err != nil {
		return Delivery{}, err
	}
	id := uuid.NewString()
	var storedID string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO sys_notification_deliveries (id, event_key, channel, recipient_user_id, recipient_user_type, tenant_id, title, body, payload, status, attempts, last_error, idempotency_key, sent_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, 0), NULLIF($6, ''), $7, $8, $9::jsonb, $10, 1, NULLIF($11, ''), NULLIF($12, ''), CASE WHEN $10 = 'sent' THEN now() ELSE NULL END)
		ON CONFLICT (idempotency_key) DO UPDATE SET id = sys_notification_deliveries.id
		RETURNING id`, id, input.EventKey, input.Channel, input.RecipientUserID, input.RecipientType, input.TenantID, input.Title, input.Body, raw, status, lastError, input.IdempotencyKey).Scan(&storedID)
	if err != nil {
		return Delivery{}, err
	}
	return s.get(ctx, storedID)
}

func (s *Service) get(ctx context.Context, id string) (Delivery, error) {
	var item Delivery
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT id, event_key, channel, COALESCE(recipient_user_id, ''), COALESCE(recipient_user_type, 0), COALESCE(tenant_id, ''), title, body, payload, status, attempts, COALESCE(last_error, ''), sent_at, created_at FROM sys_notification_deliveries WHERE id = $1`, id).Scan(&item.ID, &item.EventKey, &item.Channel, &item.RecipientUserID, &item.RecipientType, &item.TenantID, &item.Title, &item.Body, &raw, &item.Status, &item.Attempts, &item.LastError, &item.SentAt, &item.CreatedAt)
	if err != nil {
		return Delivery{}, err
	}
	_ = json.Unmarshal(raw, &item.Payload)
	return item, nil
}

func (s *Service) markSent(ctx context.Context, id string) (Delivery, error) {
	_, err := s.pool.Exec(ctx, `UPDATE sys_notification_deliveries SET status = 'sent', attempts = attempts + 1, last_error = NULL, sent_at = now() WHERE id = $1`, id)
	if err != nil {
		return Delivery{}, err
	}
	return s.get(ctx, id)
}

func (s *Service) markFailed(ctx context.Context, id string, cause error) (Delivery, error) {
	_, err := s.pool.Exec(ctx, `UPDATE sys_notification_deliveries SET status = 'failed', attempts = attempts + 1, last_error = $2 WHERE id = $1`, id, cause.Error())
	if err != nil {
		return Delivery{}, err
	}
	return s.get(ctx, id)
}

func validateWebhookURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: webhook URL must be http or https", ErrInvalidInput)
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "[::1]" {
		return fmt.Errorf("%w: localhost webhook is not allowed", ErrInvalidInput)
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return fmt.Errorf("%w: private webhook address is not allowed", ErrInvalidInput)
	}
	return nil
}
