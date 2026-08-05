package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type infoOutput struct {
	Body struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
}

// registerInfo 注册 GET /api/v1/info（服务元信息）。
func registerInfo(api huma.API, version string) {
	huma.Register(api, huma.Operation{
		OperationID: "get-info",
		Method:      http.MethodGet,
		Path:        "/api/v1/info",
		Summary:     "服务信息",
		Tags:        []string{"meta"},
	}, func(_ context.Context, _ *struct{}) (*infoOutput, error) {
		out := &infoOutput{}
		out.Body.Name = "URM"
		out.Body.Version = version
		return out, nil
	})
}
