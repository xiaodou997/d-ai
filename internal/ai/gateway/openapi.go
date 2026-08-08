package gateway

import (
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

type AsyncTaskInput struct{}

type AsyncTaskCreateJSONRequest struct {
	Type       string         `json:"type" enum:"images.generation,images.edit,chat.completions" doc:"任务能力类型"`
	Input      AsyncTaskInput `json:"input" doc:"与对应同步 API 请求体逐字段同构；chat 强制非流式"`
	Metadata   map[string]any `json:"metadata,omitempty" doc:"调用方业务元数据；不参与执行与幂等指纹，创建和查询响应会原样回显"`
	WebhookURL string         `json:"webhook_url,omitempty" format:"uri" doc:"可选终态通知地址；必须是解析到公网地址的绝对 HTTPS URL"`
}

type AsyncTaskCreateMultipartRequest struct{}

type AsyncTaskCreateResponse struct {
	ID             string         `json:"id" format:"uuid"`
	Object         string         `json:"object" enum:"task"`
	Type           string         `json:"type" enum:"images.generation,images.edit,chat.completions"`
	Status         string         `json:"status" enum:"pending,running,completed,failed,cancelled"`
	Model          string         `json:"model"`
	IdempotencyKey string         `json:"idempotency_key"`
	Metadata       map[string]any `json:"metadata"`
	WebhookURL     string         `json:"webhook_url,omitempty" format:"uri"`
	CreatedAt      int64          `json:"created_at" doc:"Unix 时间戳（秒）"`
}

type AsyncTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AsyncTaskUsage struct {
	CostUSD float64 `json:"cost_usd" doc:"任务调用方的已结算消耗USD 金额"`
}

type AsyncTaskGetResponse struct {
	AsyncTaskCreateResponse
	Result      map[string]any  `json:"result"`
	Error       *AsyncTaskError `json:"error"`
	Usage       *AsyncTaskUsage `json:"usage"`
	RequestID   string          `json:"request_id"`
	Attempt     int             `json:"attempt"`
	StartedAt   *int64          `json:"started_at" doc:"Unix 时间戳（秒）"`
	CompletedAt *int64          `json:"completed_at" doc:"Unix 时间戳（秒）"`
}

type AsyncTaskListResponse struct {
	Object  string                 `json:"object" enum:"list"`
	Data    []AsyncTaskGetResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
}

type RuntimeAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

type RuntimeAPIErrorResponse struct {
	Error RuntimeAPIError `json:"error"`
}

func (AsyncTaskInput) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	generation := r.Schema(reflect.TypeFor[AsyncTaskImageGenerationInput](), true, "")
	edit := r.Schema(reflect.TypeFor[AsyncTaskImageEditInput](), true, "")
	chat := r.Schema(reflect.TypeFor[AsyncTaskChatCompletionInput](), true, "")
	s.Type = huma.TypeObject
	s.Description = "输入与对应同步 API 同构；chat.completions 强制 stream=false。"
	s.OneOf = []*huma.Schema{generation, edit, chat}
	return s
}

type AsyncTaskChatCompletionInput struct{}

func (AsyncTaskChatCompletionInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Description = "与 POST /v1/chat/completions 请求体同构；stream 固定为 false。"
	s.Properties = map[string]*huma.Schema{
		"model":    {Type: huma.TypeString, Description: "模型代码"},
		"messages": {Type: huma.TypeArray, Items: &huma.Schema{Type: huma.TypeObject, AdditionalProperties: true}},
		"stream":   {Type: huma.TypeBoolean, Default: false},
	}
	s.AdditionalProperties = true
	s.Required = []string{"model", "messages"}
	return s
}

type AsyncTaskImageGenerationInput struct {
	Model             string `json:"model" doc:"模型代码"`
	Prompt            string `json:"prompt" doc:"图片生成提示词"`
	N                 int    `json:"n,omitempty" minimum:"1" maximum:"10"`
	Size              string `json:"size,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty" enum:"b64_json,url"`
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Stream            bool   `json:"stream,omitempty" default:"false"`
	User              string `json:"user,omitempty"`
}

type AsyncTaskImageEditInput struct{}

func (AsyncTaskImageEditInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Description = "图生图 JSON 请求；也可使用 multipart/form-data 上传 image[] 文件。"
	s.Properties = map[string]*huma.Schema{
		"model":              {Type: huma.TypeString},
		"prompt":             {Type: huma.TypeString},
		"n":                  {Type: huma.TypeInteger, Minimum: openAPIFloat(1), Maximum: openAPIFloat(10)},
		"images":             {Type: huma.TypeArray, Items: runtimeImageSourceSchema(), MinItems: openAPIInt(1), MaxItems: openAPIInt(16)},
		"mask":               runtimeImageSourceSchema(),
		"size":               {Type: huma.TypeString},
		"response_format":    {Type: huma.TypeString, Enum: []any{"b64_json", "url"}},
		"background":         {Type: huma.TypeString},
		"input_fidelity":     {Type: huma.TypeString},
		"output_format":      {Type: huma.TypeString},
		"output_compression": {Type: huma.TypeInteger},
		"stream":             {Type: huma.TypeBoolean, Default: false},
		"user":               {Type: huma.TypeString},
	}
	s.Required = []string{"model", "images"}
	return s
}

func (AsyncTaskCreateMultipartRequest) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Description = "异步图生图 multipart 信封；其余字段与 POST /v1/images/edits 相同。"
	s.Properties = map[string]*huma.Schema{
		"type":               {Type: huma.TypeString, Enum: []any{"images.edit"}},
		"metadata":           {Type: huma.TypeString, Description: "JSON 对象字符串"},
		"webhook_url":        {Type: huma.TypeString, Format: "uri"},
		"model":              {Type: huma.TypeString},
		"n":                  {Type: huma.TypeInteger, Minimum: openAPIFloat(1), Maximum: openAPIFloat(10)},
		"prompt":             {Type: huma.TypeString},
		"image[]":            {Type: huma.TypeString, Format: "binary"},
		"mask":               {Type: huma.TypeString, Format: "binary"},
		"size":               {Type: huma.TypeString},
		"response_format":    {Type: huma.TypeString},
		"background":         {Type: huma.TypeString},
		"input_fidelity":     {Type: huma.TypeString},
		"output_format":      {Type: huma.TypeString},
		"output_compression": {Type: huma.TypeInteger},
		"stream":             {Type: huma.TypeBoolean, Default: false},
		"user":               {Type: huma.TypeString},
	}
	s.Required = []string{"type", "model", "image[]"}
	return s
}

func runtimeImageSourceSchema() *huma.Schema {
	return &huma.Schema{
		Type: huma.TypeObject,
		Properties: map[string]*huma.Schema{
			"image_url": {Type: huma.TypeString, Description: "HTTP(S) URL 或 data:image/...;base64,..."},
		},
		Required: []string{"image_url"},
	}
}

func openAPIInt(value int) *int { return &value }

func openAPIFloat(value float64) *float64 { return &value }

// RegisterOpenAPI documents chi-native runtime endpoints in the unified spec.
func RegisterOpenAPI(api huma.API) {
	doc := api.OpenAPI()
	if doc.Components == nil {
		doc.Components = &huma.Components{Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)}
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	registerAsyncTaskOpenAPI(doc, doc.Components.Schemas)
}

func registerAsyncTaskOpenAPI(doc *huma.OpenAPI, schemas huma.Registry) {
	doc.Components.SecuritySchemes["taskBearerAuth"] = &huma.SecurityScheme{
		Type: "http", Scheme: "bearer", BearerFormat: "API key",
		Description: "使用 Authorization: Bearer sk-ai-xxx 调用异步任务 API。",
	}
	jsonRequestSchema := schemas.Schema(reflect.TypeFor[AsyncTaskCreateJSONRequest](), true, "")
	multipartRequestSchema := schemas.Schema(reflect.TypeFor[AsyncTaskCreateMultipartRequest](), true, "")
	createResponseSchema := schemas.Schema(reflect.TypeFor[AsyncTaskCreateResponse](), true, "")
	getResponseSchema := schemas.Schema(reflect.TypeFor[AsyncTaskGetResponse](), true, "")
	listResponseSchema := schemas.Schema(reflect.TypeFor[AsyncTaskListResponse](), true, "")
	errSchema := schemas.Schema(reflect.TypeFor[RuntimeAPIErrorResponse](), true, "")
	security := []map[string][]string{{"taskBearerAuth": {}}}

	doc.AddOperation(&huma.Operation{
		Method: http.MethodPost, Path: "/v1/tasks", OperationID: "ai-create-task",
		Summary: "创建异步任务",
		Description: "使用 API key 创建图片生成、图片编辑或聊天完成任务。type 为必填字段；chat.completions 强制 stream=false，只保存最终响应。" +
			"相同凭据复用 Idempotency-Key 且执行输入相同时返回原任务，不同输入返回 409。metadata 不参与幂等指纹并原样回显。" +
			"可选 webhook_url 仅接受公网 HTTPS。任务进入终态时发送最小通知，body 只含 source=D-AI、event 和 task_id；" +
			"完整状态、结果及错误必须通过 GET /v1/tasks/{id} 获取。",
		Tags: []string{"runtime", "tasks"}, Security: security,
		Parameters: []*huma.Param{{
			Name: "Idempotency-Key", In: "header", Required: false,
			Description: "可选幂等键；作用域按当前 API key 隔离。", Schema: &huma.Schema{Type: huma.TypeString},
		}},
		RequestBody: &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{
			"application/json":    {Schema: jsonRequestSchema},
			"multipart/form-data": {Schema: multipartRequestSchema},
		}},
		Responses: map[string]*huma.Response{
			"202": {Description: "任务已持久化并进入 pending 状态。", Content: map[string]*huma.MediaType{"application/json": {Schema: createResponseSchema}}},
			"400": taskOpenAPIErrorResponse("信封、任务类型或执行输入不合法", errSchema),
			"401": taskOpenAPIErrorResponse("API key 无效、禁用或过期", errSchema),
			"402": taskOpenAPIErrorResponse("额度、余额或订阅准入失败", errSchema),
			"403": taskOpenAPIErrorResponse("模型、分组或服务访问未授权", errSchema),
			"409": taskOpenAPIErrorResponse("Idempotency-Key 已用于不同执行输入", errSchema),
			"429": taskOpenAPIErrorResponse("租户任务达到并发上限", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})

	doc.AddOperation(&huma.Operation{
		Method: http.MethodGet, Path: "/v1/tasks", OperationID: "ai-list-tasks",
		Summary: "列出异步任务", Tags: []string{"runtime", "tasks"}, Security: security,
		Parameters: []*huma.Param{
			{Name: "status", In: "query", Schema: &huma.Schema{Type: huma.TypeString, Enum: []any{"pending", "running", "completed", "failed", "cancelled"}}},
			{Name: "type", In: "query", Schema: &huma.Schema{Type: huma.TypeString, Enum: []any{"images.generation", "images.edit", "chat.completions"}}},
			{Name: "limit", In: "query", Schema: &huma.Schema{Type: huma.TypeInteger, Minimum: openAPIFloat(1), Maximum: openAPIFloat(100), Default: 20}},
			{Name: "starting_after", In: "query", Schema: &huma.Schema{Type: huma.TypeString, Format: "uuid"}},
		},
		Responses: map[string]*huma.Response{
			"200": {Description: "任务列表。", Content: map[string]*huma.MediaType{"application/json": {Schema: listResponseSchema}}},
			"400": taskOpenAPIErrorResponse("过滤条件或游标不合法", errSchema),
			"401": taskOpenAPIErrorResponse("API key 无效、禁用或过期", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})

	doc.AddOperation(&huma.Operation{
		Method: http.MethodGet, Path: "/v1/tasks/{taskID}", OperationID: "ai-get-task",
		Summary: "查询异步任务", Tags: []string{"runtime", "tasks"}, Security: security,
		Parameters: []*huma.Param{{Name: "taskID", In: "path", Required: true, Schema: &huma.Schema{Type: huma.TypeString, Format: "uuid"}}},
		Responses: map[string]*huma.Response{
			"200": {Description: "任务当前状态及终态结果。", Content: map[string]*huma.MediaType{"application/json": {Schema: getResponseSchema}}},
			"401": taskOpenAPIErrorResponse("API key 无效、禁用或过期", errSchema),
			"404": taskOpenAPIErrorResponse("任务不存在或不可见", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})

	doc.AddOperation(&huma.Operation{
		Method: http.MethodPost, Path: "/v1/tasks/{taskID}/cancel", OperationID: "ai-cancel-task",
		Summary: "取消异步任务", Tags: []string{"runtime", "tasks"}, Security: security,
		Parameters: []*huma.Param{{Name: "taskID", In: "path", Required: true, Schema: &huma.Schema{Type: huma.TypeString, Format: "uuid"}}},
		Responses: map[string]*huma.Response{
			"200": {Description: "取消后的任务对象。", Content: map[string]*huma.MediaType{"application/json": {Schema: getResponseSchema}}},
			"401": taskOpenAPIErrorResponse("API key 无效、禁用或过期", errSchema),
			"404": taskOpenAPIErrorResponse("任务不存在或不可见", errSchema),
			"409": taskOpenAPIErrorResponse("任务已处于终态", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})
}

func taskOpenAPIErrorResponse(description string, schema *huma.Schema) *huma.Response {
	return &huma.Response{Description: description, Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
}
