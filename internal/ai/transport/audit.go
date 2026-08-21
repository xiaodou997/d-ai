package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

type auditLogsInput struct {
	Limit int32 `query:"limit" default:"100" doc:"返回条数；默认 100，最大 500"`
}

type auditLogDTO struct {
	ID             string          `json:"id" doc:"审计日志 ID"`
	Actor          *string         `json:"actor,omitempty" doc:"操作者"`
	Action         string          `json:"action" doc:"动作"`
	ObjectType     *string         `json:"object_type,omitempty" doc:"对象类型"`
	ObjectID       *string         `json:"object_id,omitempty" doc:"对象 ID"`
	RequestSummary json.RawMessage `json:"request_summary" doc:"请求摘要 JSON"`
	Result         string          `json:"result" doc:"结果"`
	HTTPStatus     *int32          `json:"http_status,omitempty" doc:"HTTP 状态码"`
	CreatedAt      *int64          `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
}

type auditLogsOutput struct {
	Body struct {
		Items []auditLogDTO `json:"items"`
		Total int           `json:"total"`
	}
}

func registerAudit(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-audit-logs",
		Method:      http.MethodGet,
		Path:        "/api/v1/audit-logs",
		Summary:     "管理审计日志列表",
		Description: "返回 AI 管理面审计日志，按创建时间倒序。limit 默认 100，最大 500。",
		Tags:        []string{"audit"},
	}, func(ctx context.Context, in *auditLogsInput) (*auditLogsOutput, error) {
		if d.AuditLogs == nil {
			return nil, httpx.ErrUnavailable.WithDetail("audit service is not configured")
		}
		limit, err := auditLimitFromInput(in.Limit)
		if err != nil {
			return nil, err
		}
		logs, err := d.AuditLogs.List(ctx, limit)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &auditLogsOutput{}
		out.Body.Items = make([]auditLogDTO, 0, len(logs))
		for _, log := range logs {
			out.Body.Items = append(out.Body.Items, auditLogToDTO(log))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

func auditLimitFromInput(limit int32) (int32, error) {
	if limit <= 0 {
		return 0, httpx.ErrBadRequest.WithDetail("invalid limit")
	}
	if limit > 500 {
		return 500, nil
	}
	return limit, nil
}

func auditLogToDTO(log domain.AuditLog) auditLogDTO {
	return auditLogDTO{
		ID:             log.ID,
		Actor:          stringPtrOrNil(log.Actor),
		Action:         log.Action,
		ObjectType:     stringPtrOrNil(log.ObjectType),
		ObjectID:       stringPtrOrNil(log.ObjectID),
		RequestSummary: jsonObjectOrNull(log.RequestSummary),
		Result:         log.Result,
		HTTPStatus:     log.HttpStatus,
		CreatedAt:      timeToMillisPtr(log.CreatedAt),
	}
}
