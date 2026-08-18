package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

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

func registerModules(api huma.API, d Deps) {
	if d.Modules == nil {
		return
	}
	admin := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireUserType(api, 1, 2)}

	huma.Register(api, huma.Operation{
		OperationID: "admin-list-modules",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/modules",
		Summary:     "模块状态",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, _ *struct{}) (*moduleStatusesOutput, error) {
		items, err := d.Modules.List(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &moduleStatusesOutput{Body: items}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-get-module",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/modules/{name}",
		Summary:     "模块详情",
		Tags:        []string{"modules"},
		Middlewares: admin,
	}, func(ctx context.Context, in *moduleNameInput) (*moduleStatusOutput, error) {
		item, err := d.Modules.Get(ctx, in.Name)
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
		item, err := d.Modules.SetEnabled(ctx, in.Name, in.Body.Enabled, actor)
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
