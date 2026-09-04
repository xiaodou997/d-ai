package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamcompat"
	"xiaodou/dai/libs/go/httpx"
)

// 上游模型发现 / 导入 —— 拉取上游 /v1/models，导入选中的 model_code 到显式上游模型绑定。
// 客户端访问 /api/v1/upstream-accounts/.../upstream-models 等路径时命中本扁平 huma 路由。

// ---- DTO ----

type discoveredModelDTO struct {
	ID             string   `json:"id" doc:"上游模型 ID"`
	Name           string   `json:"name" doc:"展示名"`
	CapabilityType string   `json:"capability_type" doc:"推断的能力类型"`
	APIFormats     []string `json:"api_formats" doc:"成功发现该模型的请求端点格式"`
	Exists         bool     `json:"exists" doc:"是否已存在部署"`
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
		Models []string `json:"models" doc:"待导入的对外 model_code 列表"`
	}
}

type importEndpointUpstreamModelsOutput struct {
	Body struct {
		Created []string `json:"created" doc:"新创建显式上游模型绑定的 model_code"`
		Skipped []string `json:"skipped" doc:"已存在而跳过的 model_code"`
	}
}

type inferModelCapabilityInput struct {
	ModelCode string `query:"model_code" doc:"对外/上游模型名，用于建议能力类型"`
}

type inferModelCapabilityOutput struct {
	Body struct {
		CapabilityType string `json:"capability_type" doc:"推断的能力类型"`
		Source         string `json:"source" enum:"external,heuristic" doc:"推断来源：external=命中 models.dev 缓存目录，heuristic=本地模型名规则兜底"`
	}
}

// ---- Register ----

func registerUpstreamDiscovery(api huma.API, d UpstreamDiagnosticsHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-fetch-account-upstream-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/upstream-accounts/{accountID}/upstream-models",
		Summary:     "拉取上游账号模型列表",
		Description: "调用账号所有启用请求端点的模型目录并按模型 ID 合并，不落库。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *fetchEndpointUpstreamModelsInput) (*fetchEndpointUpstreamModelsOutput, error) {
		if d.AccountReader == nil || d.ProviderSecrets == nil {
			return nil, httpx.ErrUnavailable.WithDetail("database or provider secret codec is not configured")
		}
		_, err := parseTransportUUID(in.AccountID)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("invalid accountID")
		}
		account, err := d.AccountReader.GetAccountSecret(ctx, in.AccountID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		apiKey, err := d.ProviderSecrets.Decrypt(account.Ciphertext)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("failed to decrypt api key")
		}
		models, err := fetchAccountUpstreamModels(ctx, d.HTTPClient, account.Endpoints, apiKey)
		if err != nil {
			return nil, httpx.New("upstream_unavailable", http.StatusBadGateway, "Upstream Unavailable").WithDetail(sanitizeUpstreamFetchError(err))
		}
		if d.ModelBindings == nil {
			return nil, httpx.ErrUnavailable.WithDetail("model binding store is not configured")
		}
		codes, err := d.ModelBindings.ListModelCodes(ctx, domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: in.AccountID})
		if err != nil {
			return nil, mapServiceError(err)
		}
		existing := make(map[string]struct{}, len(codes))
		for _, code := range codes {
			existing[code] = struct{}{}
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
		if d.AccountReader == nil || d.ModelBindings == nil {
			return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
		}
		_, err := parseTransportUUID(in.AccountID)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("invalid accountID")
		}
		account, err := d.AccountReader.GetAccountSecret(ctx, in.AccountID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if len(in.Body.Models) == 0 {
			return nil, httpx.ErrBadRequest.WithDetail("models list is empty")
		}
		writes := make([]domain.UpstreamModelBindingWrite, 0, len(in.Body.Models))
		for _, m := range in.Body.Models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			writes = append(writes, domain.UpstreamModelBindingWrite{
				ModelCode:         m,
				CapabilityType:    string(domain.InferModelCapability(m)),
				UpstreamModelName: m,
				Status:            "active",
			})
		}
		policy := modelBindingPolicyForAccount(account)
		for _, write := range writes {
			if err := validateModelBindingProtocolPolicy(write, policy); err != nil {
				return nil, mapServiceError(err)
			}
		}
		result, err := d.ModelBindings.Import(ctx, domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: in.AccountID}, writes)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &importEndpointUpstreamModelsOutput{}
		out.Body.Created = result.Created
		out.Body.Skipped = result.Skipped
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-infer-model-capability",
		Method:      http.MethodGet,
		Path:        "/api/v1/model-capability/infer",
		Summary:     "推断模型能力",
		Description: "优先查 models.dev 缓存目录，未命中时按模型名建议能力类型。API 格式由账号请求端点显式声明，绝不从模型名推断。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *inferModelCapabilityInput) (*inferModelCapabilityOutput, error) {
		modelCode := strings.TrimSpace(in.ModelCode)
		if modelCode == "" {
			return nil, httpx.ErrBadRequest.WithDetail("model_code is required")
		}
		out := &inferModelCapabilityOutput{}
		if d.ModelCapabilities != nil {
			if capability, ok := d.ModelCapabilities.Lookup(ctx, modelCode); ok {
				out.Body.CapabilityType = string(capability)
				out.Body.Source = "external"
				return out, nil
			}
		}

		out.Body.CapabilityType = string(domain.InferModelCapability(modelCode))
		out.Body.Source = "heuristic"
		return out, nil
	})
}

// ---- 上游模型列表抓取（移植自 console.fetchUpstreamModelList，去掉 *Console 依赖）----

func fetchAccountUpstreamModels(ctx context.Context, client HTTPDoer, endpoints []domain.UpstreamAccountEndpoint, apiKey string) ([]discoveredModelDTO, error) {
	byID := make(map[string]discoveredModelDTO)
	var fetchErrors []error
	successes := 0
	for _, endpoint := range endpoints {
		if endpoint.Status != domain.EndpointStatusActive {
			continue
		}
		models, err := fetchUpstreamModelList(ctx, client, endpoint, apiKey)
		if err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("%s: %w", endpoint.APIFormat, err))
			continue
		}
		successes++
		for _, model := range models {
			current, exists := byID[model.ID]
			if !exists {
				current = model
			}
			format := string(endpoint.APIFormat)
			if !containsStringValue(current.APIFormats, format) {
				current.APIFormats = append(current.APIFormats, format)
				sort.Strings(current.APIFormats)
			}
			byID[model.ID] = current
		}
	}
	if successes == 0 {
		if len(fetchErrors) == 0 {
			return nil, errors.New("account has no active request endpoints")
		}
		return nil, errors.Join(fetchErrors...)
	}
	out := make([]discoveredModelDTO, 0, len(byID))
	for _, model := range byID {
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func containsStringValue(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func fetchUpstreamModelList(ctx context.Context, client HTTPDoer, endpoint domain.UpstreamAccountEndpoint, apiKey string) ([]discoveredModelDTO, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	listURL := modelListURL(endpoint)
	authHeaders := endpointAuthHeaders(endpoint, apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range authHeaders {
		req.Header.Set(key, value)
	}
	if err := applyDiscoveryExtraHeaders(req.Header, endpoint.ExtraHeaders); err != nil {
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
	return parseModelListResponse(body, endpoint.APIFormat)
}

func modelListURL(endpoint domain.UpstreamAccountEndpoint) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint.BaseURL), "/")
	if endpoint.APIFormat == domain.ProtocolGeminiGenerate || endpoint.APIFormat == domain.ProtocolGeminiEmbeddings {
		return appendAPIPath(base, "/v1beta/models")
	}
	return appendAPIPath(base, "/v1/models")
}

func appendAPIPath(base, path string) string {
	for _, prefix := range []string{"/v1beta", "/v1"} {
		if strings.HasSuffix(strings.ToLower(base), prefix) && strings.HasPrefix(strings.ToLower(path), prefix+"/") {
			return base + path[len(prefix):]
		}
	}
	return base + path
}

func endpointAuthHeaders(endpoint domain.UpstreamAccountEndpoint, apiKey string) map[string]string {
	scheme := endpoint.AuthScheme
	if scheme == "" || scheme == domain.EndpointAuthFormatDefault {
		switch endpoint.APIFormat {
		case domain.ProtocolAnthropicMessages:
			scheme = domain.EndpointAuthAnthropicAPIKey
		case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
			scheme = domain.EndpointAuthGeminiAPIKey
		default:
			scheme = domain.EndpointAuthBearer
		}
	}
	switch scheme {
	case domain.EndpointAuthAnthropicAPIKey:
		return upstreamcompat.AnthropicAPIKeyHeaders(apiKey)
	case domain.EndpointAuthGeminiAPIKey:
		return map[string]string{"x-goog-api-key": apiKey}
	case domain.EndpointAuthCustomHeader:
		if endpoint.AuthHeader == "" {
			return nil
		}
		return map[string]string{endpoint.AuthHeader: apiKey}
	default:
		return map[string]string{"Authorization": "Bearer " + apiKey}
	}
}

func parseModelListResponse(body []byte, apiFormat domain.UpstreamProtocol) ([]discoveredModelDTO, error) {
	if apiFormat == domain.ProtocolGeminiGenerate || apiFormat == domain.ProtocolGeminiEmbeddings {
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
		out = append(out, discoveredModelDTO{ID: id, Name: displayName, CapabilityType: string(domain.InferModelCapability(id))})
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
		out = append(out, discoveredModelDTO{ID: id, Name: displayName, CapabilityType: string(domain.InferModelCapability(id))})
	}
	return out, nil
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
