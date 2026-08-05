package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/externalmodels"
	"xiaodou/dai/internal/ai/secret"
	"xiaodou/dai/internal/ai/upstreamcompat"
	"xiaodou/dai/libs/go/httpx"
)

// 上游模型发现 / 导入 —— 拉取上游 /v1/models，导入选中的 model_code 到显式上游模型绑定。
// 客户端访问 /api/v1/upstream-accounts/.../upstream-models 等路径时命中本扁平 huma 路由。

// ---- DTO ----

type discoveredModelDTO struct {
	ID             string `json:"id" doc:"上游模型 ID"`
	Name           string `json:"name" doc:"展示名"`
	CapabilityType string `json:"capability_type" doc:"推断的能力类型"`
	APIFormat      string `json:"api_format" doc:"推断的上游 API 格式"`
	Exists         bool   `json:"exists" doc:"是否已存在部署"`
}

type fetchEndpointUpstreamModelsInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
}

type fetchEndpointUpstreamModelsOutput struct {
	Body struct {
		Items []discoveredModelDTO `json:"items"`
		Total int                  `json:"total"`
	}
}

type importEndpointUpstreamModelsInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      struct {
		Models    []string `json:"models" doc:"待导入的对外 model_code 列表"`
		APIFormat string   `json:"api_format,omitempty" enum:"openai_chat,openai_responses,openai_embeddings,openai_images,anthropic_messages,gemini_generate,gemini_embeddings" doc:"导入时使用的上游 API 格式；为空按模型和账号默认家族推断"`
	}
}

type importEndpointUpstreamModelsOutput struct {
	Body struct {
		Created []string `json:"created" doc:"新创建显式上游模型绑定的 model_code"`
		Skipped []string `json:"skipped" doc:"已存在而跳过的 model_code"`
	}
}

type inferModelCapabilityInput struct {
	ModelCode        string `query:"model_code" doc:"对外/上游模型名，用于推断能力与协议"`
	EndpointProtocol string `query:"endpoint_protocol,omitempty" enum:"openai_compatible,anthropic,gemini" doc:"账号默认上游家族；留空按 openai_compatible 处理"`
}

type inferModelCapabilityOutput struct {
	Body struct {
		CapabilityType string `json:"capability_type" doc:"推断的能力类型"`
		APIFormat      string `json:"api_format" doc:"推断的上游 API 格式"`
		Source         string `json:"source" enum:"external,heuristic" doc:"推断来源：external=命中 models.dev 缓存目录，heuristic=本地模型名规则兜底"`
	}
}

// ---- Register ----

func registerUpstreamDiscovery(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-fetch-account-upstream-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/upstream-accounts/{accountID}/upstream-models",
		Summary:     "拉取上游账号模型列表",
		Description: "调用上游 /v1/models 并推断能力与协议，不落库。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *fetchEndpointUpstreamModelsInput) (*fetchEndpointUpstreamModelsOutput, error) {
		if d.Queries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
		}
		accountID, err := parseTransportUUID(in.AccountID)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("invalid accountID")
		}
		account, err := d.Queries.GetUpstreamAccount(ctx, accountID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		apiKey, err := secret.DecryptProviderKey(d.ProviderKeyMaster, account.ApiKeyCiphertext)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("failed to decrypt api key")
		}
		models, err := fetchUpstreamModelList(ctx, d.HTTPClient, account.BaseUrl, apiKey, account.DefaultProtocol, account.ExtraHeaders)
		if err != nil {
			return nil, httpx.New("upstream_unavailable", http.StatusBadGateway, "Upstream Unavailable").WithDetail(sanitizeUpstreamFetchError(err))
		}
		rows, err := d.Postgres.Query(ctx, `
			SELECT model_code
			FROM ai_upstream_models
			WHERE upstream_kind = 'direct_upstream' AND upstream_id = $1
		`, accountID)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("load upstream model bindings failed: " + err.Error())
		}
		defer rows.Close()
		existing := make(map[string]struct{})
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				return nil, httpx.ErrInternal.WithDetail("scan upstream model bindings failed: " + err.Error())
			}
			existing[code] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			return nil, httpx.ErrInternal.WithDetail("iterate upstream model bindings failed: " + err.Error())
		}
		for i := range models {
			if _, ok := existing[models[i].ID]; ok {
				models[i].Exists = true
			}
		}
		out := &fetchEndpointUpstreamModelsOutput{}
		out.Body.Items = models
		out.Body.Total = len(models)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-import-account-upstream-models",
		Method:        http.MethodPost,
		Path:          "/api/v1/upstream-accounts/{accountID}/import-upstream-models",
		Summary:       "导入上游模型到显式绑定",
		Description:   "把选中的 model_code 创建为账号的显式上游模型绑定（去重幂等）。",
		Tags:          []string{"upstream-accounts"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *importEndpointUpstreamModelsInput) (*importEndpointUpstreamModelsOutput, error) {
		if d.Queries == nil || d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
		}
		accountID, err := parseTransportUUID(in.AccountID)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("invalid accountID")
		}
		account, err := d.Queries.GetUpstreamAccount(ctx, accountID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if len(in.Body.Models) == 0 {
			return nil, httpx.ErrBadRequest.WithDetail("models list is empty")
		}
		apiFormatOverride := strings.TrimSpace(in.Body.APIFormat)
		if apiFormatOverride != "" {
			if err := validateBindingProtocol("api_format", apiFormatOverride); err != nil {
				return nil, mapServiceError(err)
			}
		}

		out := &importEndpointUpstreamModelsOutput{}
		out.Body.Created = []string{}
		out.Body.Skipped = []string{}

		tx, err := d.Postgres.Begin(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("begin tx failed: " + err.Error())
		}
		defer tx.Rollback(ctx)

		existingRows, err := tx.Query(ctx, `
			SELECT model_code
			FROM ai_upstream_models
			WHERE upstream_kind = 'direct_upstream' AND upstream_id = $1
		`, accountID)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("load upstream model bindings failed: " + err.Error())
		}
		existing := make(map[string]struct{})
		for existingRows.Next() {
			var code string
			if err := existingRows.Scan(&code); err != nil {
				existingRows.Close()
				return nil, httpx.ErrInternal.WithDetail("scan upstream model bindings failed: " + err.Error())
			}
			existing[code] = struct{}{}
		}
		existingRows.Close()
		if err := existingRows.Err(); err != nil {
			return nil, httpx.ErrInternal.WithDetail("iterate upstream model bindings failed: " + err.Error())
		}

		for _, m := range in.Body.Models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			if _, ok := existing[m]; ok {
				out.Body.Skipped = append(out.Body.Skipped, m)
				continue
			}
			defaults, err := resolveImportedModelBindingDefaults(m, account.DefaultProtocol, apiFormatOverride)
			if err != nil {
				return nil, mapServiceError(err)
			}
			if _, err := tx.Exec(ctx, `
			INSERT INTO ai_upstream_models (
				upstream_kind,
				upstream_id,
				model_code,
				capability_type,
				api_format,
				upstream_model_name,
				status
			) VALUES ('direct_upstream', $1, $2, $3, $4, $5, 'active')
		`, accountID, m, string(defaults.CapabilityType), string(defaults.APIFormat), m); err != nil {
				return nil, httpx.ErrInternal.WithDetail("insert upstream model binding failed: " + err.Error())
			}
			existing[m] = struct{}{}
			out.Body.Created = append(out.Body.Created, m)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, httpx.ErrInternal.WithDetail("commit tx failed: " + err.Error())
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-infer-model-capability",
		Method:      http.MethodGet,
		Path:        "/api/v1/model-capability/infer",
		Summary:     "推断模型能力与协议",
		Description: "优先查 models.dev 缓存目录，按模态结构判断 image/audio_tts/audio_stt；未命中或模态无法区分 chat/embedding/rerank 时回退本地模型名规则。仅用于表单默认值建议，不做任何写入，也不影响提交时的合法性校验。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *inferModelCapabilityInput) (*inferModelCapabilityOutput, error) {
		modelCode := strings.TrimSpace(in.ModelCode)
		if modelCode == "" {
			return nil, httpx.ErrBadRequest.WithDetail("model_code is required")
		}
		endpointProtocol := strings.TrimSpace(in.EndpointProtocol)

		out := &inferModelCapabilityOutput{}
		if capability, ok := externalmodels.Lookup(ctx, d.Redis, d.HTTPClient, modelCode); ok {
			out.Body.CapabilityType = string(capability)
			out.Body.APIFormat = string(domain.DefaultProtocolForCapability(capability, modelCode, endpointProtocol))
			out.Body.Source = "external"
			return out, nil
		}

		capability, apiFormat := inferCapabilityAndProtocol(modelCode, endpointProtocol)
		out.Body.CapabilityType = capability
		out.Body.APIFormat = apiFormat
		out.Body.Source = "heuristic"
		return out, nil
	})
}

// ---- 上游模型列表抓取（移植自 console.fetchUpstreamModelList，去掉 *Console 依赖）----

func fetchUpstreamModelList(ctx context.Context, client *http.Client, baseURL, apiKey, defaultProtocol string, extraHeaders []byte) ([]discoveredModelDTO, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	listURL, authHeaders := modelListURLAndAuthHeaders(baseURL, apiKey, defaultProtocol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range authHeaders {
		req.Header.Set(key, value)
	}
	if err := applyDiscoveryExtraHeaders(req.Header, extraHeaders); err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &upstreamFetchErr{Status: resp.StatusCode, URL: listURL, Body: truncateStr(string(body), 4096)}
	}
	return parseModelListResponse(body, defaultProtocol)
}

func modelListURLAndAuthHeaders(baseURL, apiKey, defaultProtocol string) (string, map[string]string) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch defaultProtocol {
	case string(domain.EndpointProtocolAnthropic):
		return base + "/v1/models", upstreamcompat.AnthropicAPIKeyHeaders(apiKey)
	case string(domain.EndpointProtocolGemini):
		return base + "/v1beta/models", map[string]string{"x-goog-api-key": apiKey}
	default:
		return base + "/v1/models", map[string]string{"Authorization": "Bearer " + apiKey}
	}
}

func parseModelListResponse(body []byte, defaultProtocol string) ([]discoveredModelDTO, error) {
	if defaultProtocol == string(domain.EndpointProtocolGemini) {
		return parseGeminiModelList(body)
	}
	var envelope struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Name        string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse model list: %w", err)
	}
	out := make([]discoveredModelDTO, 0, len(envelope.Data))
	for _, m := range envelope.Data {
		id := m.ID
		if id == "" {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = id
		}
		cap, proto := inferCapabilityAndProtocol(id, defaultProtocol)
		out = append(out, discoveredModelDTO{ID: id, Name: displayName, CapabilityType: cap, APIFormat: proto})
	}
	return out, nil
}

func parseGeminiModelList(body []byte) ([]discoveredModelDTO, error) {
	var envelope struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse gemini model list: %w", err)
	}
	out := make([]discoveredModelDTO, 0, len(envelope.Models))
	for _, m := range envelope.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = id
		}
		cap, proto := inferCapabilityAndProtocol(id, string(domain.EndpointProtocolGemini))
		out = append(out, discoveredModelDTO{ID: id, Name: displayName, CapabilityType: cap, APIFormat: proto})
	}
	return out, nil
}

// inferCapabilityAndProtocol 复用 domain 层的模型能力命名识别（单一事实源）。
// Gemini 生图模型（gemini-*-image / nano-banana）会被判为 image + gemini_generate。
func inferCapabilityAndProtocol(modelID, endpointProtocol string) (capabilityType, upstreamProtocol string) {
	capability, protocol := domain.InferModelCapabilityAndProtocol(modelID, endpointProtocol)
	return string(capability), string(protocol)
}

type importedModelBindingDefaults struct {
	CapabilityType domain.CapabilityType
	APIFormat      domain.UpstreamProtocol
}

func resolveImportedModelBindingDefaults(modelID, endpointProtocol, apiFormatOverride string) (importedModelBindingDefaults, error) {
	capability, protocol := domain.InferModelCapabilityAndProtocol(modelID, endpointProtocol)
	if apiFormatOverride != "" {
		protocol = domain.UpstreamProtocol(apiFormatOverride)
	}
	if !bindingProtocolSupportsCapability(protocol, capability) {
		return importedModelBindingDefaults{}, domain.NewValidationError(
			"api_format",
			fmt.Sprintf("API format %s does not support capability %s", protocol, capability),
		)
	}
	return importedModelBindingDefaults{CapabilityType: capability, APIFormat: protocol}, nil
}

// ---- 小工具（移植自 console，避免跨包引用未导出符号）----

func applyDiscoveryExtraHeaders(headers http.Header, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err == nil {
		for key, value := range values {
			headers.Set(key, value)
		}
		return nil
	}
	var anyValues map[string]any
	if err := json.Unmarshal(raw, &anyValues); err != nil {
		return fmt.Errorf("parse extra_headers: %w", err)
	}
	for key, value := range anyValues {
		headers.Set(key, fmt.Sprint(value))
	}
	return nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type upstreamFetchErr struct {
	Status int
	URL    string
	Body   string
}

func (e *upstreamFetchErr) Error() string {
	return fmt.Sprintf("upstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

func sanitizeUpstreamFetchError(err error) string {
	var fe *upstreamFetchErr
	if errors.As(err, &fe) {
		return fmt.Sprintf("上游返回 %d", fe.Status)
	}
	return "无法连接到上游或解析模型列表失败"
}
