package transport

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

const pricebookPerMillion = 1_000_000.0

type priceBookDTO struct {
	ID            string `json:"id" doc:"价格表 ID"`
	OwnerType     string `json:"owner_type" enum:"platform,tenant"`
	OwnerTenantID string `json:"owner_tenant_id,omitempty"`
	Writable      bool   `json:"writable"`
	Name          string `json:"name" doc:"价格表名称"`
	Description   string `json:"description" doc:"价格表描述"`
	Status        string `json:"status" enum:"active,disabled" doc:"状态"`
	Revision      int64  `json:"revision"`
	CreatedAt     *int64 `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt     *int64 `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type priceBooksOutput struct {
	Body struct {
		Items []priceBookDTO `json:"items"`
		Total int            `json:"total"`
	}
}

type createPriceBookInput struct {
	Body createPriceBookRequest
}

type createPriceBookRequest struct {
	Name        string `json:"name,omitempty" doc:"价格表名称"`
	Description string `json:"description,omitempty" doc:"价格表描述"`
}

type getPriceBookInput struct {
	BookID string `path:"bookID" doc:"价格表 ID"`
}

type priceBookOutput struct {
	Body priceBookDTO
}

type updatePriceBookInput struct {
	BookID string `path:"bookID" doc:"价格表 ID"`
	Body   updatePriceBookRequest
}

type updatePriceBookRequest struct {
	Name        string `json:"name,omitempty" doc:"价格表名称"`
	Description string `json:"description,omitempty" doc:"价格表描述"`
	Status      string `json:"status,omitempty" doc:"状态：active 或 disabled"`
}

type deletePriceBookInput struct {
	BookID string `path:"bookID" doc:"价格表 ID"`
}

type priceBookLiteLLMModelsInput struct {
	Q     string `query:"q" doc:"模型名搜索关键字；为空返回前 limit 条"`
	Limit int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"返回数量，默认 50，最大 100"`
}

type importLiteLLMInput struct {
	BookID string `path:"bookID" doc:"价格表 ID"`
}

type syncCommonModelsInput struct {
	BookID string `path:"bookID" doc:"价格表 ID"`
}

type deletePriceBookOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type priceBookEntriesInput struct {
	BookID string `path:"bookID" doc:"价格表 ID"`
}

type upsertPriceBookEntryInput struct {
	BookID    string `path:"bookID" doc:"价格表 ID"`
	ModelCode string `path:"modelCode" doc:"模型编码；如含 / 需 URL 编码"`
	Body      upsertPriceBookEntryRequest
}

type upsertPriceBookEntryRequest struct {
	CapabilityType     string              `json:"capability_type,omitempty" doc:"能力类型；为空默认 chat"`
	TokenPriceTiers    []tokenPriceTierDTO `json:"token_price_tiers,omitempty" doc:"按输入上下文选择的 token USD 价格档位"`
	ImageDefaultPrice  float64             `json:"image_default_price_usd,omitempty" doc:"图片默认 USD 单价（每张）；image 能力必填"`
	VideoDefaultPrice  float64             `json:"video_default_price_usd,omitempty" doc:"视频默认 USD 单价（每秒）；video 能力必填"`
	ImagePrices        []resolutionUSDDTO  `json:"image_prices,omitempty" doc:"图片按 1k、2k、4k 尺寸档位的 USD 覆盖单价"`
	VideoPrices        []resolutionUSDDTO  `json:"video_prices,omitempty" doc:"视频按规格的 USD 覆盖单价"`
	AudioTTSPer1MChars float64             `json:"audio_tts_per_1m_chars_usd,omitempty" doc:"TTS 每 100 万字符的 USD 单价"`
	AudioSTTPerMinute  float64             `json:"audio_stt_per_minute_usd,omitempty" doc:"STT 每分钟的 USD 单价"`
}

type priceBookEntryInput struct {
	BookID         string `path:"bookID" doc:"价格表 ID"`
	ModelCode      string `path:"modelCode" doc:"模型编码；如含 / 需 URL 编码"`
	CapabilityType string `query:"capability_type" default:"chat" doc:"能力类型"`
}

type resolutionUSDDTO struct {
	Resolution string  `json:"resolution" doc:"图片为 1k、2k 或 4k；视频为分辨率规格"`
	Price      float64 `json:"price" doc:"USD 单价，图片按张、视频按秒"`
}

type priceBookEntryDTO struct {
	ModelCode          string              `json:"model_code" doc:"模型编码"`
	CapabilityType     string              `json:"capability_type" doc:"能力类型"`
	TokenPriceTiers    []tokenPriceTierDTO `json:"token_price_tiers" doc:"按输入上下文选择的 token USD 价格档位"`
	ImageDefaultPrice  float64             `json:"image_default_price_usd" doc:"图片默认 USD 单价（每张）"`
	VideoDefaultPrice  float64             `json:"video_default_price_usd" doc:"视频默认 USD 单价（每秒）"`
	ImagePrices        []resolutionUSDDTO  `json:"image_prices,omitempty" doc:"图片按 1k、2k、4k 尺寸档位的 USD 覆盖单价"`
	VideoPrices        []resolutionUSDDTO  `json:"video_prices,omitempty" doc:"视频按规格的 USD 覆盖单价"`
	AudioTTSPer1MChars float64             `json:"audio_tts_per_1m_chars_usd" doc:"TTS 每 100 万字符的 USD 单价"`
	AudioSTTPerMinute  float64             `json:"audio_stt_per_minute_usd" doc:"STT 每分钟的 USD 单价"`
	Source             string              `json:"source" doc:"来源"`
	ManuallyEdited     bool                `json:"manually_edited" doc:"是否人工编辑"`
	UpdatedAt          *int64              `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type tokenPriceTierDTO struct {
	UpToInputTokens    *int    `json:"up_to_input_tokens" doc:"包含边界的最大输入 token；null 表示无上限"`
	InputPer1MUSD      float64 `json:"input_per_1m_usd" doc:"输入每 100 万 token 的 USD 单价"`
	OutputPer1MUSD     float64 `json:"output_per_1m_usd" doc:"输出每 100 万 token 的 USD 单价"`
	CacheWritePer1MUSD float64 `json:"cache_write_per_1m_usd" doc:"缓存写入每 100 万 token 的 USD 单价"`
	CacheReadPer1MUSD  float64 `json:"cache_read_per_1m_usd" doc:"缓存读取每 100 万 token 的 USD 单价"`
}

type priceBookEntriesOutput struct {
	Body struct {
		Items []priceBookEntryDTO `json:"items"`
		Total int                 `json:"total"`
	}
}

type priceBookEntryOutput struct {
	Body priceBookEntryDTO
}

type deletePriceBookEntryOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type liteLLMModelsOutput struct {
	Body struct {
		Items []billingcontrol.LiteLLMModelInfo `json:"items" doc:"LiteLLM 模型搜索结果"`
		Total int                               `json:"total" doc:"返回条数"`
	}
}

type importLiteLLMOutput struct {
	Body billingcontrol.ImportResult
}

type syncCommonModelsOutput struct {
	Body billingcontrol.SyncResult
}

func registerPriceBooks(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-price-books",
		Method:      http.MethodGet,
		Path:        "/api/v1/price-books",
		Summary:     "价格表列表",
		Description: "返回 AI 网关价格表基础信息。本端点为 Huma 只读契约，兼容写入口仍保留。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, _ *struct{}) (*priceBooksOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		books, err := d.PriceBookSvc.ListPriceBooks(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &priceBooksOutput{}
		out.Body.Items = make([]priceBookDTO, 0, len(books))
		for _, book := range books {
			item := priceBookToDTO(book)
			item.Writable = true
			out.Body.Items = append(out.Body.Items, item)
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-price-book",
		Method:      http.MethodPost,
		Path:        "/api/v1/price-books",
		Summary:     "创建价格表",
		Description: "创建 AI 网关价格表。状态由服务端初始化为 active。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *createPriceBookInput) (*priceBookOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		book, err := d.PriceBookSvc.CreatePriceBook(ctx, in.Body.Name, in.Body.Description)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &priceBookOutput{}
		out.Body = priceBookToDTO(book)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-price-book",
		Method:      http.MethodGet,
		Path:        "/api/v1/price-books/{bookID}",
		Summary:     "价格表详情",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *getPriceBookInput) (*priceBookOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		book, err := d.PriceBookSvc.GetPlatformPriceBook(ctx, in.BookID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &priceBookOutput{}
		out.Body = priceBookToDTO(book)
		out.Body.Writable = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-price-book",
		Method:      http.MethodPatch,
		Path:        "/api/v1/price-books/{bookID}",
		Summary:     "更新价格表",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *updatePriceBookInput) (*priceBookOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		book, err := d.PriceBookSvc.UpdatePriceBook(ctx, in.BookID, in.Body.Name, in.Body.Description, in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &priceBookOutput{}
		out.Body = priceBookToDTO(book)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-price-book",
		Method:      http.MethodDelete,
		Path:        "/api/v1/price-books/{bookID}",
		Summary:     "删除价格表",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *deletePriceBookInput) (*deletePriceBookOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		if err := d.PriceBookSvc.DeletePriceBook(ctx, in.BookID); err != nil {
			return nil, mapServiceError(err)
		}
		out := &deletePriceBookOutput{}
		out.Body.Deleted = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-search-litellm-price-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/price-books/litellm/models",
		Summary:     "搜索 LiteLLM 价格模型",
		Description: "从 LiteLLM 价格源中搜索模型，用于价格表条目自动填充。结果仅从内存缓存读取/刷新，不落库。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *priceBookLiteLLMModelsInput) (*liteLLMModelsOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		items, err := d.PriceBookSvc.SearchLiteLLM(ctx, in.Q, in.Limit)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &liteLLMModelsOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-price-book-entries",
		Method:      http.MethodGet,
		Path:        "/api/v1/price-books/{bookID}/entries",
		Summary:     "价格表条目列表",
		Description: "返回指定价格表中的模型 USD 定价，token/字符字段按每 100 万单位展示。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *priceBookEntriesInput) (*priceBookEntriesOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		entries, err := d.PriceBookSvc.ListEntries(ctx, in.BookID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &priceBookEntriesOutput{}
		out.Body.Items = make([]priceBookEntryDTO, 0, len(entries))
		for _, entry := range entries {
			out.Body.Items = append(out.Body.Items, priceBookEntryToDTO(entry))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-import-price-book-litellm",
		Method:      http.MethodPost,
		Path:        "/api/v1/price-books/{bookID}/import-litellm",
		Summary:     "导入 LiteLLM 价格",
		Description: "从 LiteLLM 价格源批量导入可识别模型。手动编辑过的条目不会被覆盖。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *importLiteLLMInput) (*importLiteLLMOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		res, err := d.PriceBookSvc.ImportFromLiteLLM(ctx, in.BookID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &importLiteLLMOutput{}
		out.Body = res
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-sync-common-price-book-models",
		Method:      http.MethodPost,
		Path:        "/api/v1/price-books/{bookID}/sync-common",
		Summary:     "同步常用模型价格",
		Description: "从 LiteLLM 价格源同步内置常用模型白名单。手动编辑过的条目不会被覆盖。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *syncCommonModelsInput) (*syncCommonModelsOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		res, err := d.PriceBookSvc.SyncCommonModels(ctx, in.BookID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &syncCommonModelsOutput{}
		out.Body = res
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-upsert-price-book-entry",
		Method:      http.MethodPut,
		Path:        "/api/v1/price-books/{bookID}/entries/{modelCode}",
		Summary:     "写入价格表条目",
		Description: "手动写入或更新指定模型的 USD 定价。token/字符字段按每 100 万单位接收，服务端落库为 per-token/per-char；写入后标记为 manual，后续 LiteLLM 导入不会覆盖。",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *upsertPriceBookEntryInput) (*priceBookEntryOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		entry, err := d.PriceBookSvc.UpsertEntry(ctx, priceBookEntryFromRequest(in.BookID, in.ModelCode, in.Body))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &priceBookEntryOutput{}
		out.Body = priceBookEntryToDTO(entry)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-price-book-entry",
		Method:      http.MethodDelete,
		Path:        "/api/v1/price-books/{bookID}/entries/{modelCode}",
		Summary:     "删除价格表条目",
		Tags:        []string{"price-books"},
	}, func(ctx context.Context, in *priceBookEntryInput) (*deletePriceBookEntryOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		if strings.TrimSpace(in.ModelCode) == "" {
			return nil, httpx.ErrBadRequest.WithDetail("modelCode is required")
		}
		if err := d.PriceBookSvc.DeleteEntry(ctx, in.BookID, in.ModelCode, in.CapabilityType); err != nil {
			return nil, mapServiceError(err)
		}
		out := &deletePriceBookEntryOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

func priceBookEntryFromRequest(bookID, modelCode string, req upsertPriceBookEntryRequest) domain.PriceBookEntry {
	capability := strings.TrimSpace(req.CapabilityType)
	if capability == "" {
		capability = "chat"
	}
	return domain.PriceBookEntry{
		PriceBookID:       bookID,
		ModelCode:         strings.TrimSpace(modelCode),
		CapabilityType:    capability,
		TokenPriceTiers:   tokenPriceTiersFromDTO(req.TokenPriceTiers),
		ImageDefaultPrice: req.ImageDefaultPrice,
		VideoDefaultPrice: req.VideoDefaultPrice,
		ImagePrices:       resolutionUSDFromDTO(req.ImagePrices),
		VideoPrices:       resolutionUSDFromDTO(req.VideoPrices),
		AudioTTSPerChar:   req.AudioTTSPer1MChars / pricebookPerMillion,
		AudioSTTPerMinute: req.AudioSTTPerMinute,
	}
}

func priceBookToDTO(book domain.PriceBook) priceBookDTO {
	return priceBookDTO{
		ID:            book.ID,
		OwnerType:     string(book.OwnerType),
		OwnerTenantID: book.OwnerTenantID,
		Writable:      book.OwnerType == domain.PriceBookOwnerTenant,
		Name:          book.Name,
		Description:   book.Description,
		Status:        book.Status,
		Revision:      book.Revision,
		CreatedAt:     timeToMillisPtr(book.CreatedAt),
		UpdatedAt:     timeToMillisPtr(book.UpdatedAt),
	}
}

func priceBookEntryToDTO(entry domain.PriceBookEntry) priceBookEntryDTO {
	return priceBookEntryDTO{
		ModelCode:          entry.ModelCode,
		CapabilityType:     entry.CapabilityType,
		TokenPriceTiers:    tokenPriceTiersToDTO(entry.TokenPriceTiers),
		ImageDefaultPrice:  domain.RoundUpCurrency2(entry.ImageDefaultPrice),
		VideoDefaultPrice:  domain.RoundUpCurrency2(entry.VideoDefaultPrice),
		ImagePrices:        resolutionUSDToDTO(entry.ImagePrices),
		VideoPrices:        resolutionUSDToDTO(entry.VideoPrices),
		AudioTTSPer1MChars: domain.RoundUpCurrency2(entry.AudioTTSPerChar * pricebookPerMillion),
		AudioSTTPerMinute:  entry.AudioSTTPerMinute,
		Source:             entry.Source,
		ManuallyEdited:     entry.ManuallyEdited,
		UpdatedAt:          timeToMillisPtr(entry.UpdatedAt),
	}
}

func tokenPriceTiersFromDTO(items []tokenPriceTierDTO) []domain.TokenPriceTier {
	out := make([]domain.TokenPriceTier, 0, len(items))
	for _, item := range items {
		out = append(out, domain.TokenPriceTier{
			UpToInputTokens:    item.UpToInputTokens,
			InputPerToken:      item.InputPer1MUSD / pricebookPerMillion,
			OutputPerToken:     item.OutputPer1MUSD / pricebookPerMillion,
			CacheWritePerToken: item.CacheWritePer1MUSD / pricebookPerMillion,
			CacheReadPerToken:  item.CacheReadPer1MUSD / pricebookPerMillion,
		})
	}
	return out
}

func tokenPriceTiersToDTO(items []domain.TokenPriceTier) []tokenPriceTierDTO {
	out := make([]tokenPriceTierDTO, 0, len(items))
	for _, item := range items {
		out = append(out, tokenPriceTierDTO{
			UpToInputTokens:    item.UpToInputTokens,
			InputPer1MUSD:      domain.RoundUpCurrency2(item.InputPerToken * pricebookPerMillion),
			OutputPer1MUSD:     domain.RoundUpCurrency2(item.OutputPerToken * pricebookPerMillion),
			CacheWritePer1MUSD: domain.RoundUpCurrency2(item.CacheWritePerToken * pricebookPerMillion),
			CacheReadPer1MUSD:  domain.RoundUpCurrency2(item.CacheReadPerToken * pricebookPerMillion),
		})
	}
	return out
}

func resolutionUSDToDTO(items []domain.ResolutionUSDPrice) []resolutionUSDDTO {
	if len(items) == 0 {
		return nil
	}
	out := make([]resolutionUSDDTO, 0, len(items))
	for _, item := range items {
		out = append(out, resolutionUSDDTO{Resolution: item.Resolution, Price: item.Price})
	}
	return out
}

func resolutionUSDFromDTO(items []resolutionUSDDTO) []domain.ResolutionUSDPrice {
	if len(items) == 0 {
		return nil
	}
	out := make([]domain.ResolutionUSDPrice, 0, len(items))
	for _, item := range items {
		out = append(out, domain.ResolutionUSDPrice{Resolution: strings.TrimSpace(item.Resolution), Price: item.Price})
	}
	return out
}

func timeToMillisPtr(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	v := t.UnixMilli()
	return &v
}
