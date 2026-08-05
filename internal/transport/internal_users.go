package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/libs/go/httpx"
	tenantpg "xiaodou/dai/internal/tenant/pg"
	userpg "xiaodou/dai/internal/user/pg"
)

// ---- 输入/输出 DTO ----

type userIDInput struct {
	ID string `path:"id" doc:"用户 ID"`
}

type getUserOutput struct {
	Body *userpg.User
}

type internalEndUserLookupInput struct {
	ID       string `path:"id" doc:"终端用户 ID"`
	TenantID string `query:"tenant_id" doc:"租户 ID"`
}

type internalEndUserOutput struct {
	Body struct {
		UserID   string `json:"userId"`
		TenantID string `json:"tenantId"`
		Username string `json:"username"`
		Status   string `json:"status"`
	}
}

type batchUsersInput struct {
	Body struct {
		UserIDs []string `json:"userIds" minItems:"1" maxItems:"100" doc:"用户 ID 列表"`
	}
}

type batchUsersOutput struct {
	Body map[string]*userpg.User
}

type batchTenantsInput struct {
	Body struct {
		TenantIDs []string `json:"tenantIds" minItems:"1" maxItems:"100" doc:"租户 ID 列表"`
	}
}

type batchTenantsOutput struct {
	Body map[string]*tenantpg.Tenant
}

type updateUserInput struct {
	ID   string         `path:"id"`
	Body map[string]any `doc:"待更新字段（id/user_id/created_time/created_at 会被忽略）"`
}

type banUserInput struct {
	ID   string `path:"id"`
	Body struct {
		Reason string `json:"reason,omitempty"`
	}
}

type successOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

func okSuccess() *successOutput {
	out := &successOutput{}
	out.Body.Success = true
	return out
}

// registerInternalUsers 注册 /internal/v1/users/* 内部用户管理端点（Service Token）。
func registerInternalUsers(api huma.API, d Deps, mw huma.Middlewares) {
	svc := d.UserService
	tenantRepo := tenantpg.NewTenantRepository(d.Pool)

	huma.Register(api, huma.Operation{
		OperationID: "internal-get-end-user",
		Method:      http.MethodGet,
		Path:        "/internal/v1/end-users/{id}",
		Summary:     "按租户查询单个终端用户",
		Tags:        []string{"internal-users"},
		Middlewares: mw,
	}, func(ctx context.Context, in *internalEndUserLookupInput) (*internalEndUserOutput, error) {
		if d.Pool == nil {
			return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant_id is required")
		}
		out := &internalEndUserOutput{}
		err := d.Pool.QueryRow(ctx, `
			SELECT user_id, tenant_id, username, status
			FROM iam_users
			WHERE user_id = $1 AND tenant_id = $2
		`, in.ID, in.TenantID).Scan(&out.Body.UserID, &out.Body.TenantID, &out.Body.Username, &out.Body.Status)
		if err == pgx.ErrNoRows {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-get-user",
		Method:      http.MethodGet,
		Path:        "/internal/v1/users/{id}",
		Summary:     "查询单个用户",
		Tags:        []string{"internal-users"},
		Middlewares: mw,
	}, func(ctx context.Context, in *userIDInput) (*getUserOutput, error) {
		u, err := svc.GetUser(ctx, in.ID)
		if err != nil {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		return &getUserOutput{Body: u}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-batch-get-users",
		Method:      http.MethodPost,
		Path:        "/internal/v1/users/batch",
		Summary:     "批量查询用户",
		Tags:        []string{"internal-users"},
		Middlewares: mw,
	}, func(ctx context.Context, in *batchUsersInput) (*batchUsersOutput, error) {
		users, err := svc.BatchGetUsers(ctx, in.Body.UserIDs)
		if err != nil {
			return nil, toProblem(err)
		}
		return &batchUsersOutput{Body: users}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-batch-get-tenants",
		Method:      http.MethodPost,
		Path:        "/internal/v1/tenants/batch",
		Summary:     "批量查询租户",
		Tags:        []string{"internal-users"},
		Middlewares: mw,
	}, func(ctx context.Context, in *batchTenantsInput) (*batchTenantsOutput, error) {
		tenants, err := tenantRepo.GetByTenantIDs(ctx, in.Body.TenantIDs)
		if err != nil {
			return nil, toProblem(err)
		}
		result := make(map[string]*tenantpg.Tenant, len(tenants))
		for _, tenant := range tenants {
			result[tenant.TenantID] = tenant
		}
		return &batchTenantsOutput{Body: result}, nil
	})

}
