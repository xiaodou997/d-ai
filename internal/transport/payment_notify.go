package transport

import (
	"net/http"

	"go.uber.org/zap"

	paymentsvc "xiaodou/dai/internal/payment/service"
)

// paymentNotifyHandlers 承载微信支付回调（无认证，验签即鉴权）。必须走 chi 原生
// （RegisterRaw），因为 wechatpay-go 的 ParseNotifyRequest 需要访问原始 *http.Request——
// huma 的 JSON 契约会提前消费/改写 body，破坏验签所需的原始字节。
type paymentNotifyHandlers struct {
	svc *paymentsvc.PaymentService
	log *zap.Logger
}

const maxWechatNotifyBodySize = 1 << 20

func newPaymentNotifyHandlers(d paymentModule) *paymentNotifyHandlers {
	return &paymentNotifyHandlers{svc: d.service, log: d.logger}
}

// wechatNotify 处理 POST /api/v1/payments/wechat/notify。成功回微信规范 JSON；失败回 500
// JSON `{"code":"FAIL","message":...}` 触发微信按退避策略重试（Settle 可重入）。
func (h *paymentNotifyHandlers) wechatNotify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxWechatNotifyBodySize)
	if err := h.svc.HandleNotify(r.Context(), r); err != nil {
		h.log.Error("[微信支付回调] 处理失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "FAIL", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": "SUCCESS", "message": "成功"})
}
