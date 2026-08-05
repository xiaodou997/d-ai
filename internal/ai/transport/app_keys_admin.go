package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/libs/go/httpx"
)

type tenantRunKeysInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
}

type userRunKeysInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
}

func registerRunKeys(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-app-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/{tenantID}/app-keys",
		Summary:     "租户应用密钥列表",
		Description: "平台查询视图：返回指定租户拥有的应用密钥列表。仅返回 last_four，不返回明文 key/hash/ciphertext。",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, in *tenantRunKeysInput) (*runKeysOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		items, err := listRunKeysByScope(ctx, d, "tenant", in.TenantID, "")
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeysOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		out.Body.Included = buildIdentityIncludedForRunKeys(ctx, d, items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-app-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/app-keys",
		Summary:     "用户应用密钥列表",
		Description: "平台查询视图：返回指定租户下指定用户拥有的应用密钥列表。仅返回 last_four，不返回明文 key/hash/ciphertext。",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, in *userRunKeysInput) (*runKeysOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		items, err := listRunKeysByScope(ctx, d, "user", in.TenantID, in.UserID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeysOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		out.Body.Included = buildIdentityIncludedForRunKeys(ctx, d, items)
		return out, nil
	})
}

func listRunKeysByScope(ctx context.Context, d AIDeps, ownerType, tenantID, userID string) ([]runKeyDTO, error) {
	records, err := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres).ListInvokeKeys(ctx, application.InvokeKeyFilter{
		OwnerScope: ownerType,
		TenantID:   tenantID,
		UserID:     userID,
	})
	if err != nil {
		return nil, err
	}
	items := make([]runKeyDTO, 0, len(records))
	for _, record := range records {
		row := invokeKeyToRunKeyRow(record)
		populateRunKeyAppFields(ctx, d, &row)
		items = append(items, runKeyRowToDTO(row))
	}
	return items, nil
}
