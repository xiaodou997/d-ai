package transport

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"xiaodou/dai/internal/auth"
)

type revocationCheckerStub struct {
	revokedJTI string
	logoutTime int64
}

func (s revocationCheckerStub) IsBlacklisted(tokenID string) bool {
	return tokenID == s.revokedJTI
}

func (s revocationCheckerStub) GetUserLogoutTime(string) int64 {
	return s.logoutTime
}

func TestTokenRevoked(t *testing.T) {
	issuedAt := time.Unix(100, 0)
	claims := &auth.Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:       "token-1",
			IssuedAt: jwt.NewNumericDate(issuedAt),
		},
	}

	if tokenRevoked(nil, claims) {
		t.Fatal("nil checker must not revoke tokens")
	}
	if !tokenRevoked(revocationCheckerStub{revokedJTI: "token-1"}, claims) {
		t.Fatal("blacklisted token ID must be rejected")
	}
	if !tokenRevoked(revocationCheckerStub{logoutTime: issuedAt.Unix() + 1}, claims) {
		t.Fatal("token issued before user logout must be rejected")
	}
	if tokenRevoked(revocationCheckerStub{logoutTime: issuedAt.Unix()}, claims) {
		t.Fatal("token issued at logout timestamp must remain valid")
	}
}
