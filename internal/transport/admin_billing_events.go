package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/httpx"
	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
)

// ---- DTO ----

type manualConfirmInput struct {
	ID   string `path:"id"`
	Body struct {
		ActualTenantCredits int64  `json:"actualTenantCredits" required:"false"`
		ActualUserCredits   int64  `json:"actualUserCredits" required:"false"`
		Note                string `json:"note"`
	}
}

type dismissEventInput struct {
	ID   string `path:"id"`
	Body struct {
		Note string `json:"note"`
	}
}

type batchConfirmInput struct {
	Body struct {
		EventIDs []string `json:"eventIds" minItems:"1" maxItems:"100"`
		Note     string   `json:"note"`
	}
}

type batchRefundInput struct {
	Body struct {
		EventIDs []string `json:"eventIds" minItems:"1" maxItems:"100"`
		Reason   string   `json:"reason"`
	}
}

type batchOpOutput struct {
	Body struct {
		Succeeded          []string                  `json:"succeeded"`
		Failed             []billingsvc.BatchOpError `json:"failed"`
		TotalTenantCredits float64                   `json:"totalTenantCredits"`
		TotalUserCredits   float64                   `json:"totalUserCredits"`
		SuccessCount       int                       `json:"successCount"`
		FailCount          int                       `json:"failCount"`
	}
}

// registerAdminBillingEvents 注册计费事件人工干预端点（确认/免除/批量确认/批量退款）。
func registerAdminBillingEvents(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}

	huma.Register(api, huma.Operation{OperationID: "admin-manual-confirm-event", Method: http.MethodPost, Path: "/api/v1/billing/events/{id}/confirm",
		Summary: "手动确认已释放事件", Tags: []string{"admin-billing-events"}, Middlewares: sysUser}, h.manualConfirmEvent)
	huma.Register(api, huma.Operation{OperationID: "admin-dismiss-event", Method: http.MethodPost, Path: "/api/v1/billing/events/{id}/dismiss",
		Summary: "免除收费", Tags: []string{"admin-billing-events"}, Middlewares: sysUser}, h.dismissEvent)
	huma.Register(api, huma.Operation{OperationID: "admin-batch-confirm-events", Method: http.MethodPost, Path: "/api/v1/billing/events/batch-confirm",
		Summary: "批量手动确认", Tags: []string{"admin-billing-events"}, Middlewares: sysUser}, h.batchConfirmEvents)
	huma.Register(api, huma.Operation{OperationID: "admin-batch-refund-events", Method: http.MethodPost, Path: "/api/v1/billing/events/batch-refund",
		Summary: "批量退款", Tags: []string{"admin-billing-events"}, Middlewares: sysUser}, h.batchRefundEvents)
}

func (h *adminHandlers) manualConfirmEvent(ctx context.Context, in *manualConfirmInput) (*eventStatusOutput, error) {
	claims := userClaimsFromCtx(ctx)
	tenantMicro, err := billingdomain.CreditsToMicro(in.Body.ActualTenantCredits)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail(err.Error())
	}
	userMicro, err := billingdomain.CreditsToMicro(in.Body.ActualUserCredits)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail(err.Error())
	}
	if err := h.deduction.ManualConfirm(in.ID, tenantMicro, userMicro, userIDOf(claims), in.Body.Note); err != nil {
		return nil, toProblem(err)
	}
	out := &eventStatusOutput{}
	out.Body.EventID = in.ID
	out.Body.Status = "succeeded"
	return out, nil
}

func (h *adminHandlers) dismissEvent(ctx context.Context, in *dismissEventInput) (*eventStatusOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if err := h.deduction.AdminDismiss(in.ID, userIDOf(claims), in.Body.Note); err != nil {
		return nil, toProblem(err)
	}
	out := &eventStatusOutput{}
	out.Body.EventID = in.ID
	out.Body.Status = "cancelled"
	return out, nil
}

func (h *adminHandlers) batchConfirmEvents(ctx context.Context, in *batchConfirmInput) (*batchOpOutput, error) {
	claims := userClaimsFromCtx(ctx)
	res := h.deduction.BatchConfirm(in.Body.EventIDs, in.Body.Note, userIDOf(claims))
	return batchResult(res), nil
}

func (h *adminHandlers) batchRefundEvents(ctx context.Context, in *batchRefundInput) (*batchOpOutput, error) {
	claims := userClaimsFromCtx(ctx)
	res := h.deduction.BatchRefund(in.Body.EventIDs, in.Body.Reason, userIDOf(claims))
	return batchResult(res), nil
}

func batchResult(res billingsvc.BatchOpResult) *batchOpOutput {
	out := &batchOpOutput{}
	out.Body.Succeeded = res.Succeeded
	out.Body.Failed = res.Failed
	out.Body.TotalTenantCredits = billingdomain.MicroToCredits(res.TotalTenantCredits)
	out.Body.TotalUserCredits = billingdomain.MicroToCredits(res.TotalUserCredits)
	out.Body.SuccessCount = len(res.Succeeded)
	out.Body.FailCount = len(res.Failed)
	return out
}
