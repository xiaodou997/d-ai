package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/zap"
	"xiaodou/dai/internal/ai/audit"
)

type RecordingSettingsStore interface {
	Get(context.Context) (audit.RecordingSettings, error)
	Update(context.Context, audit.RecordingSettings) (audit.RecordingSettings, error)
}

type recordingSettingsOutput struct{ Body audit.RecordingSettings }
type recordingSettingsInput struct{ Body audit.RecordingSettings }

func registerRecordingSettings(api huma.API, store RecordingSettingsStore) {
	huma.Register(api, huma.Operation{OperationID: "admin-get-request-recording", Method: http.MethodGet, Path: "/api/v1/admin/modules/request-recording/config", Summary: "读取请求记录设置", Tags: []string{"system-modules"}}, func(ctx context.Context, _ *struct{}) (*recordingSettingsOutput, error) {
		if store == nil {
			return nil, huma.Error503ServiceUnavailable("请求记录设置不可用")
		}
		c, err := store.Get(ctx)
		if err != nil {
			zap.L().Error("read request recording settings", zap.Error(err))
			return nil, huma.Error500InternalServerError("读取请求记录设置失败")
		}
		return &recordingSettingsOutput{Body: c}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "admin-update-request-recording", Method: http.MethodPut, Path: "/api/v1/admin/modules/request-recording/config", Summary: "保存请求记录设置", Tags: []string{"system-modules"}}, func(ctx context.Context, in *recordingSettingsInput) (*recordingSettingsOutput, error) {
		if store == nil {
			return nil, huma.Error503ServiceUnavailable("请求记录设置不可用")
		}
		c, err := store.Update(ctx, in.Body)
		if err != nil {
			if errors.Is(err, audit.ErrInvalidRecordingSettings) {
				return nil, huma.Error400BadRequest("请求记录设置无效")
			}
			zap.L().Error("update request recording settings", zap.Error(err))
			return nil, huma.Error500InternalServerError("保存请求记录设置失败")
		}
		return &recordingSettingsOutput{Body: c}, nil
	})
}
