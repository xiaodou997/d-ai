package console

const (
	BizOK = 0

	// 1xxxx Auth / Permission
	BizErrTokenInvalid  = 10001
	BizErrTokenExpired  = 10002
	BizErrForbidden     = 10003
	BizErrMissingCtx    = 10004
	BizErrAccountBanned = 10005

	// 2xxxx Resource
	BizErrNotFound      = 20001
	BizErrAlreadyExists = 20002
	BizErrConflict      = 20003

	// 3xxxx Validation
	BizErrBadRequest   = 30001
	BizErrInvalidField = 30002
	BizErrMissingField = 30003

	// 5xxxx Server
	BizErrInternal            = 50001
	BizErrDatabase            = 50002
	BizErrUpstreamUnavailable = 50003
)
