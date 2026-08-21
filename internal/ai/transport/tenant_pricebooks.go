package transport

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

type tenantPriceBookCloneRequest struct {
	Name string `json:"name,omitempty"`
}

type tenantPriceBookCloneInput struct {
	BookID string `path:"bookID"`
	Body   tenantPriceBookCloneRequest
}

type priceBookTransferBundle struct {
	SchemaVersion int                 `json:"schema_version"`
	Name          string              `json:"name"`
	Description   string              `json:"description,omitempty"`
	Entries       []priceBookEntryDTO `json:"entries"`
}

type priceBookTransferOutput struct{ Body priceBookTransferBundle }
type importTenantPriceBookInput struct{ Body priceBookTransferBundle }

func registerTenantPriceBooks(api huma.API, d AIDeps) {
	managerReady := func() error {
		if d.TenantPriceBooks == nil {
			return httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		return nil
	}
	syncReady := func() error {
		if d.PriceBookSync == nil {
			return httpx.ErrUnavailable.WithDetail("price book sync service is not configured")
		}
		return nil
	}
	tenantID := func(ctx context.Context) (string, error) {
		id := strings.TrimSpace(tenantIDFromContext(ctx))
		if id == "" {
			return "", httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		return id, nil
	}

	huma.Register(api, huma.Operation{OperationID: "ai-list-tenant-price-books", Method: http.MethodGet, Path: "/api/v1/tenants/me/price-books", Summary: "租户可见价格表", Tags: []string{"price-books"}},
		func(ctx context.Context, _ *struct{}) (*priceBooksOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			books, err := d.TenantPriceBooks.ListVisiblePriceBooks(ctx, tid)
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &priceBooksOutput{}
			out.Body.Items = make([]priceBookDTO, 0, len(books))
			for _, book := range books {
				out.Body.Items = append(out.Body.Items, priceBookToDTO(book))
			}
			out.Body.Total = len(out.Body.Items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-create-tenant-price-book", Method: http.MethodPost, Path: "/api/v1/tenants/me/price-books", Summary: "创建租户价格表", Tags: []string{"price-books"}, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *createPriceBookInput) (*priceBookOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			book, err := d.TenantPriceBooks.CreateTenantPriceBook(ctx, tid, in.Body.Name, in.Body.Description)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &priceBookOutput{Body: priceBookToDTO(book)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-get-tenant-price-book", Method: http.MethodGet, Path: "/api/v1/tenants/me/price-books/{bookID}", Summary: "租户价格表详情", Tags: []string{"price-books"}},
		func(ctx context.Context, in *getPriceBookInput) (*priceBookOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			book, err := d.TenantPriceBooks.GetVisiblePriceBook(ctx, tid, in.BookID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &priceBookOutput{Body: priceBookToDTO(book)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-update-tenant-price-book", Method: http.MethodPatch, Path: "/api/v1/tenants/me/price-books/{bookID}", Summary: "更新租户价格表", Tags: []string{"price-books"}},
		func(ctx context.Context, in *updatePriceBookInput) (*priceBookOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			book, err := d.TenantPriceBooks.UpdateTenantPriceBook(ctx, tid, in.BookID, in.Body.Name, in.Body.Description, in.Body.Status)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &priceBookOutput{Body: priceBookToDTO(book)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-delete-tenant-price-book", Method: http.MethodDelete, Path: "/api/v1/tenants/me/price-books/{bookID}", Summary: "删除租户价格表", Tags: []string{"price-books"}},
		func(ctx context.Context, in *deletePriceBookInput) (*deletePriceBookOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			if err := d.TenantPriceBooks.DeleteTenantPriceBook(ctx, tid, in.BookID); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletePriceBookOutput{}
			out.Body.Deleted = true
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-list-tenant-price-book-entries", Method: http.MethodGet, Path: "/api/v1/tenants/me/price-books/{bookID}/entries", Summary: "租户可见价格条目", Tags: []string{"price-books"}},
		func(ctx context.Context, in *priceBookEntriesInput) (*priceBookEntriesOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			entries, err := d.TenantPriceBooks.ListVisibleEntries(ctx, tid, in.BookID)
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

	huma.Register(api, huma.Operation{OperationID: "ai-upsert-tenant-price-book-entry", Method: http.MethodPut, Path: "/api/v1/tenants/me/price-books/{bookID}/entries/{modelCode}", Summary: "写入租户价格条目", Tags: []string{"price-books"}},
		func(ctx context.Context, in *upsertPriceBookEntryInput) (*priceBookEntryOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			entry, err := d.TenantPriceBooks.UpsertTenantEntry(ctx, tid, priceBookEntryFromRequest(in.BookID, in.ModelCode, in.Body))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &priceBookEntryOutput{Body: priceBookEntryToDTO(entry)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-delete-tenant-price-book-entry", Method: http.MethodDelete, Path: "/api/v1/tenants/me/price-books/{bookID}/entries/{modelCode}", Summary: "删除租户价格条目", Tags: []string{"price-books"}},
		func(ctx context.Context, in *priceBookEntryInput) (*deletePriceBookEntryOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			if err := d.TenantPriceBooks.DeleteTenantEntry(ctx, tid, in.BookID, in.ModelCode, in.CapabilityType); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletePriceBookEntryOutput{}
			out.Body.Deleted = true
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-search-tenant-litellm-price-models", Method: http.MethodGet, Path: "/api/v1/tenants/me/price-books/litellm/models", Summary: "搜索 LiteLLM 价格模型", Tags: []string{"price-books"}},
		func(ctx context.Context, in *priceBookLiteLLMModelsInput) (*liteLLMModelsOutput, error) {
			if err := syncReady(); err != nil {
				return nil, err
			}
			items, err := d.PriceBookSync.SearchLiteLLM(ctx, in.Q, in.Limit)
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &liteLLMModelsOutput{}
			out.Body.Items = items
			out.Body.Total = len(items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-import-tenant-price-book-litellm", Method: http.MethodPost, Path: "/api/v1/tenants/me/price-books/{bookID}/import-litellm", Summary: "导入 LiteLLM 价格", Tags: []string{"price-books"}},
		func(ctx context.Context, in *importLiteLLMInput) (*importLiteLLMOutput, error) {
			if err := syncReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			res, err := d.PriceBookSync.ImportTenantFromLiteLLM(ctx, tid, in.BookID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &importLiteLLMOutput{Body: res}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-sync-tenant-common-price-models", Method: http.MethodPost, Path: "/api/v1/tenants/me/price-books/{bookID}/sync-common", Summary: "同步常用模型价格", Tags: []string{"price-books"}},
		func(ctx context.Context, in *syncCommonModelsInput) (*syncCommonModelsOutput, error) {
			if err := syncReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			res, err := d.PriceBookSync.SyncTenantCommonModels(ctx, tid, in.BookID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &syncCommonModelsOutput{Body: res}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-clone-tenant-price-book", Method: http.MethodPost, Path: "/api/v1/tenants/me/price-books/{bookID}/clone", Summary: "克隆可见价格表", Tags: []string{"price-books"}, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *tenantPriceBookCloneInput) (*priceBookOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			book, err := d.TenantPriceBooks.CloneVisiblePriceBook(ctx, tid, in.BookID, in.Body.Name)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &priceBookOutput{Body: priceBookToDTO(book)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-export-tenant-price-book", Method: http.MethodGet, Path: "/api/v1/tenants/me/price-books/{bookID}/export", Summary: "导出可见价格表", Tags: []string{"price-books"}},
		func(ctx context.Context, in *getPriceBookInput) (*priceBookTransferOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			book, err := d.TenantPriceBooks.GetVisiblePriceBook(ctx, tid, in.BookID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			entries, err := d.TenantPriceBooks.ListVisibleEntries(ctx, tid, in.BookID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			bundle := priceBookTransferBundle{SchemaVersion: 1, Name: book.Name, Description: book.Description, Entries: make([]priceBookEntryDTO, 0, len(entries))}
			for _, entry := range entries {
				bundle.Entries = append(bundle.Entries, priceBookEntryToDTO(entry))
			}
			return &priceBookTransferOutput{Body: bundle}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-import-tenant-price-book", Method: http.MethodPost, Path: "/api/v1/tenants/me/price-books/import", Summary: "导入价格表文件", Tags: []string{"price-books"}, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *importTenantPriceBookInput) (*priceBookOutput, error) {
			if err := managerReady(); err != nil {
				return nil, err
			}
			tid, err := tenantID(ctx)
			if err != nil {
				return nil, err
			}
			if in.Body.SchemaVersion != 1 {
				return nil, httpx.ErrBadRequest.WithDetail("unsupported price book schema_version")
			}
			book, err := d.TenantPriceBooks.CreateTenantPriceBook(ctx, tid, in.Body.Name, in.Body.Description)
			if err != nil {
				return nil, mapServiceError(err)
			}
			for _, dto := range in.Body.Entries {
				entry := transferEntryToDomain(book.ID, dto)
				if _, err := d.TenantPriceBooks.UpsertTenantEntry(ctx, tid, entry); err != nil {
					return nil, mapServiceError(err)
				}
			}
			book, err = d.TenantPriceBooks.GetVisiblePriceBook(ctx, tid, book.ID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &priceBookOutput{Body: priceBookToDTO(book)}, nil
		})
}

func transferEntryToDomain(bookID string, dto priceBookEntryDTO) domain.PriceBookEntry {
	return domain.PriceBookEntry{
		PriceBookID: bookID, ModelCode: dto.ModelCode, CapabilityType: dto.CapabilityType,
		TokenPriceTiers:   tokenPriceTiersFromDTO(dto.TokenPriceTiers),
		ImageDefaultPrice: dto.ImageDefaultPrice, VideoDefaultPrice: dto.VideoDefaultPrice,
		ImagePrices: resolutionUSDFromDTO(dto.ImagePrices), VideoPrices: resolutionUSDFromDTO(dto.VideoPrices),
		AudioTTSPerChar: dto.AudioTTSPer1MChars / pricebookPerMillion, AudioSTTPerMinute: dto.AudioSTTPerMinute,
	}
}
