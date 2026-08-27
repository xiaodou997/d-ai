package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	cleanuppkg "xiaodou/dai/internal/cleanup"
	"xiaodou/dai/libs/go/httpx"
)

type dataCleanupPolicyOutput struct{ Body cleanuppkg.Policy }
type dataCleanupPreviewOutput struct{ Body cleanuppkg.Preview }
type dataCleanupRunsOutput struct{ Body []cleanuppkg.Run }
type dataCleanupRunOutput struct{ Body cleanuppkg.Run }

type dataCleanupPolicyInput struct{ Body cleanuppkg.Policy }
type dataCleanupRunInput struct {
	Body struct {
		Targets      []string `json:"targets" minItems:"1"`
		Confirmation string   `json:"confirmation"`
	}
}

func registerDataCleanup(api huma.API, d dataCleanupModule) {
	admin := huma.Middlewares{userAuth(api, d.auth.JWT, d.auth.Blacklist), requireCapability(api, auth.CapabilityPlatformAdmin)}

	huma.Register(api, huma.Operation{
		OperationID: "admin-get-data-cleanup-policy",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/data-cleanup/policy",
		Summary:     "读取数据清理策略",
		Tags:        []string{"data-cleanup"},
		Middlewares: admin,
	}, func(ctx context.Context, _ *struct{}) (*dataCleanupPolicyOutput, error) {
		policy, err := d.service.GetPolicy(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &dataCleanupPolicyOutput{Body: policy}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-update-data-cleanup-policy",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/data-cleanup/policy",
		Summary:     "更新数据清理策略",
		Tags:        []string{"data-cleanup"},
		Middlewares: admin,
	}, func(ctx context.Context, in *dataCleanupPolicyInput) (*dataCleanupPolicyOutput, error) {
		policy, err := d.service.UpdatePolicy(ctx, in.Body, actorID(ctx))
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		return &dataCleanupPolicyOutput{Body: policy}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-preview-data-cleanup",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/data-cleanup/preview",
		Summary:     "预览数据清理范围",
		Tags:        []string{"data-cleanup"},
		Middlewares: admin,
	}, func(ctx context.Context, _ *struct{}) (*dataCleanupPreviewOutput, error) {
		preview, err := d.service.Preview(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &dataCleanupPreviewOutput{Body: preview}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-list-data-cleanup-runs",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/data-cleanup/runs",
		Summary:     "查看数据清理运行记录",
		Tags:        []string{"data-cleanup"},
		Middlewares: admin,
	}, func(ctx context.Context, _ *struct{}) (*dataCleanupRunsOutput, error) {
		runs, err := d.service.ListRuns(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &dataCleanupRunsOutput{Body: runs}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "admin-start-data-cleanup",
		Method:        http.MethodPost,
		Path:          "/api/v1/admin/data-cleanup/runs",
		Summary:       "手动执行数据清理",
		Tags:          []string{"data-cleanup"},
		Middlewares:   admin,
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *dataCleanupRunInput) (*dataCleanupRunOutput, error) {
		if in.Body.Confirmation != cleanuppkg.ConfirmationPhrase {
			return nil, httpx.ErrBadRequest.WithDetail("请输入正确的确认短语：" + cleanuppkg.ConfirmationPhrase)
		}
		run, err := d.service.StartManual(in.Body.Targets, actorID(ctx))
		if errors.Is(err, cleanuppkg.ErrAlreadyRunning) {
			return nil, httpx.ErrConflict.WithDetail("已有数据清理任务正在执行")
		}
		if errors.Is(err, cleanuppkg.ErrInvalidTarget) {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &dataCleanupRunOutput{Body: run}, nil
	})
}
