package server

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// HealthOutput 是健康检查的强类型响应体。
type HealthOutput struct {
	Body struct {
		Status  string `json:"status" doc:"固定为 ok" example:"ok"`
		Service string `json:"service" doc:"服务名"`
		Version string `json:"version" doc:"服务版本"`
	}
}

// Health 在 api 上注册强类型健康检查端点 GET /healthz。各服务启动后调用本函数即可
// 获得统一的存活探针，并作为 code-first OpenAPI 的最小契约样例。
func Health(api huma.API, service, version string) {
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "健康检查",
		Description: "返回服务存活状态，用于探针与冒烟。",
		Tags:        []string{"meta"},
	}, func(_ context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		out.Body.Service = service
		out.Body.Version = version
		return out, nil
	})
}

// OpenAPIYAML 导出 api 的 OpenAPI 3.1 契约（YAML）。供各服务的导出命令
// （如 cmd/openapi）写入 api/openapi.yaml，作为前端生成 TS client 的来源。
func OpenAPIYAML(api huma.API) ([]byte, error) {
	return api.OpenAPI().YAML()
}
