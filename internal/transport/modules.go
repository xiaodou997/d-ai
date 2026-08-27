package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/privacy"
	"xiaodou/dai/internal/auth"
	systempkg "xiaodou/dai/internal/system"
	"xiaodou/dai/libs/go/httpx"
)

type moduleStatusOutput struct {
	Body systempkg.Status
}

type moduleStatusesOutput struct {
	Body []systempkg.Status
}

type moduleNameInput struct {
	Name string `path:"name"`
}

type moduleEnabledInput struct {
	Name string `path:"name"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

type piiConfigOutput struct {
	Body systempkg.PIIConfig
}

type updatePIIConfigInput struct {
	Body systempkg.PIIConfig
}

type previewPIIConfigInput struct {
	Body struct {
		Config systempkg.PIIConfig `json:"config"`
		Text   string              `json:"text" maxLength:"10000"`
	}
}

type previewPIIConfigOutput struct {
	Body struct {
		ProtectedText string `json:"protectedText"`
	}
}

func registerModules(api huma.API, d systemModule) {
	admin := huma.Middlewares{userAuth(api, d.auth.JWT, d.auth.Blacklist), requireCapability(api, auth.CapabilityPlatformAdmin)}

	huma.Register(api, huma.Operation{
		OperationID: "admin-list-modules",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/modules",
		Summary:     "模块状态",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, _ *struct{}) (*moduleStatusesOutput, error) {
		items, err := d.service.List(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &moduleStatusesOutput{Body: items}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-get-pii-protection-config",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/modules/pii-protection/config",
		Summary:     "敏感信息保护配置",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, _ *struct{}) (*piiConfigOutput, error) {
		config, err := d.service.GetPIIConfig(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &piiConfigOutput{Body: config}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-get-pii-protection-defaults",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/modules/pii-protection/defaults",
		Summary:     "敏感信息保护默认配置",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(context.Context, *struct{}) (*piiConfigOutput, error) {
		return &piiConfigOutput{Body: systempkg.DefaultPIIConfig()}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-update-pii-protection-config",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/modules/pii-protection/config",
		Summary:     "更新敏感信息保护配置",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, in *updatePIIConfigInput) (*piiConfigOutput, error) {
		actor := ""
		if claims := userClaimsFromCtx(ctx); claims != nil {
			actor = claims.UserID
		}
		config, err := d.service.UpdatePIIConfig(ctx, in.Body, actor)
		if errors.Is(err, systempkg.ErrModuleConfigInvalid) {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &piiConfigOutput{Body: config}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-preview-pii-protection",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/modules/pii-protection/preview",
		Summary:     "预览敏感信息替换结果",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(_ context.Context, in *previewPIIConfigInput) (*previewPIIConfigOutput, error) {
		protector, err := privacy.NewProtectorWithConfig(privacy.Config{
			Rules:             in.Body.Config.Rules,
			PlaceholderPrefix: in.Body.Config.PlaceholderPrefix,
		})
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		protected, _ := protector.RedactText([]byte(in.Body.Text))
		out := &previewPIIConfigOutput{}
		out.Body.ProtectedText = string(protected)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-get-module",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/modules/{name}",
		Summary:     "模块详情",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, in *moduleNameInput) (*moduleStatusOutput, error) {
		item, err := d.service.Get(ctx, in.Name)
		if errors.Is(err, systempkg.ErrUnknownModule) {
			return nil, httpx.ErrNotFound.WithDetail("模块不存在")
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &moduleStatusOutput{Body: item}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-set-module-enabled",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/modules/{name}/enabled",
		Summary:     "启用或禁用模块",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, in *moduleEnabledInput) (*moduleStatusOutput, error) {
		claims := userClaimsFromCtx(ctx)
		actor := ""
		if claims != nil {
			actor = claims.UserID
		}
		item, err := d.service.SetEnabled(ctx, in.Name, in.Body.Enabled, actor)
		if errors.Is(err, systempkg.ErrUnknownModule) {
			return nil, httpx.ErrNotFound.WithDetail("模块不存在")
		}
		if errors.Is(err, systempkg.ErrModuleConfigInvalid) {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &moduleStatusOutput{Body: item}, nil
	})
}
