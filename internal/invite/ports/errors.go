package ports

import "errors"

var ErrInvitationCodeNotFound = errors.New("invitation code not found")
var ErrInvitationCodeUnavailable = errors.New("invitation code unavailable")
var ErrInvalidInvitationCodeFormat = errors.New("invalid invitation code format")
var ErrUsernameExists = errors.New("username already exists")
var ErrEmailExists = errors.New("email already exists")
var ErrInvalidUsername = errors.New("invalid username")
var ErrLegalAcceptanceRequired = errors.New("current legal documents must be accepted")
