package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
)

// ---- DTO ----

type batchRefundInput struct {
	Body struct {
		EventIDs []string `json:"eventIds" minItems:"1" maxItems:"100"`
		Reason   string   `json:"reason"`
	}
}

type batchOpOutput struct {
	Body struct {
		Succeeded      []string                  `json:"succeeded"`
		Failed         []billingsvc.BatchOpError `json:"failed"`
		TotalTenantUSD float64                   `json:"totalTenantUsd"`
		TotalUserUSD   float64                   `json:"totalUserUsd"`
		SuccessCount   int                       `json:"successCount"`
		FailCount      int                       `json:"failCount"`
	}
}

// registerAdminBillingEvents 注册账务审计所需的批量退款端点。
func registerAdminBillingEvents(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}

	huma.Register(api, huma.Operation{OperationID: "admin-batch-refund-events", Method: http.MethodPost, Path: "/api/v1/billing/events/batch-refund",
		Summary: "批量退款", Tags: []string{"admin-billing-events"}, Middlewares: sysUser}, h.batchRefundEvents)
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
	out.Body.TotalTenantUSD = billingdomain.MicroToUSD(res.TotalTenantCredits)
	out.Body.TotalUserUSD = billingdomain.MicroToUSD(res.TotalUserCredits)
	out.Body.SuccessCount = len(res.Succeeded)
	out.Body.FailCount = len(res.Failed)
	return out
}
