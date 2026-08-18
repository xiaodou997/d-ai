package domain

import (
	"errors"
	"fmt"
)

// BizError 业务错误类型
type BizError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *BizError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Is 实现 errors.Is 接口
func (e *BizError) Is(target error) bool {
	t, ok := target.(*BizError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Unwrap 返回原因错误
func (e *BizError) Unwrap() error {
	if e.Detail != "" {
		return errors.New(e.Detail)
	}
	return nil
}

// ==================== 错误构造函数 ====================

// NewError 创建业务错误
func NewError(code int, message string) *BizError {
	return &BizError{Code: code, Message: message}
}

// NewErrorWithDetail 创建带详情的业务错误
func NewErrorWithDetail(code int, message, detail string) *BizError {
	return &BizError{Code: code, Message: message, Detail: detail}
}

// Wrap 包装错误
func Wrap(err error, code int, message string) *BizError {
	if err == nil {
		return nil
	}
	return &BizError{Code: code, Message: message, Detail: err.Error()}
}

// ==================== 预定义业务错误 ====================

var (
	// 通用业务错误 (1xxx)
	ErrBadRequest = &BizError{Code: 1000, Message: "请求参数错误"}
	ErrNotFound   = &BizError{Code: 1003, Message: "资源不存在"}
	// 注意：1001(未授权) 已迁移至 5006，1002(禁止访问) 已迁移至 5007
	// 注意：1004(服务器内部错误) 已废弃，改用 999999

	// 账户错误 (2xxx)
	ErrAccountNotFound           = &BizError{Code: 2001, Message: "账户不存在"}
	ErrAccountFrozen             = &BizError{Code: 2002, Message: "账户已冻结"}
	ErrAccountCanceled           = &BizError{Code: 2003, Message: "账户已注销"}
	ErrInsufficientBalance       = &BizError{Code: 2004, Message: "余额不足"}
	ErrAccountExists             = &BizError{Code: 2005, Message: "账户已存在"}
	ErrTenantInsufficientBalance = &BizError{Code: 2006, Message: "租户余额不足"}
	ErrUserInsufficientBalance   = &BizError{Code: 2007, Message: "用户余额不足"}
	ErrTenantSuspended           = &BizError{Code: 2008, Message: "租户已停用"}
	ErrTenantInOverdraft         = &BizError{Code: 2009, Message: "租户账户已透支，请充值后继续"}
	ErrUserInOverdraft           = &BizError{Code: 2010, Message: "用户账户已透支，请充值后继续"}
	ErrTenantOverdraftExceeded   = &BizError{Code: 2011, Message: "租户余额不足且透支额度已用尽"}
	ErrUserOverdraftExceeded     = &BizError{Code: 2012, Message: "用户余额不足且透支额度已用尽"}

	// 使用记录/充值错误 (3xxx)
	ErrUsageNotFound            = &BizError{Code: 3001, Message: "使用记录不存在"}
	ErrDuplicateRequest         = &BizError{Code: 3005, Message: "重复请求"}
	ErrInvalidAmount            = &BizError{Code: 3006, Message: "无效金额"}
	ErrRechargeNotFound         = &BizError{Code: 3007, Message: "充值记录不存在"}
	ErrRechargeNotReversible    = &BizError{Code: 3008, Message: "该充值不可撤销"}
	ErrRechargeAlreadyReversed  = &BizError{Code: 3009, Message: "充值已撤销"}
	ErrRechargeCreditsExhausted = &BizError{Code: 3010, Message: "充值余额已全部消耗，无法撤销"}

	// 资源包错误 (4xxx)
	ErrPackageNotFound  = &BizError{Code: 4001, Message: "资源包不存在"}
	ErrPackageExpired   = &BizError{Code: 4002, Message: "资源包已过期"}
	ErrPackageExhausted = &BizError{Code: 4003, Message: "资源包已耗尽"}
	ErrPackageRevoked   = &BizError{Code: 4004, Message: "资源包已撤销"}

	// 认证/权限错误 (5xxx)
	ErrInvalidToken       = &BizError{Code: 5001, Message: "无效的Token"}
	ErrTokenExpired       = &BizError{Code: 5002, Message: "Token已过期"}
	ErrTokenRevoked       = &BizError{Code: 5003, Message: "Token已撤销"}
	ErrInvalidCredentials = &BizError{Code: 5004, Message: "用户名或密码错误"}
	ErrUserDisabled       = &BizError{Code: 5005, Message: "账户已被禁用"}
	ErrUnauthorized       = &BizError{Code: 5006, Message: "未授权访问"} // 原编号 1001
	ErrForbidden          = &BizError{Code: 5007, Message: "禁止访问"}  // 原编号 1002

	// 在线支付/账户余额错误 (7xxx)
	ErrPaymentDisabled         = &BizError{Code: 7001, Message: "微信支付未启用"}
	ErrPaymentAmountOutOfRange = &BizError{Code: 7002, Message: "充值金额超出允许范围"}
	ErrPaymentOrderNotFound    = &BizError{Code: 7003, Message: "支付订单不存在"}
	ErrPaymentConfigIncomplete = &BizError{Code: 7004, Message: "微信商户配置不完整，请先在管理端补全"}
	ErrCashInsufficientBalance = &BizError{Code: 7005, Message: "现金余额不足"}
	ErrWithdrawalNotFound      = &BizError{Code: 7006, Message: "提现申请不存在"}
	ErrWithdrawalInvalidState  = &BizError{Code: 7007, Message: "提现申请状态已变化，请刷新后重试"}
	ErrPaymentRefundNotAllowed = &BizError{Code: 7008, Message: "该支付订单不能执行退款冲正"}
	ErrPaymentAlreadyRefunded  = &BizError{Code: 7009, Message: "该支付订单已经完成退款冲正"}
)

// CodeInternalError 系统内部错误码（不可预期的系统故障，非业务错误）
const CodeInternalError = 999999

// ==================== 辅助函数 ====================

// IsNotFoundError 判断是否为资源不存在错误
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrAccountNotFound) ||
		errors.Is(err, ErrUsageNotFound) ||
		errors.Is(err, ErrRechargeNotFound) ||
		errors.Is(err, ErrPackageNotFound)
}

// IsAuthError 判断是否为认证错误
func IsAuthError(err error) bool {
	var bizErr *BizError
	if errors.As(err, &bizErr) {
		return bizErr.Code >= 5000 && bizErr.Code < 6000
	}
	return false
}

// IsBusinessError 判断是否为业务错误
func IsBusinessError(err error) bool {
	var bizErr *BizError
	return errors.As(err, &bizErr)
}

// GetErrorCode 获取错误码
func GetErrorCode(err error) int {
	var bizErr *BizError
	if errors.As(err, &bizErr) {
		return bizErr.Code
	}
	return CodeInternalError
}

// GetErrorMessage 获取错误消息
func GetErrorMessage(err error) string {
	var bizErr *BizError
	if errors.As(err, &bizErr) {
		return bizErr.Message
	}
	return err.Error()
}
