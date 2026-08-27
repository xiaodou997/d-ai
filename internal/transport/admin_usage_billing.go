package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
)

type batchRefundUsageInput struct {
	Body struct {
		RequestIDs []string `json:"requestIds" minItems:"1" maxItems:"100"`
		Reason     string   `json:"reason"`
	}
}

type batchUsageOpOutput struct {
	Body struct {
		Succeeded      []string                  `json:"succeeded"`
		Failed         []billingsvc.BatchOpError `json:"failed"`
		TotalTenantUSD float64                   `json:"totalTenantUsd"`
		TotalUserUSD   float64                   `json:"totalUserUsd"`
		SuccessCount   int                       `json:"successCount"`
		FailCount      int                       `json:"failCount"`
	}
}

func registerAdminUsageBilling(api huma.API, d adminUsageBillingModule) {
	h := newAdminUsageBillingHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireCapability(api, auth.CapabilityPlatformAdmin)}

	huma.Register(api, huma.Operation{OperationID: "admin-batch-refund-usage", Method: http.MethodPost, Path: "/api/v1/ai/usage/batch-refund",
		Summary: "批量退款 AI 使用记录", Tags: []string{"admin-usage"}, Middlewares: sysUser}, h.batchRefundUsage)
}

func (h *adminHandlers) batchRefundUsage(ctx context.Context, in *batchRefundUsageInput) (*batchUsageOpOutput, error) {
	claims := userClaimsFromCtx(ctx)
	res := h.deduction.BatchRefundUsage(in.Body.RequestIDs, in.Body.Reason, userIDOf(claims))
	return batchUsageResult(res), nil
}

func batchUsageResult(res billingsvc.BatchOpResult) *batchUsageOpOutput {
	out := &batchUsageOpOutput{}
	out.Body.Succeeded = res.Succeeded
	out.Body.Failed = res.Failed
	out.Body.TotalTenantUSD = billingdomain.MicroToUSD(res.TotalTenantCredits)
	out.Body.TotalUserUSD = billingdomain.MicroToUSD(res.TotalUserCredits)
	out.Body.SuccessCount = len(res.Succeeded)
	out.Body.FailCount = len(res.Failed)
	return out
}
