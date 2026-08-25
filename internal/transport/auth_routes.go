package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	authports "xiaodou/dai/internal/auth/ports"
	"xiaodou/dai/libs/go/httpx"
)

type userInfoOutput struct {
	Body struct {
		Sub        string `json:"sub"`
		Username   string `json:"username"`
		UserType   int    `json:"userType"`
		TenantID   string `json:"tenantId"`
		TenantName string `json:"tenantName"`
		MFAEnabled bool   `json:"mfaEnabled"`
	}
}

type changePasswordInput struct {
	Body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword" minLength:"12" maxLength:"72" doc:"新密码必须符合统一密码策略"`
	}
}

type updateProfileInput struct {
	Body struct {
		Username *string `json:"username" doc:"新用户名"`
		Email    *string `json:"email" doc:"新邮箱，空字符串表示清除"`
	}
}

type messageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// registerAuthProtected 注册统一 Portal 的登录后账号端点。
func registerAuthProtected(api huma.API, d Deps, mw huma.Middlewares) {
	recent := append(append(huma.Middlewares{}, mw...), requestClientMetadata(api))
	logoutMiddleware := append(append(huma.Middlewares{}, mw...), requireSameOrigin(api))
	mfaConfirmLimiter := auth.NewScopedRateLimiter(d.Redis, "dai:auth:mfa-confirm:")
	recentAuthLimiter := auth.NewScopedRateLimiter(d.Redis, "dai:auth:recent-auth:")
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
		out.Body.MFAEnabled = snapshot.mfaEnabled
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-logout",
		Method:      http.MethodPost,
		Path:        "/api/auth/logout",
		Summary:     "登出（撤销当前会话）",
		Tags:        []string{"auth"},
		Middlewares: logoutMiddleware,
	}, func(ctx context.Context, _ *struct{}) (*authLogoutOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		if d.Sessions == nil {
			return nil, httpx.ErrUnavailable.WithDetail("会话服务不可用")
		}
		if err := d.Sessions.Revoke(ctx, claims.SessionID, "logout"); err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		if d.Blacklist != nil && d.Blacklist.IsEnabled() && claims.ExpiresAt != nil {
			if exp := time.Until(claims.ExpiresAt.Time); exp > 0 {
				_ = d.Blacklist.AddToBlacklist(claims.ID, exp)
			}
		}
		out := &authLogoutOutput{SetCookie: clearRefreshCookie(d.SecureCookies)}
		out.Body.Success = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-change-password",
		Method:      http.MethodPut,
		Path:        "/api/auth/password",
		Summary:     "修改密码",
		Tags:        []string{"auth"},
		Middlewares: recent,
	}, func(ctx context.Context, in *changePasswordInput) (*messageOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		if d.AuthAccountReader == nil || d.AuthAccountWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("账号服务不可用")
		}
		hash, qerr := d.AuthAccountReader.GetPasswordHash(ctx, claims.UserID, claims.UserType)
		if qerr != nil {
			if errors.Is(qerr, authports.ErrAccountNotFound) {
				return nil, httpx.ErrNotFound.WithDetail("用户不存在")
			}
			return nil, httpx.ErrInternal.WithCause(qerr)
		}
		if hash == "" {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Body.OldPassword)) != nil {
			return nil, httpx.ErrBadRequest.WithDetail("旧密码不正确")
		}
		if err := auth.ValidatePassword(in.Body.NewPassword, claims.Username); err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(auth.CurrentPasswordPolicy().Description)
		}
		newHash, herr := bcrypt.GenerateFromPassword([]byte(in.Body.NewPassword), bcrypt.DefaultCost)
		if herr != nil {
			return nil, httpx.ErrInternal.WithCause(herr)
		}
		updated, uerr := d.AuthAccountWriter.UpdatePassword(ctx, claims.UserID, claims.UserType, string(newHash))
		if uerr != nil {
			return nil, httpx.ErrInternal.WithCause(uerr)
		}
		if !updated {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		// 数据库触发器撤销全部 refresh session；Redis 立即拒绝现存 access token。
		if d.Blacklist != nil && d.Blacklist.IsEnabled() && claims.ID != "" {
			_ = d.Blacklist.AddToBlacklist(claims.ID, 2*time.Hour)
			_ = d.Blacklist.LogoutUser(claims.UserID)
		}

		out := &messageOutput{}
		out.Body.Message = "密码修改成功"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-update-profile",
		Method:      http.MethodPut,
		Path:        "/api/auth/profile",
		Summary:     "修改用户名/邮箱（仅租户用户和终端用户）",
		Tags:        []string{"auth"},
		Middlewares: mw,
	}, func(ctx context.Context, in *updateProfileInput) (*messageOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		if d.AuthAccountWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("账号服务不可用")
		}
		if claims.UserType != 3 && claims.UserType != 4 {
			return nil, httpx.ErrForbidden.WithDetail("仅租户用户和终端用户可修改用户名或邮箱")
		}

		usernameSet := in.Body.Username != nil
		emailSet := in.Body.Email != nil
		if !usernameSet && !emailSet {
			return nil, httpx.ErrBadRequest.WithDetail("至少提供一个需要更新的字段")
		}

		username := ""
		if usernameSet {
			username = auth.NormalizeUsername(*in.Body.Username)
			if username == "" {
				return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
			}
		}

		email := ""
		if emailSet {
			email = strings.TrimSpace(*in.Body.Email)
			if email != "" {
				if !strings.Contains(email, "@") {
					return nil, httpx.ErrBadRequest.WithDetail("邮箱格式不正确")
				}
			}
		}

		updated, err := d.AuthAccountWriter.UpdateProfile(ctx, authports.ProfileUpdate{
			UserID: claims.UserID, UserType: claims.UserType,
			UsernameSet: usernameSet, Username: username,
			EmailSet: emailSet, Email: email,
		})
		if err != nil {
			if authpg.IsUsernameTaken(err) {
				return nil, httpx.ErrConflict.WithDetail("用户名已被占用，请换一个")
			}
			if authpg.IsEmailTaken(err) {
				return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
			}
			return nil, httpx.ErrInternal.WithCause(err)
		}
		if !updated {
			return nil, httpx.ErrNotFound.WithDetail("用户不存在")
		}

		// 用户名变更后旧 token 中的 claim 已过期，强制重新登录
		if d.Blacklist != nil && d.Blacklist.IsEnabled() && claims.ID != "" {
			_ = d.Blacklist.AddToBlacklist(claims.ID, 2*time.Hour)
			_ = d.Blacklist.LogoutUser(claims.UserID)
		}

		out := &messageOutput{}
		out.Body.Message = "资料已更新，请重新登录"
		return out, nil
	})

	huma.Register(api, huma.Operation{OperationID: "auth-mfa-enroll", Method: http.MethodPost, Path: "/api/auth/mfa/enroll", Summary: "注册管理员 MFA", Tags: []string{"auth"}, Middlewares: recent}, func(ctx context.Context, _ *struct{}) (*mfaEnrollmentOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil || (claims.UserType != 1 && claims.UserType != 2) {
			return nil, httpx.ErrForbidden.WithDetail("仅平台管理员可注册 MFA")
		}
		if d.MFA == nil {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务不可用")
		}
		result, err := d.MFA.Enroll(ctx, claims.UserID, claims.Username)
		if errors.Is(err, auth.ErrMFAAlreadyEnabled) {
			return nil, httpx.ErrConflict.WithDetail("MFA 已启用")
		}
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &mfaEnrollmentOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "auth-mfa-confirm", Method: http.MethodPost, Path: "/api/auth/mfa/confirm", Summary: "确认管理员 MFA", Tags: []string{"auth"}, Middlewares: recent}, func(ctx context.Context, in *mfaCodeInput) (*messageOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil || (claims.UserType != 1 && claims.UserType != 2) {
			return nil, httpx.ErrForbidden.WithDetail("仅平台管理员可确认 MFA")
		}
		if d.MFA == nil {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务不可用")
		}
		dimensions := auth.LoginRateDimensions{Account: claims.UserID, IP: requestClientIP(ctx)}
		decision, err := mfaConfirmLimiter.Check(ctx, dimensions)
		if err != nil {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
		}
		if !decision.Allowed {
			return nil, httpx.ErrTooManyReqs.WithDetail("MFA 尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(decision.RetryAfter)})
		}
		if err := d.MFA.ConfirmEnrollment(ctx, claims.UserID, strings.TrimSpace(in.Body.Code)); err != nil {
			retryAfter, rateErr := mfaConfirmLimiter.RecordFailure(ctx, dimensions)
			if rateErr != nil {
				return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
			}
			if retryAfter > 0 {
				return nil, httpx.ErrTooManyReqs.WithDetail("MFA 尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(retryAfter)})
			}
			if errors.Is(err, auth.ErrInvalidMFACode) {
				return nil, httpx.ErrUnauthorized.WithDetail("MFA 验证码无效")
			}
			return nil, httpx.ErrInternal.WithCause(err)
		}
		if err := mfaConfirmLimiter.Reset(ctx, dimensions); err != nil {
			return nil, httpx.ErrUnavailable.WithDetail("MFA 服务暂不可用，请稍后重试")
		}
		if d.RecentAuth == nil || d.RecentAuth.Mark(ctx, claims.UserID, "totp_enrollment") != nil {
			return nil, httpx.ErrUnavailable.WithDetail("近期认证服务暂不可用")
		}
		return &messageOutput{Body: struct {
			Message string `json:"message"`
		}{Message: "MFA 已启用"}}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "auth-recent-auth", Method: http.MethodPost, Path: "/api/auth/recent-auth", Summary: "重新验证当前账号", Tags: []string{"auth"}, Middlewares: recent}, func(ctx context.Context, in *recentAuthInput) (*messageOutput, error) {
		claims := userClaimsFromCtx(ctx)
		if claims == nil {
			return nil, httpx.ErrUnauthorized
		}
		dimensions := auth.LoginRateDimensions{Account: claims.UserID, IP: requestClientIP(ctx)}
		decision, err := recentAuthLimiter.Check(ctx, dimensions)
		if err != nil {
			return nil, httpx.ErrUnavailable.WithDetail("重新认证服务暂不可用，请稍后重试")
		}
		if !decision.Allowed {
			return nil, httpx.ErrTooManyReqs.WithDetail("重新认证尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(decision.RetryAfter)})
		}
		if d.AuthLoginReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("重新认证服务不可用，请稍后重试")
		}
		u, err := d.AuthLoginReader.GetPortalUserForLogin(ctx, claims.Username)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Body.Password)) != nil {
			retryAfter, rateErr := recentAuthLimiter.RecordFailure(ctx, dimensions)
			if rateErr != nil {
				return nil, httpx.ErrUnavailable.WithDetail("重新认证服务暂不可用，请稍后重试")
			}
			if retryAfter > 0 {
				return nil, httpx.ErrTooManyReqs.WithDetail("重新认证尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(retryAfter)})
			}
			return nil, httpx.ErrUnauthorized.WithDetail("重新认证失败")
		}
		if u.MFAEnabled {
			if d.MFA == nil || !d.MFA.VerifyCode(ctx, claims.UserID, strings.TrimSpace(in.Body.Code)) {
				retryAfter, rateErr := recentAuthLimiter.RecordFailure(ctx, dimensions)
				if rateErr != nil {
					return nil, httpx.ErrUnavailable.WithDetail("重新认证服务暂不可用，请稍后重试")
				}
				if retryAfter > 0 {
					return nil, httpx.ErrTooManyReqs.WithDetail("重新认证尝试过于频繁，请稍后再试").WithMeta(map[string]any{"retryAfter": auth.RetryAfterSeconds(retryAfter)})
				}
				return nil, httpx.ErrUnauthorized.WithDetail("需要有效的 MFA 验证码")
			}
		}
		if err := recentAuthLimiter.Reset(ctx, dimensions); err != nil {
			return nil, httpx.ErrUnavailable.WithDetail("重新认证服务暂不可用，请稍后重试")
		}
		if d.RecentAuth == nil || d.RecentAuth.Mark(ctx, claims.UserID, "recent_auth") != nil {
			return nil, httpx.ErrUnavailable.WithDetail("近期认证服务暂不可用")
		}
		return &messageOutput{Body: struct {
			Message string `json:"message"`
		}{Message: "重新认证成功"}}, nil
	})
}

type currentUserSnapshot struct {
	userID     string
	username   string
	userType   int
	tenantID   string
	tenantName string
	mfaEnabled bool
	status     string
}

func loadCurrentUserSnapshot(ctx context.Context, d Deps, claims *auth.Claims) (currentUserSnapshot, error) {
	if d.AuthAccountReader == nil {
		return currentUserSnapshot{}, httpx.ErrUnavailable.WithDetail("账号服务不可用")
	}
	snapshot, err := queryCurrentUserSnapshot(ctx, d, claims)
	if err != nil {
		return currentUserSnapshot{}, err
	}
	if snapshot.status != "active" {
		return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("账户已被禁用，请重新登录")
	}

	if snapshot.userType == 3 || snapshot.userType == 4 {
		active, err := d.AuthAccountReader.CheckTenantActive(ctx, snapshot.tenantID)
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
	projected, err := d.AuthAccountReader.GetCurrentUserSnapshot(ctx, claims.UserID, claims.UserType)
	if err != nil {
		if errors.Is(err, authports.ErrAccountNotFound) {
			return currentUserSnapshot{}, httpx.ErrNotFound.WithDetail("用户不存在")
		}
		return currentUserSnapshot{}, httpx.ErrInternal.WithCause(err)
	}
	snapshot := currentUserSnapshot{
		userID: projected.UserID, username: projected.Username, userType: projected.UserType,
		tenantID: projected.TenantID, tenantName: projected.TenantName,
		mfaEnabled: projected.MFAEnabled, status: projected.Status,
	}
	if claims.TenantID != "" && snapshot.tenantID != "" && claims.TenantID != snapshot.tenantID {
		return currentUserSnapshot{}, httpx.ErrForbidden.WithDetail("账户信息已变更，请重新登录")
	}
	return snapshot, nil
}
