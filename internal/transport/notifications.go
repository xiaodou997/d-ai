package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	notificationpkg "xiaodou/dai/internal/notification"
	"xiaodou/dai/libs/go/httpx"
)

type notificationListInput struct {
	Limit int `query:"limit" default:"50"`
}
type notificationsOutput struct{ Body []notificationpkg.Delivery }
type notificationSendInput struct {
	Body struct {
		EventKey        string         `json:"eventKey"`
		Channel         string         `json:"channel" enum:"in_app,webhook"`
		RecipientUserID string         `json:"recipientUserId,omitempty" required:"false"`
		RecipientType   int            `json:"recipientUserType,omitempty" required:"false"`
		TenantID        string         `json:"tenantId,omitempty" required:"false"`
		Title           string         `json:"title"`
		Body            string         `json:"body"`
		Payload         map[string]any `json:"payload,omitempty" required:"false"`
		IdempotencyKey  string         `json:"idempotencyKey,omitempty" required:"false"`
		WebhookURL      string         `json:"webhookUrl,omitempty" required:"false"`
	}
}
type notificationOutput struct{ Body notificationpkg.Delivery }

func registerNotifications(api huma.API, d Deps) {
	if d.Notifications == nil {
		return
	}
	ua := userAuth(api, d.JWT, d.Blacklist)
	allUsers := huma.Middlewares{ua, requireUserType(api, 1, 2, 3, 4)}
	admins := huma.Middlewares{ua, requireUserType(api, 1, 2)}
	huma.Register(api, huma.Operation{OperationID: "list-my-notifications", Method: http.MethodGet, Path: "/api/v1/notifications", Summary: "我的通知", Tags: []string{"notifications"}, Middlewares: allUsers}, func(ctx context.Context, in *notificationListInput) (*notificationsOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		items, err := d.Notifications.ListForUser(ctx, claims.UserID, claims.UserType, claims.TenantID, in.Limit)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &notificationsOutput{Body: items}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "admin-send-notification", Method: http.MethodPost, Path: "/api/v1/admin/notifications/send", Summary: "发送通知", Tags: []string{"notifications"}, Middlewares: admins}, func(ctx context.Context, in *notificationSendInput) (*notificationOutput, error) {
		body := in.Body
		input := notificationpkg.Input{EventKey: body.EventKey, Channel: body.Channel, RecipientUserID: body.RecipientUserID, RecipientType: body.RecipientType, TenantID: body.TenantID, Title: body.Title, Body: body.Body, Payload: body.Payload, IdempotencyKey: body.IdempotencyKey, WebhookURL: body.WebhookURL}
		var item notificationpkg.Delivery
		var err error
		if body.Channel == "webhook" {
			item, err = d.Notifications.SendWebhook(ctx, input)
		} else {
			item, err = d.Notifications.CreateInApp(ctx, input)
		}
		if errors.Is(err, notificationpkg.ErrInvalidChannel) || errors.Is(err, notificationpkg.ErrInvalidInput) {
			return nil, httpx.ErrBadRequest.WithCause(err)
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &notificationOutput{Body: item}, nil
	})
}
