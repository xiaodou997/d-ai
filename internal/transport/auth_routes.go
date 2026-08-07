package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	"xiaodou/dai/libs/go/httpx"
)

type userInfoOutput struct {
	Body struct {
		Sub        string `json:"sub"`
		Username   string `json:"username"`
		UserType   int    `json:"userType"`
		TenantID   string `json:"tenantId"`
		TenantName string `json:"tenantName"`
	}
}

type changePasswordInput struct {
	Body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword" minLength:"6" doc:"新密码至少 6 位"`
	}
}

type messageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// registerAuthProtected 注册统一 Portal 的登录后账号端点。
func registerAuthProtected(api huma.API, d Deps, mw huma.Middlewares) {
	huma.Register(api, huma.Operation{
		OperationID: "auth-current-user",
		Method:      http.MethodGet,
		Path:        "/api/auth/me",
		Summary:     "当前用户信息",
		Tags:        []string{"auth"},
		Middlewares: mw,
	}, func(ctx context.Context, _ *struct{}) (*userInfoOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}

		snapshot, err := loadCurrentUserSnapshot(ctx, d, claims)
		if err != nil {
			return nil, err
		}

		out := &userInfoOutput{}
		out.Body.Sub = snapshot.userID
		out.Body.Username = snapshot.username
		out.Body.UserType = snapshot.userType
		out.Body.TenantID = snapshot.tenantID
		out.Body.TenantName = snapshot.tenantName
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-logout",
		Method:      http.MethodPost,
		Path:        "/api/auth/logout",
		Summary:     "登出（撤销当前 Token）",
		Tags:        []string{"auth"},
		Middlewares: mw,
	}, func(ctx context.Context, _ *struct{}) (*successOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		if d.Blacklist != nil && d.Blacklist.IsEnabled() && claims.ExpiresAt != nil {
			if exp := time.Until(claims.ExpiresAt.Time); exp > 0 {
				_ = d.Blacklist.AddToBlacklist(claims.ID, exp)
			}
		}
		return okSuccess(), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-change-password",
		Method:      http.MethodPut,
		Path:        "/api/auth/password",
		Summary:     "修改密码",
		Tags:        []string{"auth"},
		Middlewares: mw,
	}, func(ctx context.Context, in *changePasswordInput) (*messageOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		var hash string
		if qerr := d.Pool.QueryRow(ctx, `
				SELECT password_hash FROM iam_accounts WHERE user_id = $1 AND user_type = $2
			`, claims.UserID, claims.UserType).Scan(&hash); qerr != nil {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Body.OldPassword)) != nil {
			return nil, httpx.ErrBadRequest.WithDetail("旧密码不正确")
		}
		newHash, herr := bcrypt.GenerateFromPassword([]byte(in.Body.NewPassword), bcrypt.DefaultCost)
		if herr != nil {
			return nil, httpx.ErrInternal.WithCause(herr)
		}
		if _, uerr := d.Pool.Exec(ctx, `
				UPDATE iam_accounts SET password_hash = $1, updated_at = $2
				WHERE user_id = $3 AND user_type = $4
			`, string(newHash), time.Now().UTC(), claims.UserID, claims.UserType,
		); uerr != nil {
			return nil, httpx.ErrInternal.WithCause(uerr)
		}
		// 使旧 token 失效
		if d.Blacklist != nil && d.Blacklist.IsEnabled() && claims.ID != "" {
			_ = d.Blacklist.AddToBlacklist(claims.ID, 2*time.Hour)
		}

		out := &messageOutput{}
		out.Body.Message = "密码修改成功"
		return out, nil
	})
}

type currentUserSnapshot struct {
	userID     string
	username   string
	userType   int
	tenantID   string
	tenantName string
	status     string
}

func loadCurrentUserSnapshot(ctx context.Context, d Deps, claims *auth.Claims) (currentUserSnapshot, error) {
	snapshot, err := queryCurrentUserSnapshot(ctx, d, claims)
	if err != nil {
		return currentUserSnapshot{}, err
	}
	if snapshot.status != "active" {
		return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("账户已被禁用，请重新登录")
	}

	repo := authpg.NewAuthRepository(d.Pool)
	if snapshot.userType == 3 || snapshot.userType == 4 {
		active, err := repo.CheckTenantActive(ctx, snapshot.tenantID)
		if err != nil {
			return currentUserSnapshot{}, httpx.ErrInternal.WithCause(err)
		}
		if !active {
			return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("租户已被停用或暂停，请重新登录")
		}
	}
	return snapshot, nil
}

func queryCurrentUserSnapshot(ctx context.Context, d Deps, claims *auth.Claims) (currentUserSnapshot, error) {
	if claims.UserType < 1 || claims.UserType > 4 {
		return currentUserSnapshot{}, httpx.ErrBadRequest.WithDetail("无效的用户类型")
	}
	var snapshot currentUserSnapshot
	err := d.Pool.QueryRow(ctx, `
		SELECT u.user_id, u.username, u.user_type, COALESCE(u.tenant_id, ''),
		       COALESCE(t.tenant_name, ''), u.status
		FROM iam_accounts u
		LEFT JOIN iam_tenants t ON t.tenant_id = u.tenant_id
		WHERE u.user_id = $1 AND u.user_type = $2
	`, claims.UserID, claims.UserType).Scan(
		&snapshot.userID,
		&snapshot.username,
		&snapshot.userType,
		&snapshot.tenantID,
		&snapshot.tenantName,
		&snapshot.status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return currentUserSnapshot{}, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		return currentUserSnapshot{}, httpx.ErrInternal.WithCause(err)
	}
	if claims.TenantID != "" && snapshot.tenantID != "" && claims.TenantID != snapshot.tenantID {
		return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("账户信息已变更，请重新登录")
	}
	return snapshot, nil
}
