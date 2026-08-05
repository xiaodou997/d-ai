package gateway

import (
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

type RunRuntimeRequest struct {
	Input       string                 `json:"input" doc:"本次用户输入文本"`
	Variables   map[string]string      `json:"variables,omitempty" doc:"可选；用于替换应用提示词中的 {{变量名}} 占位符"`
	Attachments []RunRuntimeAttachment `json:"attachments,omitempty" doc:"可选；图片或文件直连 URL 数组，仅应用允许附件时可用"`
	Stream      bool                   `json:"stream,omitempty" default:"false" doc:"true 时返回 text/event-stream，false 时返回 JSON"`
	Temperature *float64               `json:"temperature,omitempty" doc:"应用锁定该参数；一旦传值会返回 400，不会被采纳"`
	MaxTokens   *int                   `json:"max_tokens,omitempty" doc:"应用锁定该参数；一旦传值会返回 400，不会被采纳"`
}

type RunRuntimeAttachment struct {
	Type     string `json:"type,omitempty" enum:"image,file" doc:"附件类型：image 或 file"`
	URL      string `json:"url" doc:"HTTPS 直连 URL 或平台认可的签名 URL"`
	Name     string `json:"name,omitempty" doc:"文件名，可选"`
	MIMEType string `json:"mime_type,omitempty" doc:"MIME 类型，可选"`
}

type RunRuntimeImageGenerationRequest struct {
	Input          string            `json:"input" doc:"本次文生图输入；分辨率等运行设置由应用固定"`
	Variables      map[string]string `json:"variables,omitempty" doc:"可选；用于替换应用提示词中的 {{变量名}} 占位符"`
	N              int               `json:"n,omitempty" minimum:"1" maximum:"10" doc:"可选；输出图片张数。省略时使用应用默认值，是否可覆盖及上限由应用配置决定"`
	Stream         bool              `json:"stream,omitempty" default:"false" doc:"true 时返回 text/event-stream；仅流式返回最终完成图，不暴露 partial_images 预览事件"`
	Size           string            `json:"size,omitempty" doc:"可选；图片尺寸，例如 1024x1024"`
	ResponseFormat string            `json:"response_format,omitempty" doc:"可选；b64_json 或 url"`
	Background     string            `json:"background,omitempty" doc:"可选；背景配置"`
	OutputFormat   string            `json:"output_format,omitempty" doc:"可选；输出格式"`
}

type RunRuntimeImageEditJSONRequest struct{}
type RunRuntimeImageEditMultipartRequest struct{}

type AsyncTaskInput struct{}

type AsyncTaskCreateJSONRequest struct {
	Type       string         `json:"type,omitempty" enum:"images.generation,images.edit,chat.completions" doc:"任务能力类型；API key 必填，app key 可省略并由绑定应用推断"`
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
	CostCredits float64 `json:"cost_credits" doc:"任务调用方的已结算消耗积分；失败或取消前已产生可计费用量时可大于 0"`
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

type RunRuntimeError struct {
	Code    string `json:"code" doc:"稳定错误码，例如 invalid_request_error、invalid_api_key、rate_limit_exceeded、service_unavailable"`
	Message string `json:"message" doc:"面向调用方的错误信息"`
	Type    string `json:"type" doc:"OpenAI 风格错误类型"`
}

type RunRuntimeErrorResponse struct {
	Error RunRuntimeError `json:"error" doc:"错误详情"`
}

type RunRuntimeChatResponse struct{}
type RunRuntimeImageResponse struct{}

func (RunRuntimeChatResponse) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Properties = map[string]*huma.Schema{}
	s.AdditionalProperties = true
	s.Description = "统一应用输出：type=chat、text、usage、request_id。"
	return s
}

func (RunRuntimeImageResponse) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Properties = map[string]*huma.Schema{}
	s.AdditionalProperties = true
	s.Description = "统一应用输出：type=image、images[]、usage、request_id。请求 url 且上游返回内联图片时，url 为平台默认有效 24 小时的 capability URL，并附 asset_ref 与 expires_at；上游已有 HTTP URL 时原样透传。"
	return s
}

func (RunRuntimeImageEditJSONRequest) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Description = "应用密钥图生图 JSON 请求。图片来源使用 images[].image_url；image_url 可为 HTTP(S) URL 或 base64 data URL。未列出的 JSON 属性不参与请求映射。"
	s.Properties = map[string]*huma.Schema{
		"input":           {Type: huma.TypeString, Description: "本次图生图输入；动态提示词应用可在这里使用 {{提示词名称}}"},
		"variables":       {Type: huma.TypeObject, AdditionalProperties: &huma.Schema{Type: huma.TypeString}, Description: "可选；用于替换应用提示词中的 {{变量名}} 占位符"},
		"n":               {Type: huma.TypeInteger, Minimum: openAIFloat(1), Maximum: openAIFloat(10), Description: "可选；输出图片张数。省略时使用应用默认值，是否可覆盖及上限由应用配置决定"},
		"images":          {Type: huma.TypeArray, Items: runRuntimeImageSourceSchema(), MinItems: openAPIInt(1), MaxItems: openAPIInt(16), Description: "参考图数组；每项只包含 image_url。"},
		"mask":            runRuntimeImageSourceSchema(),
		"stream":          {Type: huma.TypeBoolean, Default: false, Description: "true 时返回 text/event-stream；仅流式返回最终完成图，不暴露 partial_images 预览事件"},
		"response_format": {Type: huma.TypeString, Description: "可选；控制平台返回 b64_json 或 url，不发送给 GPT Image 上游"},
	}
	s.Required = []string{"input", "images"}
	return s
}

func (RunRuntimeImageEditMultipartRequest) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Description = "应用密钥图生图 multipart 请求。使用一个或多个重复的 image[] 文件字段；可选 mask 文件。不接受 URL 文本字段或其它文件字段别名。"
	s.Properties = map[string]*huma.Schema{
		"input":           {Type: huma.TypeString, Description: "本次图生图输入；动态提示词应用可在这里使用 {{提示词名称}}"},
		"variables":       {Type: huma.TypeString, Description: "可选；JSON 字符串，用于替换应用提示词中的 {{变量名}} 占位符"},
		"n":               {Type: huma.TypeInteger, Minimum: openAIFloat(1), Maximum: openAIFloat(10), Description: "可选；输出图片张数。省略时使用应用默认值，是否可覆盖及上限由应用配置决定"},
		"image[]":         {Type: huma.TypeString, Format: "binary", Description: "参考图文件；多图时重复提交该字段。"},
		"mask":            {Type: huma.TypeString, Format: "binary", Description: "可选；上传蒙版文件"},
		"stream":          {Type: huma.TypeBoolean, Default: false, Description: "true 时返回 text/event-stream；仅流式返回最终完成图，不暴露 partial_images 预览事件"},
		"response_format": {Type: huma.TypeString, Description: "可选；控制平台返回 b64_json 或 url，不发送给 GPT Image 上游"},
	}
	s.Required = []string{"input", "image[]"}
	return s
}

func (AsyncTaskInput) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	generation := r.Schema(reflect.TypeFor[AsyncTaskImageGenerationInput](), true, "")
	edit := r.Schema(reflect.TypeFor[AsyncTaskImageEditInput](), true, "")
	appGeneration := r.Schema(reflect.TypeFor[RunRuntimeImageGenerationRequest](), true, "")
	appEdit := r.Schema(reflect.TypeFor[RunRuntimeImageEditJSONRequest](), true, "")
	chat := r.Schema(reflect.TypeFor[AsyncTaskChatCompletionInput](), true, "")
	appChat := r.Schema(reflect.TypeFor[RunRuntimeRequest](), true, "")
	s.Type = huma.TypeObject
	s.Description = "API key 输入与对应同步 API 同构；app key 输入与 POST /v1/run 的绑定应用请求同构。chat.completions 总是强制 stream=false，只产出最终响应。"
	s.OneOf = []*huma.Schema{generation, edit, appGeneration, appEdit, chat, appChat}
	return s
}

type AsyncTaskChatCompletionInput struct{}

func (AsyncTaskChatCompletionInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Type = huma.TypeObject
	s.Description = "与 POST /v1/chat/completions 请求体同构；stream 即使传 true 也会被规范化为 false。"
	s.Properties = map[string]*huma.Schema{
		"model":    {Type: huma.TypeString, Description: "模型代码"},
		"messages": {Type: huma.TypeArray, Items: &huma.Schema{Type: huma.TypeObject, AdditionalProperties: true}, Description: "OpenAI Chat Completions messages"},
		"stream":   {Type: huma.TypeBoolean, Default: false, Description: "异步任务固定为 false"},
	}
	s.AdditionalProperties = true
	s.Required = []string{"model", "messages"}
	return s
}

type AsyncTaskImageGenerationInput struct {
	Model             string `json:"model" doc:"模型代码"`
	Prompt            string `json:"prompt" doc:"图片生成提示词"`
	N                 int    `json:"n,omitempty" minimum:"1" maximum:"10" doc:"输出图片张数；还需满足上游模型绑定上限"`
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
	s.Description = "图生图 JSON 请求；也可改用 multipart/form-data 直接上传 image[] 文件。"
	s.Properties = map[string]*huma.Schema{
		"model":              {Type: huma.TypeString, Description: "模型代码"},
		"prompt":             {Type: huma.TypeString, Description: "编辑提示词"},
		"n":                  {Type: huma.TypeInteger, Minimum: openAIFloat(1), Maximum: openAIFloat(10), Description: "输出图片张数；还需满足上游模型绑定上限"},
		"images":             {Type: huma.TypeArray, Items: runRuntimeImageSourceSchema(), MinItems: openAPIInt(1), MaxItems: openAPIInt(16)},
		"mask":               runRuntimeImageSourceSchema(),
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
	s.Description = "异步图生图 multipart 信封。type、metadata 和 webhook_url 是普通标量字段，其余字段与 POST /v1/images/edits 相同。"
	s.Properties = map[string]*huma.Schema{
		"type":               {Type: huma.TypeString, Enum: []any{"images.edit"}},
		"metadata":           {Type: huma.TypeString, Description: "JSON 对象字符串"},
		"webhook_url":        {Type: huma.TypeString, Format: "uri", Description: "可选终态 HTTPS 通知地址"},
		"model":              {Type: huma.TypeString},
		"n":                  {Type: huma.TypeInteger, Minimum: openAIFloat(1), Maximum: openAIFloat(10)},
		"prompt":             {Type: huma.TypeString},
		"image[]":            {Type: huma.TypeString, Format: "binary", Description: "参考图文件；多图时重复提交该字段"},
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
	s.Required = []string{"image[]"}
	return s
}

func runRuntimeImageSourceSchema() *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeObject,
		Description: "图片来源对象；当前只映射 image_url。",
		Properties: map[string]*huma.Schema{
			"image_url": {Type: huma.TypeString, Description: "HTTP(S) URL 或 data:image/...;base64,..."},
		},
		Required: []string{"image_url"},
	}
}

func openAPIInt(value int) *int {
	return &value
}

func openAIFloat(value float64) *float64 {
	return &value
}

func openAPIFloat(value float64) *float64 {
	return &value
}

// RegisterOpenAPI adds runtime-plane documentation that is served by chi
// handlers instead of Huma management handlers.
func RegisterOpenAPI(api huma.API) {
	doc := api.OpenAPI()
	if doc.Components == nil {
		doc.Components = &huma.Components{
			Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer),
		}
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	doc.Components.SecuritySchemes["bearerAuth"] = &huma.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "应用密钥",
		Description:  "使用 Authorization: Bearer rk_xxx 调用 /v1/run；密钥绑定的应用类型决定请求体形状与响应形式。",
	}

	schemas := doc.Components.Schemas
	reqSchema := schemas.Schema(reflect.TypeFor[RunRuntimeRequest](), true, "")
	chatSchema := schemas.Schema(reflect.TypeFor[RunRuntimeChatResponse](), true, "")
	imageReqSchema := schemas.Schema(reflect.TypeFor[RunRuntimeImageGenerationRequest](), true, "")
	imageEditJSONSchema := schemas.Schema(reflect.TypeFor[RunRuntimeImageEditJSONRequest](), true, "")
	imageEditMultipartSchema := schemas.Schema(reflect.TypeFor[RunRuntimeImageEditMultipartRequest](), true, "")
	imageSchema := schemas.Schema(reflect.TypeFor[RunRuntimeImageResponse](), true, "")
	errSchema := schemas.Schema(reflect.TypeFor[RunRuntimeErrorResponse](), true, "")

	// A single app key is bound to exactly one app, so its app type (chat /
	// image_generation / image_edit) unambiguously determines the request and
	// response shape. All three are documented on this one operation via
	// oneOf rather than three separate paths.
	doc.AddOperation(&huma.Operation{
		Method:      http.MethodPost,
		Path:        "/v1/run",
		OperationID: "ai-run-runtime",
		Summary:     "应用密钥统一运行入口",
		Description: "使用 rk_ 应用密钥调用预先绑定好的应用；密钥绑定的应用类型（对话/文生图/图生图）决定请求体形状与响应形式，调用方无需选择路径。图生图额外支持 multipart/form-data 上传参考图。生图张数由应用设置默认值与可覆盖上限；支持 stream=true 返回 text/event-stream，但只返回最终完成图，不支持 partial_images 预览图。",
		Tags:        []string{"runtime"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: &huma.Schema{
						Description: "按密钥绑定的应用类型解析请求；对话、文生图和图生图统一使用 input。",
						OneOf:       []*huma.Schema{reqSchema, imageReqSchema, imageEditJSONSchema},
					},
					Examples: map[string]*huma.Example{
						"bound-chat-app": {
							Summary: "调用绑定对话应用的应用密钥并传入模板变量",
							Value: map[string]any{
								"input": "请帮我生成一段面向租户公告的说明",
								"variables": map[string]any{
									"tenant_name": "Demo Tenant",
								},
								"stream": true,
							},
						},
						"bound-dynamic-chat-app": {
							Summary: "按已绑定提示词名称动态组合输入",
							Value: map[string]any{
								"input": "客户想了解交付周期，请结合 {{客户背景}}，遵循 {{售前规范}}，参考 {{产品资料}}。",
							},
						},
						"bound-image-generation-app": {
							Summary: "调用绑定文生图应用的应用密钥",
							Value: map[string]any{
								"stream": true,
								"input":  "增加一点现代品牌视觉感",
								"variables": map[string]any{
									"brand_name": "Demo Tenant",
								},
							},
						},
						"bound-image-edit-app-single-reference": {
							Summary: "调用绑定图生图应用的应用密钥（单张远程参考图）",
							Value: map[string]any{
								"input":           "Retouch the lighting and keep the composition.",
								"images":          []any{map[string]any{"image_url": "https://example.com/reference.png"}},
								"stream":          false,
								"response_format": "b64_json",
							},
						},
						"bound-image-edit-app-multi-reference": {
							Summary: "调用绑定图生图应用的应用密钥（多张参考图）",
							Value: map[string]any{
								"input": "Use the first image composition and the second image color palette.",
								"images": []any{
									map[string]any{"image_url": "https://example.com/layout.png"},
									map[string]any{"image_url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAUA"},
								},
								"stream": true,
							},
						},
					},
				},
				"multipart/form-data": {
					Schema: imageEditMultipartSchema,
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "非流式时返回该应用类型对应的响应体（对话为 Chat Completions 风格 JSON，图片为 Images 风格 JSON，仅含最终完成图）；流式时返回 SSE。",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: &huma.Schema{OneOf: []*huma.Schema{chatSchema, imageSchema}},
						Examples: map[string]*huma.Example{
							"chat-completion": {
								Summary: "非流式对话响应",
								Value: map[string]any{
									"id":      "chatcmpl-run-example",
									"object":  "chat.completion",
									"created": 1761200000,
									"model":   "gpt-5.4",
									"choices": []map[string]any{
										{
											"index": 0,
											"message": map[string]any{
												"role":    "assistant",
												"content": "这段需求的核心是统一运行入口，并把提示词逻辑下沉到服务端。",
											},
											"finish_reason": "stop",
										},
									},
									"usage": map[string]any{
										"prompt_tokens":     42,
										"completion_tokens": 24,
										"total_tokens":      66,
									},
								},
							},
							"image-response": {
								Summary: "非流式图片响应",
								Value: map[string]any{
									"created": 1761200000,
									"data": []map[string]any{
										{"url": "https://example.com/final.png"},
									},
								},
							},
						},
					},
					"text/event-stream": {
						Schema:  &huma.Schema{Type: huma.TypeString},
						Example: "data: {\"id\":\"chatcmpl-run-example\",\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n",
					},
				},
			},
			"400": {
				Description: "请求体错误、输入缺失、应用目标无效或请求了应用不支持的预览图",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: errSchema,
						Examples: map[string]*huma.Example{
							"invalid-body": {
								Summary: "请求体不合法",
								Value:   runRuntimeErrorExample("invalid_request_error", "Invalid request body.", "invalid_request_error"),
							},
							"missing-input": {
								Summary: "缺少 input",
								Value:   runRuntimeErrorExample("invalid_request_error", "input is required.", "invalid_request_error"),
							},
							"partial-images-unsupported": {
								Summary: "不支持预览图",
								Value:   runRuntimeErrorExample("invalid_request_error", openAIImagePartialImagesUnsupportedMessage, "invalid_request_error"),
							},
						},
					},
				},
			},
			"401": {
				Description: "应用密钥缺失、格式错误、禁用或已过期",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: errSchema,
						Examples: map[string]*huma.Example{
							"invalid-key": {
								Summary: "应用密钥无效",
								Value:   runRuntimeErrorExample("invalid_api_key", "Invalid run key.", "invalid_api_key"),
							},
							"disabled-key": {
								Summary: "应用密钥已停用",
								Value:   runRuntimeErrorExample("invalid_api_key", "Run key disabled.", "invalid_api_key"),
							},
							"expired-key": {
								Summary: "应用密钥已过期",
								Value:   runRuntimeErrorExample("invalid_api_key", "Run key expired.", "invalid_api_key"),
							},
						},
					},
				},
			},
			"404": {
				Description: "绑定应用不存在",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: errSchema,
						Examples: map[string]*huma.Example{
							"missing-agent": {
								Summary: "绑定应用不存在",
								Value:   runRuntimeErrorExample("invalid_request_error", "no rows in result set", "invalid_request_error"),
							},
						},
					},
				},
			},
			"429": {
				Description: "命中既有限流策略",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: errSchema,
						Examples: map[string]*huma.Example{
							"rate-limit": {
								Summary: "触发限流",
								Value:   runRuntimeErrorExample("rate_limit_exceeded", "Rate limit exceeded.", "rate_limit_exceeded"),
							},
						},
					},
				},
			},
			"502": {
				Description: "上游调用失败",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: errSchema,
						Examples: map[string]*huma.Example{
							"upstream-failed": {
								Summary: "上游请求失败",
								Value:   runRuntimeErrorExample("upstream_error", "Upstream request failed.", "server_error"),
							},
						},
					},
				},
			},
			"503": {
				Description: "运行面依赖未配置或暂时不可用",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: errSchema,
						Examples: map[string]*huma.Example{
							"service-unavailable": {
								Summary: "运行时未就绪",
								Value:   runRuntimeErrorExample("service_unavailable", "Run runtime is not configured.", "service_unavailable"),
							},
						},
					},
				},
			},
		},
	})

	registerAsyncTaskOpenAPI(doc, schemas, errSchema)
}

func registerAsyncTaskOpenAPI(doc *huma.OpenAPI, schemas huma.Registry, errSchema *huma.Schema) {
	doc.Components.SecuritySchemes["taskBearerAuth"] = &huma.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "API key or app key",
		Description:  "使用 Authorization: Bearer sk-ai-xxx 或 Bearer rk_xxx 调用异步任务 API。",
	}
	jsonRequestSchema := schemas.Schema(reflect.TypeFor[AsyncTaskCreateJSONRequest](), true, "")
	multipartRequestSchema := schemas.Schema(reflect.TypeFor[AsyncTaskCreateMultipartRequest](), true, "")
	createResponseSchema := schemas.Schema(reflect.TypeFor[AsyncTaskCreateResponse](), true, "")
	getResponseSchema := schemas.Schema(reflect.TypeFor[AsyncTaskGetResponse](), true, "")
	listResponseSchema := schemas.Schema(reflect.TypeFor[AsyncTaskListResponse](), true, "")
	security := []map[string][]string{{"taskBearerAuth": {}}}

	doc.AddOperation(&huma.Operation{
		Method:      http.MethodPost,
		Path:        "/v1/tasks",
		OperationID: "ai-create-task",
		Summary:     "创建异步任务",
		Description: "使用 API key 或 app key 创建图片生成、图片编辑或聊天完成任务。API key 必须传 type；app key 可省略，由绑定应用类型推断，显式 type 仅作一致性断言。chat.completions 强制 stream=false，只保存最终响应。任务 ID 由服务端生成；相同凭据复用 Idempotency-Key 且执行输入相同时返回原任务，不同输入返回 409。metadata 不参与幂等指纹并原样回显。可选 webhook_url 仅接受公网 HTTPS。任务进入终态时发送最小通知，body 只含 source=UniHub、event（task.completed/task.failed/task.cancelled）和 task_id；完整状态、结果及错误必须通过 GET /v1/tasks/{id} 获取。首次立即投递，失败后按 10s、1m、5m、30m、2h 重试；2xx 成功，410 立即终止，单次超时 10s，禁止重定向。接收方应按 task_id + event 幂等处理，以容忍发送成功后进程崩溃造成的重复投递。",
		Tags:        []string{"runtime", "tasks"},
		Security:    security,
		Parameters: []*huma.Param{{
			Name: "Idempotency-Key", In: "header", Required: false,
			Description: "可选幂等键；作用域按当前 API key 或 app key 隔离，生命周期与任务一致。",
			Schema:      &huma.Schema{Type: huma.TypeString},
		}},
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: jsonRequestSchema,
					Example: map[string]any{
						"type": "images.generation",
						"input": map[string]any{
							"model": "gpt-image-1", "prompt": "Draw a lighthouse at dusk", "size": "1024x1024",
						},
						"metadata":    map[string]any{"order_id": "A-1"},
						"webhook_url": "https://caller.example.com/hooks/ai",
					},
				},
				"multipart/form-data": {Schema: multipartRequestSchema},
			},
		},
		Responses: map[string]*huma.Response{
			"202": {Description: "任务已持久化并进入 pending 状态；幂等重试也返回同一任务。", Content: map[string]*huma.MediaType{"application/json": {Schema: createResponseSchema}}},
			"400": taskOpenAPIErrorResponse("信封、任务类型或执行输入不合法", errSchema),
			"401": taskOpenAPIErrorResponse("API key 无效、禁用或过期", errSchema),
			"402": taskOpenAPIErrorResponse("额度、余额或订阅准入失败", errSchema),
			"403": taskOpenAPIErrorResponse("模型、分组或服务访问未授权", errSchema),
			"409": taskOpenAPIErrorResponse("Idempotency-Key 已用于不同执行输入", errSchema),
			"429": taskOpenAPIErrorResponse("租户 pending + running 任务达到上限", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务或准入依赖暂不可用", errSchema),
		},
	})

	doc.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/v1/tasks",
		OperationID: "ai-list-tasks",
		Summary:     "列出异步任务",
		Description: "按创建时间倒序列出当前 tenant + user 可见的 API/app 任务。starting_after 传上一页最后一个任务 ID，内部游标为 (created_at,id)。",
		Tags:        []string{"runtime", "tasks"},
		Security:    security,
		Parameters: []*huma.Param{
			{Name: "status", In: "query", Description: "可选状态过滤", Schema: &huma.Schema{Type: huma.TypeString, Enum: []any{"pending", "running", "completed", "failed", "cancelled"}}},
			{Name: "type", In: "query", Description: "可选 wire type 过滤", Schema: &huma.Schema{Type: huma.TypeString, Enum: []any{"images.generation", "images.edit", "chat.completions"}}},
			{Name: "limit", In: "query", Description: "页大小，默认 20，范围 1..100", Schema: &huma.Schema{Type: huma.TypeInteger, Minimum: openAPIFloat(1), Maximum: openAPIFloat(100), Default: 20}},
			{Name: "starting_after", In: "query", Description: "上一页最后一个任务 UUID", Schema: &huma.Schema{Type: huma.TypeString, Format: "uuid"}},
		},
		Responses: map[string]*huma.Response{
			"200": {Description: "任务列表。", Content: map[string]*huma.MediaType{"application/json": {Schema: listResponseSchema}}},
			"400": taskOpenAPIErrorResponse("过滤条件或游标不合法", errSchema),
			"401": taskOpenAPIErrorResponse("API key/app key 无效、禁用或过期", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})

	doc.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/v1/tasks/{taskID}",
		OperationID: "ai-get-task",
		Summary:     "查询异步任务",
		Description: "按任务 ID 查询当前状态与终态结果。可见范围为 tenant + user，不按具体 API key 限制，因此轮换密钥不会使任务失联。",
		Tags:        []string{"runtime", "tasks"},
		Security:    security,
		Parameters: []*huma.Param{{
			Name: "taskID", In: "path", Required: true, Description: "服务端生成的任务 UUID",
			Schema: &huma.Schema{Type: huma.TypeString, Format: "uuid"},
		}},
		Responses: map[string]*huma.Response{
			"200": {Description: "任务当前状态；终态时包含 result 或 error，以及实际 usage。", Content: map[string]*huma.MediaType{"application/json": {Schema: getResponseSchema}}},
			"401": taskOpenAPIErrorResponse("API key 无效、禁用或过期", errSchema),
			"404": taskOpenAPIErrorResponse("任务不存在、已过期或对当前调用方不可见", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})

	doc.AddOperation(&huma.Operation{
		Method:      http.MethodPost,
		Path:        "/v1/tasks/{taskID}/cancel",
		OperationID: "ai-cancel-task",
		Summary:     "取消异步任务",
		Description: "取消 pending 或 running 任务。跨实例运行的任务会在下一次租约心跳时停止；终态任务返回 409。",
		Tags:        []string{"runtime", "tasks"},
		Security:    security,
		Parameters: []*huma.Param{{
			Name: "taskID", In: "path", Required: true, Description: "服务端生成的任务 UUID",
			Schema: &huma.Schema{Type: huma.TypeString, Format: "uuid"},
		}},
		Responses: map[string]*huma.Response{
			"200": {Description: "取消后的任务对象。", Content: map[string]*huma.MediaType{"application/json": {Schema: getResponseSchema}}},
			"401": taskOpenAPIErrorResponse("API key/app key 无效、禁用或过期", errSchema),
			"404": taskOpenAPIErrorResponse("任务不存在、已过期或对当前调用方不可见", errSchema),
			"409": taskOpenAPIErrorResponse("任务已处于终态，不能取消", errSchema),
			"503": taskOpenAPIErrorResponse("任务服务暂不可用", errSchema),
		},
	})
}

func taskOpenAPIErrorResponse(description string, schema *huma.Schema) *huma.Response {
	return &huma.Response{
		Description: description,
		Content:     map[string]*huma.MediaType{"application/json": {Schema: schema}},
	}
}

func runRuntimeErrorExample(code, message, errorType string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"type":    errorType,
		},
	}
}
