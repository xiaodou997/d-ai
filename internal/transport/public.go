package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/config"
	invitepkg "xiaodou/dai/internal/invite"
	invitepg "xiaodou/dai/internal/invite/pg"
	"xiaodou/dai/libs/go/httpx"
)

type publicHandlers struct {
	invite *invitepkg.InviteService
	legal  config.LegalConfig
}

func newPublicHandlers(d Deps) *publicHandlers {
	return &publicHandlers{
		invite: d.Invite,
		legal:  d.Legal,
	}
}

type publicInvitationInput struct {
	Code string `path:"code"`
}

type publicInvitationOutput struct {
	Body struct {
		Code             string                           `json:"code"`
		TenantName       string                           `json:"tenantName"`
		CustomerSiteName string                           `json:"customerSiteName"`
		FaviconPath      string                           `json:"faviconPath,omitempty"`
		Description      string                           `json:"description"`
		ExpireTime       *int64                           `json:"expiresAt,omitempty"`
		Status           invitepkg.PublicInvitationStatus `json:"status"`
		CanRegister      bool                             `json:"canRegister"`
		Message          string                           `json:"message"`
		Legal            struct {
			TermsURL       string `json:"termsUrl"`
			TermsVersion   string `json:"termsVersion"`
			PrivacyURL     string `json:"privacyUrl"`
			PrivacyVersion string `json:"privacyVersion"`
		} `json:"legal"`
	}
}

type publicRegistrationInput struct {
	Code string `path:"code"`
	Body struct {
		Username       string  `json:"username"`
		Password       string  `json:"password" minLength:"6"`
		Email          *string `json:"email" required:"false"`
		Phone          *string `json:"phone" required:"false"`
		TermsVersion   string  `json:"termsVersion" minLength:"1"`
		PrivacyVersion string  `json:"privacyVersion" minLength:"1"`
	}
}

type publicRegistrationOutput struct {
	Body struct {
		Success bool   `json:"success"`
		UserID  string `json:"userId"`
		Message string `json:"message"`
	}
}

// registerPublic 注册无需认证的公开端点（邀请查询 + 邀请注册）。
func registerPublic(api huma.API, d Deps) {
	h := newPublicHandlers(d)

	huma.Register(api, huma.Operation{
		OperationID: "public-get-invitation",
		Method:      http.MethodGet,
		Path:        "/api/v1/public/invitations/{code}",
		Summary:     "公开查看邀请码",
		Tags:        []string{"public"},
	}, h.getInvitation)

	huma.Register(api, huma.Operation{
		OperationID:   "public-register-invitation-user",
		Method:        http.MethodPost,
		Path:          "/api/v1/public/invitations/{code}/registrations",
		Summary:       "通过邀请注册链接注册终端用户",
		Tags:          []string{"public"},
		DefaultStatus: http.StatusCreated,
	}, h.registerInvitation)
}

func (h *publicHandlers) getInvitation(ctx context.Context, in *publicInvitationInput) (*publicInvitationOutput, error) {
	view, err := h.invite.DescribePublicInvitation(ctx, in.Code)
	if err != nil {
		if errors.Is(err, invitepkg.ErrInvalidInvitationCodeFormat) {
			return nil, httpx.ErrBadRequest.WithDetail("邀请码格式无效")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}

	out := &publicInvitationOutput{}
	out.Body.Code = view.Code
	out.Body.TenantName = view.TenantName
	out.Body.CustomerSiteName = view.CustomerSiteName
	if view.FaviconVersion > 0 {
		out.Body.FaviconPath = publicTenantFaviconPath(view.TenantID, view.FaviconVersion)
	}
	out.Body.Description = view.Description
	out.Body.ExpireTime = view.ExpireTime
	out.Body.Status = view.Status
	out.Body.CanRegister = view.CanRegister
	out.Body.Message = view.Message
	out.Body.Legal.TermsURL = legalDocumentURL(ctx, "terms")
	out.Body.Legal.TermsVersion = h.legal.TermsVersion
	out.Body.Legal.PrivacyURL = legalDocumentURL(ctx, "privacy")
	out.Body.Legal.PrivacyVersion = h.legal.PrivacyVersion
	return out, nil
}

func (h *publicHandlers) registerInvitation(ctx context.Context, in *publicRegistrationInput) (*publicRegistrationOutput, error) {
	if in.Body.TermsVersion != h.legal.TermsVersion || in.Body.PrivacyVersion != h.legal.PrivacyVersion {
		return nil, httpx.ErrBadRequest.WithDetail("请重新阅读并同意最新服务条款和隐私政策")
	}

	user, err := h.invite.RegisterUser(ctx, in.Code, in.Body.Username, in.Body.Password, in.Body.Email, in.Body.Phone, invitepkg.LegalAcceptance{
		TermsVersion:   in.Body.TermsVersion,
		PrivacyVersion: in.Body.PrivacyVersion,
	})
	if err != nil {
		switch {
		case errors.Is(err, invitepkg.ErrUsernameExists):
			return nil, httpx.ErrConflict.WithDetail("用户名已存在")
		case errors.Is(err, invitepkg.ErrEmailExists):
			return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
		case errors.Is(err, invitepkg.ErrInvalidUsername):
			return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
		case errors.Is(err, invitepkg.ErrInvalidInvitationCodeFormat):
			return nil, httpx.ErrBadRequest.WithDetail("邀请码格式无效")
		case errors.Is(err, invitepkg.ErrLegalAcceptanceRequired):
			return nil, httpx.ErrBadRequest.WithDetail("请同意服务条款和隐私政策")
		case errors.Is(err, invitepg.ErrInvitationCodeNotFound):
			return nil, httpx.ErrConflict.WithDetail("邀请码不存在")
		case errors.Is(err, invitepkg.ErrInvitationCodeUnavailable):
			return nil, httpx.ErrConflict.WithDetail("邀请码不可用")
		default:
			return nil, httpx.ErrInternal.WithCause(err)
		}
	}

	out := &publicRegistrationOutput{}
	out.Body.Success = true
	out.Body.UserID = user.UserID
	out.Body.Message = "registered"
	return out, nil
}
