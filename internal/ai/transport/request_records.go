package transport

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"net/http"
	"xiaodou/dai/internal/ai/domain"
)

type recordListInput struct {
	TenantID string `query:"tenant_id"`
	UserID   string `query:"user_id"`
	Model    string `query:"model"`
	Source   string `query:"source"`
	From     string `query:"from"`
	To       string `query:"to"`
	Cursor   string `query:"cursor"`
	Limit    int    `query:"limit" default:"20" minimum:"1" maximum:"100"`
}
type recordListOutput struct{ Body domain.RecordPage }
type recordDetailInput struct {
	RequestID string `path:"requestID"`
}
type recordDetailOutput struct{ Body domain.RequestRecord }
type recordSummaryOutput struct{ Body domain.RecordSummary }
type recordRefundInput struct {
	RequestID string `path:"requestID"`
	Body      struct {
		Reason string `json:"reason" minLength:"1" maxLength:"500"`
	}
}
type recordRefundOutput struct {
	Body struct {
		Refunded bool `json:"refunded"`
	}
}

func recordScope(ctx context.Context, tenant, user string) (domain.RecordScope, error) {
	c := claimsFromContext(ctx)
	if c == nil {
		return domain.RecordScope{}, huma.Error401Unauthorized("authentication required")
	}
	switch c.UserType {
	case 1, 2:
		return domain.RecordScope{TenantID: tenant, UserID: user, Admin: true}, nil
	case 3:
		if c.TenantID == "" {
			return domain.RecordScope{}, huma.Error403Forbidden("tenant identity required")
		}
		return domain.RecordScope{TenantID: c.TenantID, UserID: user}, nil
	case 4:
		if c.TenantID == "" || c.UserID == "" {
			return domain.RecordScope{}, huma.Error403Forbidden("user identity required")
		}
		return domain.RecordScope{TenantID: c.TenantID, UserID: c.UserID, EndUser: true}, nil
	default:
		return domain.RecordScope{}, huma.Error403Forbidden("unsupported account type")
	}
}
func recordQuery(ctx context.Context, in *recordListInput, kind string) (domain.RecordQuery, error) {
	scope, err := recordScope(ctx, in.TenantID, in.UserID)
	if err != nil {
		return domain.RecordQuery{}, err
	}
	from, to, err := parseOptionalRFC3339Window(in.From, in.To)
	if err != nil {
		return domain.RecordQuery{}, err
	}
	return domain.RecordQuery{RecordScope: scope, Kind: kind, Model: in.Model, Source: in.Source, From: from, To: to, Cursor: in.Cursor, Limit: in.Limit}, nil
}
func registerRequestRecords(api huma.API, d UsageHTTPDeps) {
	registerRecordDebug(api, d)
	group := huma.NewGroup(api)
	group.UseMiddleware(userAuth(api, d.Auth, "", "authentication required"))
	for _, spec := range []struct{ path, kind, id string }{{"/api/v2/requests", "requests", "ai-v2-requests"}, {"/api/v2/request-errors", "errors", "ai-v2-request-errors"}, {"/api/v2/billing/settlements", "settlements", "ai-v2-settlements"}} {
		huma.Register(group, huma.Operation{OperationID: spec.id, Method: http.MethodGet, Path: spec.path, Summary: "请求与独立消费记录", Tags: []string{"request-records"}}, func(ctx context.Context, in *recordListInput) (*recordListOutput, error) {
			if d.Records == nil {
				return nil, huma.Error503ServiceUnavailable("request records unavailable")
			}
			q, err := recordQuery(ctx, in, spec.kind)
			if err != nil {
				return nil, err
			}
			page, err := d.Records.Records(ctx, q)
			if err != nil {
				if errors.Is(err, domain.ErrInvalidRecordCursor) {
					return nil, huma.Error400BadRequest("invalid record cursor")
				}
				return nil, huma.Error500InternalServerError("unable to read records")
			}
			return &recordListOutput{Body: page}, nil
		})
	}
	for _, path := range []string{"/api/v2/requests/{requestID}", "/api/v2/billing/settlements/{requestID}"} {
		id := "ai-v2-request-detail"
		if path != "/api/v2/requests/{requestID}" {
			id = "ai-v2-settlement-detail"
		}
		huma.Register(group, huma.Operation{OperationID: id, Method: http.MethodGet, Path: path, Summary: "请求与费用详情", Tags: []string{"request-records"}}, func(ctx context.Context, in *recordDetailInput) (*recordDetailOutput, error) {
			if d.Records == nil {
				return nil, huma.Error503ServiceUnavailable("request records unavailable")
			}
			scope, err := recordScope(ctx, "", "")
			if err != nil {
				return nil, err
			}
			record, err := d.Records.Record(ctx, scope, in.RequestID)
			if errors.Is(err, domain.ErrNotFound) {
				return nil, huma.Error404NotFound("record not found")
			}
			if err != nil {
				return nil, huma.Error500InternalServerError("unable to read record")
			}
			return &recordDetailOutput{Body: record}, nil
		})
	}
	huma.Register(group, huma.Operation{OperationID: "ai-v2-record-summary", Method: http.MethodGet, Path: "/api/v2/request-summary", Summary: "用量与实际扣款日汇总", Tags: []string{"request-records"}}, func(ctx context.Context, in *recordListInput) (*recordSummaryOutput, error) {
		if d.Records == nil {
			return nil, huma.Error503ServiceUnavailable("request records unavailable")
		}
		q, err := recordQuery(ctx, in, "requests")
		if err != nil {
			return nil, err
		}
		r, err := d.Records.RecordSummary(ctx, q)
		if err != nil {
			return nil, huma.Error500InternalServerError("unable to read summary")
		}
		return &recordSummaryOutput{Body: r}, nil
	})
	admin := huma.NewGroup(api)
	admin.UseMiddleware(platformUserAuth(api, d.Auth))
	huma.Register(admin, huma.Operation{OperationID: "ai-v2-refund-settlement", Method: http.MethodPost, Path: "/api/v2/billing/settlements/{requestID}/refund", Summary: "全额退回实际费用与计费额度", Tags: []string{"request-records"}}, func(ctx context.Context, in *recordRefundInput) (*recordRefundOutput, error) {
		if d.Records == nil {
			return nil, huma.Error503ServiceUnavailable("settlements unavailable")
		}
		c := claimsFromContext(ctx)
		if c == nil {
			return nil, huma.Error401Unauthorized("authentication required")
		}
		if err := d.Records.Refund(ctx, in.RequestID, in.Body.Reason, c.UserID); err != nil {
			return nil, huma.Error409Conflict("settlement cannot be refunded")
		}
		out := &recordRefundOutput{}
		out.Body.Refunded = true
		return out, nil
	})
}

type recordDebugInput struct{ Body domain.DebugSessionInput }
type recordDebugOutput struct{ Body domain.DebugSession }
type recordDebugPayloadOutput struct{ Body json.RawMessage }

func registerRecordDebug(api huma.API, d UsageHTTPDeps) {
	group := huma.NewGroup(api)
	group.UseMiddleware(platformUserAuth(api, d.Auth))
	huma.Register(group, huma.Operation{OperationID: "ai-v2-debug-session", Method: http.MethodPost, Path: "/api/v2/request-debug-sessions", Summary: "限时开启指定范围的完整内容记录", Tags: []string{"request-records"}}, func(ctx context.Context, in *recordDebugInput) (*recordDebugOutput, error) {
		repo, ok := d.Records.(domain.DebugRecordsRepository)
		if !ok {
			return nil, huma.Error503ServiceUnavailable("debug storage unavailable")
		}
		claims := claimsFromContext(ctx)
		if claims == nil {
			return nil, huma.Error401Unauthorized("authentication required")
		}
		session, err := repo.StartDebug(ctx, in.Body, claims.UserID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid debug session", err)
		}
		return &recordDebugOutput{Body: session}, nil
	})
	huma.Register(group, huma.Operation{OperationID: "ai-v2-debug-payload", Method: http.MethodGet, Path: "/api/v2/requests/{requestID}/debug", Summary: "管理员读取限时调试内容", Tags: []string{"request-records"}}, func(ctx context.Context, in *recordDetailInput) (*recordDebugPayloadOutput, error) {
		repo, ok := d.Records.(domain.DebugRecordsRepository)
		if !ok {
			return nil, huma.Error503ServiceUnavailable("debug storage unavailable")
		}
		body, err := repo.DebugPayload(ctx, in.RequestID)
		if err != nil {
			return nil, huma.Error404NotFound("debug payload unavailable or expired")
		}
		return &recordDebugPayloadOutput{Body: body}, nil
	})
}
