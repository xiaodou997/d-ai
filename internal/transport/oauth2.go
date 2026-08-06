package transport

import (
	"context"
	"fmt"
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

// registerOAuth2Protected 注册 /api/oauth2 下需用户 JWT 的端点（userinfo/revoke/
// password）。登录与刷新统一通过 token 端点完成。
func registerOAuth2Protected(api huma.API, d Deps, mw huma.Middlewares) {
	// OIDC UserInfo
	huma.Register(api, huma.Operation{
		OperationID: "oauth2-userinfo",
		Method:      http.MethodGet,
		Path:        "/api/oauth2/userinfo",
		Summary:     "当前用户信息",
		Tags:        []string{"oauth2"},
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

	// 撤销当前 Access Token（加入黑名单）
	huma.Register(api, huma.Operation{
		OperationID: "oauth2-revoke",
		Method:      http.MethodPost,
		Path:        "/api/oauth2/revoke",
		Summary:     "登出（撤销当前 Token）",
		Tags:        []string{"oauth2"},
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

	// 修改密码
	huma.Register(api, huma.Operation{
		OperationID: "oauth2-change-password",
		Method:      http.MethodPut,
		Path:        "/api/oauth2/password",
		Summary:     "修改密码",
		Tags:        []string{"oauth2"},
		Middlewares: mw,
	}, func(ctx context.Context, in *changePasswordInput) (*messageOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		table, err := userTable(claims.UserType)
		if err != nil {
			return nil, err
		}

		var hash string
		if qerr := d.Pool.QueryRow(ctx,
			fmt.Sprintf("SELECT password_hash FROM %s WHERE user_id = $1", table), claims.UserID,
		).Scan(&hash); qerr != nil {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Body.OldPassword)) != nil {
			return nil, httpx.ErrBadRequest.WithDetail("旧密码不正确")
		}
		newHash, herr := bcrypt.GenerateFromPassword([]byte(in.Body.NewPassword), bcrypt.DefaultCost)
		if herr != nil {
			return nil, httpx.ErrInternal.WithCause(herr)
		}
		if _, uerr := d.Pool.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET password_hash = $1, updated_at = $2 WHERE user_id = $3", table),
			string(newHash), time.Now().UTC(), claims.UserID,
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
	switch claims.UserType {
	case 1, 2:
		return queryCurrentAdminSnapshot(ctx, d, claims)
	case 3:
		return queryCurrentTenantUserSnapshot(ctx, d, claims)
	case 4:
		return queryCurrentEndUserSnapshot(ctx, d, claims)
	default:
		return currentUserSnapshot{}, httpx.ErrBadRequest.WithDetail("无效的用户类型")
	}
}

func queryCurrentAdminSnapshot(ctx context.Context, d Deps, claims *auth.Claims) (currentUserSnapshot, error) {
	var snapshot currentUserSnapshot
	err := d.Pool.QueryRow(ctx, `
		SELECT user_id, username, user_type, status
		FROM iam_admins
		WHERE user_id = $1
	`, claims.UserID).Scan(&snapshot.userID, &snapshot.username, &snapshot.userType, &snapshot.status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return currentUserSnapshot{}, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		return currentUserSnapshot{}, httpx.ErrInternal.WithCause(err)
	}
	return snapshot, nil
}

func queryCurrentTenantUserSnapshot(ctx context.Context, d Deps, claims *auth.Claims) (currentUserSnapshot, error) {
	var snapshot currentUserSnapshot
	err := d.Pool.QueryRow(ctx, `
		SELECT u.user_id, u.username, u.tenant_id, u.status, COALESCE(t.tenant_name, '')
		FROM iam_tenant_users u
		LEFT JOIN iam_tenants t ON t.tenant_id = u.tenant_id
		WHERE u.user_id = $1
	`, claims.UserID).Scan(
		&snapshot.userID,
		&snapshot.username,
		&snapshot.tenantID,
		&snapshot.status,
		&snapshot.tenantName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return currentUserSnapshot{}, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		return currentUserSnapshot{}, httpx.ErrInternal.WithCause(err)
	}
	snapshot.userType = 3
	if claims.TenantID != "" && snapshot.tenantID != "" && claims.TenantID != snapshot.tenantID {
		return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("账户信息已变更，请重新登录")
	}
	return snapshot, nil
}

func queryCurrentEndUserSnapshot(ctx context.Context, d Deps, claims *auth.Claims) (currentUserSnapshot, error) {
	var snapshot currentUserSnapshot
	err := d.Pool.QueryRow(ctx, `
		SELECT u.user_id, u.username, u.tenant_id, u.status, COALESCE(t.tenant_name, '')
		FROM iam_users u
		LEFT JOIN iam_tenants t ON t.tenant_id = u.tenant_id
		WHERE u.user_id = $1
	`, claims.UserID).Scan(
		&snapshot.userID,
		&snapshot.username,
		&snapshot.tenantID,
		&snapshot.status,
		&snapshot.tenantName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return currentUserSnapshot{}, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		return currentUserSnapshot{}, httpx.ErrInternal.WithCause(err)
	}
	snapshot.userType = 4
	if claims.TenantID != "" && snapshot.tenantID != "" && claims.TenantID != snapshot.tenantID {
		return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("账户信息已变更，请重新登录")
	}
	return snapshot, nil
}

// userTable 按用户类型返回承载凭证的表名。
func userTable(userType int) (string, error) {
	switch userType {
	case 1, 2:
		return "iam_admins", nil
	case 3:
		return "iam_tenant_users", nil
	case 4:
		return "iam_users", nil
	default:
		return "", httpx.ErrBadRequest.WithDetail("无效的用户类型")
	}
}
