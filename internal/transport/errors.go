// Package transport 是 D-AI 的 HTTP 层：用 chi + Huma（code-first）注册
// 端点，handler 仅做请求绑定、调用领域服务、错误归一，不含业务逻辑。领域层返回的
// *domain.BizError 在此映射为统一的 RFC 7807 problem+json（libs/httpx.AppError）。
package transport

import (
	"errors"
	"net/http"
	"strconv"

	"xiaodou/dai/internal/domain"
	"xiaodou/dai/libs/go/httpx"
)

// toProblem 把领域错误转换为统一 problem+json 错误。*domain.BizError 按业务码映射
// HTTP 状态，并把整数业务码原样保留为 problem 的 code 扩展字段（忠于 v1 语义，前端
// 低成本迁移）。非业务错误回退 500 internal，不泄露底层细节。
func toProblem(err error) error {
	if err == nil {
		return nil
	}
	if be, ok := errors.AsType[*domain.BizError](err); ok {
		return &httpx.AppError{
			Code:   strconv.Itoa(be.Code),
			Status: statusForBizCode(be.Code),
			Title:  be.Message,
			Detail: be.Detail,
		}
	}
	return httpx.ErrInternal.WithCause(err)
}

// statusForBizCode 把 v1 的整数业务码映射为 HTTP 状态码。先列特例，再按千位段兜底。
func statusForBizCode(code int) int {
	switch code {
	case 1000, 3006, 7002, 7004: // 参数错误 / 无效金额 / 充值金额超限 / 商户配置不完整
		return http.StatusBadRequest
	case 1003, 2001, 3001, 3007, 3011, 4001, 7003, 7006: // 各类资源不存在
		return http.StatusNotFound
	case 2004, 2006, 2007, 2009, 2010, 2011, 2012, 7005: // 余额不足 / 透支
		return http.StatusPaymentRequired
	case 2008, 5005, 5007, 7001: // 租户停用 / 账户禁用 / 禁止访问 / 支付未启用
		return http.StatusForbidden
	case 2002, 2003, 2005, 3002, 3003, 3004, 3005, 3008, 3009, 3010, 3012, 3013, 3014, 4002, 4003, 4004, 7007: // 状态冲突 / 重复
		return http.StatusConflict
	case domain.CodeInternalError:
		return http.StatusInternalServerError
	}
	switch {
	case code >= 5000 && code < 6000: // 其余认证错误
		return http.StatusUnauthorized
	case code >= 1000 && code < 5000: // 其余业务校验类
		return http.StatusBadRequest
	}
	return http.StatusBadRequest
}
