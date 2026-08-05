package transport

import (
	"errors"

	"xiaodou/dai/libs/go/httpx"
	serviceaccess "xiaodou/dai/internal/serviceaccess"
)

// serviceAccessHTTPError 将 serviceaccess 领域错误映射为 HTTP problem+json。
func serviceAccessHTTPError(err error) error {
	switch {
	case errors.Is(err, serviceaccess.ErrNotFound):
		return httpx.ErrNotFound.WithDetail("服务准入策略不存在")
	case errors.Is(err, serviceaccess.ErrInvalid):
		return httpx.ErrBadRequest.WithDetail(err.Error())
	case errors.Is(err, serviceaccess.ErrForbidden):
		return httpx.ErrForbidden.WithDetail("无权授予该服务范围")
	case errors.Is(err, serviceaccess.ErrDenied):
		return httpx.New("service_access_denied", 403, "Service Access Denied")
	case errors.Is(err, serviceaccess.ErrUnavailable):
		return httpx.New("service_access_unavailable", 503, "Service Access Unavailable")
	default:
		return httpx.ErrInternal.WithCause(err)
	}
}
