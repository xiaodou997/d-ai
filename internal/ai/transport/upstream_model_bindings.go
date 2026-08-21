package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

type upstreamModelBindingDTO struct {
	ID                          string `json:"id" doc:"显式上游模型绑定 ID"`
	ModelCode                   string `json:"model_code" doc:"模型标识"`
	CapabilityType              string `json:"capability_type" doc:"能力类型"`
	APIFormat                   string `json:"api_format" doc:"上游 API 格式"`
	UpstreamModelName           string `json:"upstream_model_name" doc:"上游模型 ID"`
	Status                      string `json:"status" doc:"状态"`
	ImageStreamMode             string `json:"image_stream_mode,omitempty" enum:"auto,force_stream,force_sync" doc:"生图上游流式策略"`
	ImageEditTransport          string `json:"image_edit_transport,omitempty" enum:"application/json,multipart/form-data" doc:"图生图上游请求格式"`
	ImageUpstreamResponseFormat string `json:"image_upstream_response_format,omitempty" enum:"url,b64_json" doc:"上游生图返回格式；未设置时不向上游发送 response_format"`
	ImageMaxOutputCount         int    `json:"image_max_output_count" minimum:"1" maximum:"10" doc:"文生图单次最大输出张数"`
	ImageEditMaxOutputCount     int    `json:"image_edit_max_output_count" minimum:"1" maximum:"10" doc:"图生图单次最大输出张数"`
	CreatedAt                   *int64 `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt                   *int64 `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type upstreamModelBindingsOutput struct {
	Body struct {
		Items []upstreamModelBindingDTO `json:"items"`
		Total int                       `json:"total"`
	}
}

type upstreamModelBindingOutput struct {
	Body upstreamModelBindingDTO
}

type upstreamModelBindingWriteRequest struct {
	ModelCode                   string  `json:"model_code,omitempty" doc:"模型标识"`
	CapabilityType              string  `json:"capability_type,omitempty" enum:"chat,image,video,embedding,audio_tts,audio_stt,rerank" doc:"能力类型；为空时按 model_code 推断"`
	APIFormat                   string  `json:"api_format,omitempty" enum:"openai_chat,openai_responses,openai_embeddings,openai_images,anthropic_messages,gemini_generate,gemini_embeddings" doc:"上游 API 格式"`
	UpstreamModelName           string  `json:"upstream_model_name,omitempty" doc:"上游模型 ID；为空默认同 model_code"`
	Status                      string  `json:"status,omitempty" enum:"active,disabled" doc:"状态；为空默认/保留 active"`
	ImageStreamMode             string  `json:"image_stream_mode,omitempty" enum:"auto,force_stream,force_sync" doc:"生图上游流式策略"`
	ImageEditTransport          string  `json:"image_edit_transport,omitempty" enum:"application/json,multipart/form-data" doc:"图生图上游请求格式"`
	ImageUpstreamResponseFormat *string `json:"image_upstream_response_format,omitempty" doc:"上游生图返回格式；空字符串表示清除配置并且不向上游发送 response_format"`
	ImageMaxOutputCount         *int    `json:"image_max_output_count,omitempty" minimum:"1" maximum:"10" doc:"文生图单次最大输出张数"`
	ImageEditMaxOutputCount     *int    `json:"image_edit_max_output_count,omitempty" minimum:"1" maximum:"10" doc:"图生图单次最大输出张数"`
}

type accountModelBindingsInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
}

type createAccountModelBindingInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      upstreamModelBindingWriteRequest
}

type updateAccountModelBindingInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	BindingID string `path:"bindingID" doc:"显式上游模型绑定 ID"`
	Body      upstreamModelBindingWriteRequest
}

type deleteAccountModelBindingInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	BindingID string `path:"bindingID" doc:"显式上游模型绑定 ID"`
}

type batchDeleteUpstreamModelBindingsRequest struct {
	BindingIDs []string `json:"binding_ids" doc:"要删除的显式上游模型绑定 ID 列表"`
}

type batchDeleteAccountModelBindingsInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      batchDeleteUpstreamModelBindingsRequest
}

type poolModelBindingsInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
}

type createPoolModelBindingInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	Body   upstreamModelBindingWriteRequest
}

type updatePoolModelBindingInput struct {
	PoolID    string `path:"poolID" doc:"凭证池 ID"`
	BindingID string `path:"bindingID" doc:"显式上游模型绑定 ID"`
	Body      upstreamModelBindingWriteRequest
}

type deletePoolModelBindingInput struct {
	PoolID    string `path:"poolID" doc:"凭证池 ID"`
	BindingID string `path:"bindingID" doc:"显式上游模型绑定 ID"`
}

type batchDeletePoolModelBindingsInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	Body   batchDeleteUpstreamModelBindingsRequest
}

type deleteUpstreamModelBindingOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type batchDeleteUpstreamModelBindingsOutput struct {
	Body struct {
		Deleted int64 `json:"deleted" doc:"实际删除数量"`
	}
}

func registerUpstreamModelBindings(api huma.API, d AIDeps) {
	registerAccountModelBindings(api, d)
	registerPoolModelBindings(api, d)
}

func registerAccountModelBindings(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-account-model-bindings",
		Method:      http.MethodGet,
		Path:        "/api/v1/upstream-accounts/{accountID}/model-bindings",
		Summary:     "上游账号显式模型绑定列表",
		Description: "返回指定上游账号的 ai_upstream_models 显式绑定列表。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *accountModelBindingsInput) (*upstreamModelBindingsOutput, error) {
		if _, err := ensureDirectUpstreamExists(ctx, d, in.AccountID); err != nil {
			return nil, err
		}
		items, err := listUpstreamModelBindings(ctx, d, "direct_upstream", in.AccountID)
		if err != nil {
			return nil, err
		}
		return bindingListOutput(items), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-account-model-binding",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts/{accountID}/model-bindings",
		Summary:     "创建上游账号显式模型绑定",
		Description: "为指定上游账号直接创建一条 ai_upstream_models 显式绑定。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *createAccountModelBindingInput) (*upstreamModelBindingOutput, error) {
		account, err := ensureDirectUpstreamExists(ctx, d, in.AccountID)
		if err != nil {
			return nil, err
		}
		item, err := createUpstreamModelBinding(ctx, d, "direct_upstream", in.AccountID, fixedProviderEndpointProtocolFromAccount(account.DefaultProtocol), nil, in.Body)
		if err != nil {
			return nil, err
		}
		return &upstreamModelBindingOutput{Body: bindingRecordToDTO(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-account-model-binding",
		Method:      http.MethodPatch,
		Path:        "/api/v1/upstream-accounts/{accountID}/model-bindings/{bindingID}",
		Summary:     "更新上游账号显式模型绑定",
		Description: "更新指定上游账号的一条 ai_upstream_models 显式绑定。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *updateAccountModelBindingInput) (*upstreamModelBindingOutput, error) {
		account, err := ensureDirectUpstreamExists(ctx, d, in.AccountID)
		if err != nil {
			return nil, err
		}
		item, err := updateUpstreamModelBinding(ctx, d, "direct_upstream", in.AccountID, in.BindingID, fixedProviderEndpointProtocolFromAccount(account.DefaultProtocol), nil, in.Body)
		if err != nil {
			return nil, err
		}
		return &upstreamModelBindingOutput{Body: bindingRecordToDTO(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-account-model-binding",
		Method:      http.MethodDelete,
		Path:        "/api/v1/upstream-accounts/{accountID}/model-bindings/{bindingID}",
		Summary:     "删除上游账号显式模型绑定",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *deleteAccountModelBindingInput) (*deleteUpstreamModelBindingOutput, error) {
		if _, err := ensureDirectUpstreamExists(ctx, d, in.AccountID); err != nil {
			return nil, err
		}
		if err := deleteUpstreamModelBinding(ctx, d, "direct_upstream", in.AccountID, in.BindingID); err != nil {
			return nil, err
		}
		out := &deleteUpstreamModelBindingOutput{}
		out.Body.Deleted = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-batch-delete-account-model-bindings",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts/{accountID}/model-bindings/batch-delete",
		Summary:     "批量删除上游账号显式模型绑定",
		Description: "在指定账号范围内原子删除选中的显式绑定；不存在的 binding_id 会被幂等忽略。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *batchDeleteAccountModelBindingsInput) (*batchDeleteUpstreamModelBindingsOutput, error) {
		bindingIDs, err := normalizeBatchDeleteBindingIDs(in.Body.BindingIDs)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if _, err := ensureDirectUpstreamExists(ctx, d, in.AccountID); err != nil {
			return nil, err
		}
		deleted, err := batchDeleteUpstreamModelBindings(ctx, d, "direct_upstream", in.AccountID, bindingIDs)
		if err != nil {
			return nil, err
		}
		out := &batchDeleteUpstreamModelBindingsOutput{}
		out.Body.Deleted = deleted
		return out, nil
	})
}

func registerPoolModelBindings(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-pool-model-bindings",
		Method:      http.MethodGet,
		Path:        "/api/v1/credential-pools/{poolID}/model-bindings",
		Summary:     "凭证池显式模型绑定列表",
		Description: "返回指定凭证池的 ai_upstream_models 显式绑定列表。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *poolModelBindingsInput) (*upstreamModelBindingsOutput, error) {
		if _, err := ensurePoolExists(ctx, d, in.PoolID); err != nil {
			return nil, err
		}
		items, err := listUpstreamModelBindings(ctx, d, "oauth_pool", in.PoolID)
		if err != nil {
			return nil, err
		}
		return bindingListOutput(items), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-pool-model-binding",
		Method:      http.MethodPost,
		Path:        "/api/v1/credential-pools/{poolID}/model-bindings",
		Summary:     "创建凭证池显式模型绑定",
		Description: "为指定凭证池直接创建一条 ai_upstream_models 显式绑定。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *createPoolModelBindingInput) (*upstreamModelBindingOutput, error) {
		pool, err := ensurePoolExists(ctx, d, in.PoolID)
		if err != nil {
			return nil, err
		}
		item, err := createUpstreamModelBinding(ctx, d, "oauth_pool", in.PoolID, fixedProviderEndpointProtocol(pool.FixedProviderType), &pool.FixedProviderType, in.Body)
		if err != nil {
			return nil, err
		}
		return &upstreamModelBindingOutput{Body: bindingRecordToDTO(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-pool-model-binding",
		Method:      http.MethodPatch,
		Path:        "/api/v1/credential-pools/{poolID}/model-bindings/{bindingID}",
		Summary:     "更新凭证池显式模型绑定",
		Description: "更新指定凭证池的一条 ai_upstream_models 显式绑定。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *updatePoolModelBindingInput) (*upstreamModelBindingOutput, error) {
		pool, err := ensurePoolExists(ctx, d, in.PoolID)
		if err != nil {
			return nil, err
		}
		item, err := updateUpstreamModelBinding(ctx, d, "oauth_pool", in.PoolID, in.BindingID, fixedProviderEndpointProtocol(pool.FixedProviderType), &pool.FixedProviderType, in.Body)
		if err != nil {
			return nil, err
		}
		return &upstreamModelBindingOutput{Body: bindingRecordToDTO(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-pool-model-binding",
		Method:      http.MethodDelete,
		Path:        "/api/v1/credential-pools/{poolID}/model-bindings/{bindingID}",
		Summary:     "删除凭证池显式模型绑定",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *deletePoolModelBindingInput) (*deleteUpstreamModelBindingOutput, error) {
		if _, err := ensurePoolExists(ctx, d, in.PoolID); err != nil {
			return nil, err
		}
		if err := deleteUpstreamModelBinding(ctx, d, "oauth_pool", in.PoolID, in.BindingID); err != nil {
			return nil, err
		}
		out := &deleteUpstreamModelBindingOutput{}
		out.Body.Deleted = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-batch-delete-pool-model-bindings",
		Method:      http.MethodPost,
		Path:        "/api/v1/credential-pools/{poolID}/model-bindings/batch-delete",
		Summary:     "批量删除凭证池显式模型绑定",
		Description: "在指定凭证池范围内原子删除选中的显式绑定；不存在的 binding_id 会被幂等忽略。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *batchDeletePoolModelBindingsInput) (*batchDeleteUpstreamModelBindingsOutput, error) {
		bindingIDs, err := normalizeBatchDeleteBindingIDs(in.Body.BindingIDs)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if _, err := ensurePoolExists(ctx, d, in.PoolID); err != nil {
			return nil, err
		}
		deleted, err := batchDeleteUpstreamModelBindings(ctx, d, "oauth_pool", in.PoolID, bindingIDs)
		if err != nil {
			return nil, err
		}
		out := &batchDeleteUpstreamModelBindingsOutput{}
		out.Body.Deleted = deleted
		return out, nil
	})
}

func bindingListOutput(items []domain.UpstreamModelBinding) *upstreamModelBindingsOutput {
	out := &upstreamModelBindingsOutput{}
	out.Body.Items = make([]upstreamModelBindingDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, bindingRecordToDTO(item))
	}
	out.Body.Total = len(out.Body.Items)
	return out
}

func bindingRecordToDTO(item domain.UpstreamModelBinding) upstreamModelBindingDTO {
	imagePolicy := parseImageGenerationBindingPolicy(item.ConfigJSON)
	return upstreamModelBindingDTO{
		ID:                          item.ID,
		ModelCode:                   item.ModelCode,
		CapabilityType:              item.CapabilityType,
		APIFormat:                   item.APIFormat,
		UpstreamModelName:           item.UpstreamModelName,
		Status:                      item.Status,
		ImageStreamMode:             imagePolicy.StreamMode,
		ImageEditTransport:          imagePolicy.EditTransport,
		ImageUpstreamResponseFormat: imagePolicy.UpstreamResponseFormat,
		ImageMaxOutputCount:         imagePolicy.MaxOutputCount,
		ImageEditMaxOutputCount:     imagePolicy.EditMaxOutputCount,
		CreatedAt:                   timeToMillisPtr(item.CreatedAt),
		UpdatedAt:                   timeToMillisPtr(item.UpdatedAt),
	}
}

func ensureDirectUpstreamExists(ctx context.Context, d AIDeps, accountID string) (upstreamAccountDTO, error) {
	if d.AccountReader == nil {
		return upstreamAccountDTO{}, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	_, err := parseTransportUUID(accountID)
	if err != nil {
		return upstreamAccountDTO{}, httpx.ErrBadRequest.WithDetail("invalid accountID")
	}
	account, err := d.AccountReader.GetAccountSecret(ctx, accountID)
	if err != nil {
		return upstreamAccountDTO{}, mapServiceError(err)
	}
	return upstreamAccountDTO{DefaultProtocol: account.DefaultProtocol}, nil
}

type upstreamAccountDTO struct {
	DefaultProtocol string
}

func ensurePoolExists(ctx context.Context, d AIDeps, poolID string) (*domain.CredentialPool, error) {
	if d.PoolReader == nil {
		return nil, httpx.ErrUnavailable.WithDetail("oauth credential store is not configured")
	}
	pool, err := d.PoolReader.GetPool(ctx, poolID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return pool, nil
}

func modelBindingScope(upstreamKind, upstreamID string) domain.UpstreamModelBindingScope {
	return domain.UpstreamModelBindingScope{Kind: domain.UpstreamKind(upstreamKind), ID: upstreamID}
}

func listUpstreamModelBindings(ctx context.Context, d AIDeps, upstreamKind, upstreamID string) ([]domain.UpstreamModelBinding, error) {
	if d.ModelBindings == nil {
		return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	items, err := d.ModelBindings.List(ctx, modelBindingScope(upstreamKind, upstreamID))
	if err != nil {
		return nil, mapServiceError(err)
	}
	return items, nil
}

func createUpstreamModelBinding(ctx context.Context, d AIDeps, upstreamKind, upstreamID, endpointProtocol string, fixedProviderType *domain.FixedProviderType, req upstreamModelBindingWriteRequest) (domain.UpstreamModelBinding, error) {
	if d.ModelBindings == nil {
		return domain.UpstreamModelBinding{}, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	write, err := normalizeUpstreamModelBindingWrite(req, endpointProtocol, fixedProviderType, nil)
	if err != nil {
		return domain.UpstreamModelBinding{}, mapServiceError(err)
	}
	item, err := d.ModelBindings.Create(ctx, modelBindingScope(upstreamKind, upstreamID), write)
	if err != nil {
		return domain.UpstreamModelBinding{}, mapServiceError(err)
	}
	return item, nil
}

func updateUpstreamModelBinding(ctx context.Context, d AIDeps, upstreamKind, upstreamID, bindingID, endpointProtocol string, fixedProviderType *domain.FixedProviderType, req upstreamModelBindingWriteRequest) (domain.UpstreamModelBinding, error) {
	current, err := getUpstreamModelBinding(ctx, d, upstreamKind, upstreamID, bindingID)
	if err != nil {
		return domain.UpstreamModelBinding{}, err
	}
	write, normalizeErr := normalizeUpstreamModelBindingWrite(req, endpointProtocol, fixedProviderType, &current)
	if normalizeErr != nil {
		return domain.UpstreamModelBinding{}, mapServiceError(normalizeErr)
	}
	item, err := d.ModelBindings.Update(ctx, modelBindingScope(upstreamKind, upstreamID), bindingID, write)
	if err != nil {
		return domain.UpstreamModelBinding{}, mapServiceError(err)
	}
	return item, nil
}

func deleteUpstreamModelBinding(ctx context.Context, d AIDeps, upstreamKind, upstreamID, bindingID string) error {
	if d.ModelBindings == nil {
		return httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	return mapServiceError(d.ModelBindings.Delete(ctx, modelBindingScope(upstreamKind, upstreamID), bindingID))
}

func normalizeBatchDeleteBindingIDs(rawIDs []string) ([]string, error) {
	if len(rawIDs) == 0 {
		return nil, domain.NewValidationError("binding_ids", "at least one binding ID is required")
	}

	ids := make([]string, 0, len(rawIDs))
	seen := make(map[uuid.UUID]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := parseTransportUUID(strings.TrimSpace(rawID))
		if err != nil {
			return nil, domain.NewValidationError("binding_ids", "must contain valid UUIDs")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id.String())
	}
	return ids, nil
}

func batchDeleteUpstreamModelBindings(ctx context.Context, d AIDeps, upstreamKind, upstreamID string, bindingIDs []string) (int64, error) {
	if d.ModelBindings == nil {
		return 0, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	deleted, err := d.ModelBindings.BatchDelete(ctx, modelBindingScope(upstreamKind, upstreamID), bindingIDs)
	if err != nil {
		return 0, mapServiceError(err)
	}
	return deleted, nil
}

func getUpstreamModelBinding(ctx context.Context, d AIDeps, upstreamKind, upstreamID, bindingID string) (domain.UpstreamModelBinding, error) {
	if d.ModelBindings == nil {
		return domain.UpstreamModelBinding{}, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	item, err := d.ModelBindings.Get(ctx, modelBindingScope(upstreamKind, upstreamID), bindingID)
	if err != nil {
		return domain.UpstreamModelBinding{}, mapServiceError(err)
	}
	return item, nil
}

type imageGenerationBindingPolicy struct {
	StreamMode             string
	EditTransport          string
	UpstreamResponseFormat string
	MaxOutputCount         int
	EditMaxOutputCount     int
}

func normalizeUpstreamModelBindingWrite(req upstreamModelBindingWriteRequest, endpointProtocol string, fixedProviderType *domain.FixedProviderType, current *domain.UpstreamModelBinding) (domain.UpstreamModelBindingWrite, error) {
	modelCode := strings.TrimSpace(req.ModelCode)
	if modelCode == "" && current != nil {
		modelCode = current.ModelCode
	}
	if modelCode == "" {
		return domain.UpstreamModelBindingWrite{}, domain.NewValidationError("model_code", "model_code is required")
	}

	capabilityType := strings.TrimSpace(req.CapabilityType)
	if capabilityType == "" {
		if current != nil {
			capabilityType = current.CapabilityType
		} else {
			capabilityType, _ = inferCapabilityAndProtocol(modelCode, endpointProtocol)
		}
	}
	if err := validateBindingCapabilityType(capabilityType); err != nil {
		return domain.UpstreamModelBindingWrite{}, err
	}

	apiFormat := strings.TrimSpace(req.APIFormat)
	if apiFormat == "" {
		if current != nil {
			apiFormat = current.APIFormat
		} else if fixedProviderType != nil {
			apiFormat = string(domain.FixedProviderProtocol(*fixedProviderType))
		} else {
			_, apiFormat = inferCapabilityAndProtocol(modelCode, endpointProtocol)
		}
	}
	if err := validateBindingProtocol("api_format", apiFormat); err != nil {
		return domain.UpstreamModelBindingWrite{}, err
	}

	upstreamModelName := strings.TrimSpace(req.UpstreamModelName)
	if upstreamModelName == "" {
		if current != nil {
			upstreamModelName = current.UpstreamModelName
		} else {
			upstreamModelName = modelCode
		}
	}

	if fixedProviderType != nil {
		expectedProtocol := string(domain.FixedProviderProtocol(*fixedProviderType))
		if apiFormat != expectedProtocol {
			return domain.UpstreamModelBindingWrite{}, domain.NewValidationError("api_format", "fixed-provider pool bindings must use API format "+expectedProtocol)
		}
	}

	if !bindingProtocolSupportsCapability(domain.UpstreamProtocol(apiFormat), domain.CapabilityType(capabilityType)) {
		return domain.UpstreamModelBindingWrite{}, domain.NewValidationError("api_format", fmt.Sprintf("API format %s does not support capability %s", apiFormat, capabilityType))
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		if current != nil {
			status = current.Status
		} else {
			status = "active"
		}
	}
	switch status {
	case "active", "disabled":
	default:
		return domain.UpstreamModelBindingWrite{}, domain.NewValidationError("status", "status must be active or disabled")
	}

	configJSON, err := mergeImageGenerationBindingPolicy(currentConfigJSON(current), req)
	if err != nil {
		return domain.UpstreamModelBindingWrite{}, err
	}

	return domain.UpstreamModelBindingWrite{
		ModelCode:         modelCode,
		CapabilityType:    capabilityType,
		APIFormat:         apiFormat,
		UpstreamModelName: upstreamModelName,
		Status:            status,
		ConfigJSON:        configJSON,
	}, nil
}

func currentConfigJSON(current *domain.UpstreamModelBinding) []byte {
	if current == nil {
		return nil
	}
	return current.ConfigJSON
}

func defaultImageGenerationBindingPolicy() imageGenerationBindingPolicy {
	return imageGenerationBindingPolicy{
		StreamMode:         domain.ImageStreamModeForceSync,
		EditTransport:      domain.ImageEditTransportMultipart,
		MaxOutputCount:     domain.DefaultImageOutputCount,
		EditMaxOutputCount: domain.DefaultImageOutputCount,
	}
}

func parseImageGenerationBindingPolicy(raw []byte) imageGenerationBindingPolicy {
	policy := defaultImageGenerationBindingPolicy()
	if len(raw) == 0 {
		return policy
	}
	var cfg struct {
		ImageGeneration struct {
			StreamMode             string `json:"stream_mode"`
			EditTransport          string `json:"edit_transport"`
			UpstreamResponseFormat string `json:"upstream_response_format"`
			MaxOutputCount         int    `json:"max_output_count"`
			EditMaxOutputCount     int    `json:"edit_max_output_count"`
		} `json:"image_generation"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return policy
	}
	if cfg.ImageGeneration.StreamMode != "" {
		if normalized, ok := normalizeImageStreamMode(cfg.ImageGeneration.StreamMode); ok {
			policy.StreamMode = normalized
		}
	}
	if cfg.ImageGeneration.EditTransport != "" {
		policy.EditTransport = strings.TrimSpace(cfg.ImageGeneration.EditTransport)
	}
	if normalized, ok := normalizeImageResponseFormat(cfg.ImageGeneration.UpstreamResponseFormat); ok {
		policy.UpstreamResponseFormat = normalized
	}
	if validImageOutputCount(cfg.ImageGeneration.MaxOutputCount) {
		policy.MaxOutputCount = cfg.ImageGeneration.MaxOutputCount
	}
	if validImageOutputCount(cfg.ImageGeneration.EditMaxOutputCount) {
		policy.EditMaxOutputCount = cfg.ImageGeneration.EditMaxOutputCount
	}
	return policy
}

func mergeImageGenerationBindingPolicy(current []byte, req upstreamModelBindingWriteRequest) ([]byte, error) {
	cfg := map[string]any{}
	if len(current) > 0 {
		_ = json.Unmarshal(current, &cfg)
	}
	policy := parseImageGenerationBindingPolicy(current)
	if req.ImageStreamMode != "" {
		normalized, ok := normalizeImageStreamMode(req.ImageStreamMode)
		if !ok {
			return nil, domain.NewValidationError("image_stream_mode", "image_stream_mode must be auto, force_stream or force_sync")
		}
		policy.StreamMode = normalized
	}
	if req.ImageEditTransport != "" {
		normalized, ok := normalizeImageEditTransport(req.ImageEditTransport)
		if !ok {
			return nil, domain.NewValidationError("image_edit_transport", "image_edit_transport must be application/json or multipart/form-data")
		}
		policy.EditTransport = normalized
	}
	if req.ImageUpstreamResponseFormat != nil {
		normalized, ok := normalizeImageResponseFormat(*req.ImageUpstreamResponseFormat)
		if !ok {
			return nil, domain.NewValidationError("image_upstream_response_format", "image_upstream_response_format must be url, b64_json or empty")
		}
		policy.UpstreamResponseFormat = normalized
	}
	if req.ImageMaxOutputCount != nil {
		if !validImageOutputCount(*req.ImageMaxOutputCount) {
			return nil, domain.NewValidationError("image_max_output_count", "image_max_output_count must be between 1 and 10")
		}
		policy.MaxOutputCount = *req.ImageMaxOutputCount
	}
	if req.ImageEditMaxOutputCount != nil {
		if !validImageOutputCount(*req.ImageEditMaxOutputCount) {
			return nil, domain.NewValidationError("image_edit_max_output_count", "image_edit_max_output_count must be between 1 and 10")
		}
		policy.EditMaxOutputCount = *req.ImageEditMaxOutputCount
	}
	imageConfig := map[string]any{
		"stream_mode":           policy.StreamMode,
		"edit_transport":        policy.EditTransport,
		"max_output_count":      policy.MaxOutputCount,
		"edit_max_output_count": policy.EditMaxOutputCount,
	}
	if policy.UpstreamResponseFormat != "" {
		imageConfig["upstream_response_format"] = policy.UpstreamResponseFormat
	}
	cfg["image_generation"] = imageConfig
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func validImageOutputCount(value int) bool {
	return value >= domain.DefaultImageOutputCount && value <= domain.MaxImageOutputCount
}

func normalizeImageStreamMode(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "", domain.ImageStreamModeAuto:
		return domain.ImageStreamModeAuto, true
	case domain.ImageStreamModeForceStream:
		return domain.ImageStreamModeForceStream, true
	case domain.ImageStreamModeForceSync:
		return domain.ImageStreamModeForceSync, true
	default:
		return "", false
	}
}

func normalizeImageEditTransport(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "", domain.ImageEditTransportMultipart:
		return domain.ImageEditTransportMultipart, true
	case domain.ImageEditTransportJSON:
		return domain.ImageEditTransportJSON, true
	default:
		return "", false
	}
}

func normalizeImageResponseFormat(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "":
		return "", true
	case domain.ImageResponseFormatURL:
		return domain.ImageResponseFormatURL, true
	case domain.ImageResponseFormatB64:
		return domain.ImageResponseFormatB64, true
	default:
		return "", false
	}
}

func validateBindingCapabilityType(capabilityType string) error {
	switch domain.CapabilityType(capabilityType) {
	case domain.CapabilityChat,
		domain.CapabilityImage,
		domain.CapabilityVideo,
		domain.CapabilityEmbedding,
		domain.CapabilityAudioTTS,
		domain.CapabilityAudioSTT,
		domain.CapabilityRerank:
		return nil
	default:
		return domain.NewValidationError("capability_type", "unsupported capability_type")
	}
}

func validateBindingProtocol(field, protocol string) error {
	switch domain.UpstreamProtocol(protocol) {
	case domain.ProtocolOpenAIChat,
		domain.ProtocolOpenAIResponses,
		domain.ProtocolOpenAIEmbeddings,
		domain.ProtocolOpenAIImages,
		domain.ProtocolAnthropicMessages,
		domain.ProtocolGeminiGenerate,
		domain.ProtocolGeminiEmbeddings:
		return nil
	default:
		return domain.NewValidationError(field, "unsupported API format")
	}
}

func bindingProtocolSupportsCapability(protocol domain.UpstreamProtocol, capability domain.CapabilityType) bool {
	switch capability {
	case domain.CapabilityEmbedding:
		return protocol == domain.ProtocolOpenAIEmbeddings || protocol == domain.ProtocolGeminiEmbeddings
	case domain.CapabilityImage:
		return protocol == domain.ProtocolOpenAIImages || protocol == domain.ProtocolGeminiGenerate
	case domain.CapabilityChat:
		return protocol == domain.ProtocolOpenAIChat ||
			protocol == domain.ProtocolOpenAIResponses ||
			protocol == domain.ProtocolAnthropicMessages ||
			protocol == domain.ProtocolGeminiGenerate
	default:
		return true
	}
}

func fixedProviderEndpointProtocolFromAccount(endpointProtocol string) string {
	switch endpointProtocol {
	case string(domain.EndpointProtocolAnthropic):
		return string(domain.EndpointProtocolAnthropic)
	case string(domain.EndpointProtocolGemini):
		return string(domain.EndpointProtocolGemini)
	default:
		return string(domain.EndpointProtocolOpenAICompatible)
	}
}
