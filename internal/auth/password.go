package auth

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	PasswordMinLength       = 12
	PasswordMaxBytes        = 72
	PasswordRequiredClasses = 3
)

var ErrWeakPassword = errors.New("password does not satisfy policy")

type PasswordPolicy struct {
	MinLength                int    `json:"minLength"`
	MaxBytes                 int    `json:"maxBytes"`
	RequiredCharacterClasses int    `json:"requiredCharacterClasses"`
	Description              string `json:"description"`
}

func CurrentPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength: PasswordMinLength, MaxBytes: PasswordMaxBytes,
		RequiredCharacterClasses: PasswordRequiredClasses,
		Description:              "至少 12 个字符，大写字母、小写字母、数字、符号至少包含三类，且不能包含用户名",
	}
}

func ValidatePassword(password, username string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < PasswordMinLength || len(password) > PasswordMaxBytes {
		return ErrWeakPassword
	}
	classes := 0
	var lower, upper, digit, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	for _, present := range []bool{lower, upper, digit, symbol} {
		if present {
			classes++
		}
	}
	if classes < PasswordRequiredClasses {
		return ErrWeakPassword
	}
	normalizedUsername := strings.ToLower(strings.TrimSpace(username))
	if utf8.RuneCountInString(normalizedUsername) >= 4 && strings.Contains(strings.ToLower(password), normalizedUsername) {
		return ErrWeakPassword
	}
	return nil
}
