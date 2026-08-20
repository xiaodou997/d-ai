package transport

import (
	"testing"

	"xiaodou/dai/internal/auth"
)

func TestIsUserAccessClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims *auth.Claims
		want   bool
	}{
		{name: "access token", claims: &auth.Claims{PrincipalType: "user", TokenUse: "access", SessionID: "session-1"}, want: true},
		{name: "legacy access without session", claims: &auth.Claims{PrincipalType: "user", TokenUse: "access"}},
		{name: "refresh token", claims: &auth.Claims{PrincipalType: "user", TokenUse: "refresh"}},
		{name: "non-user principal", claims: &auth.Claims{PrincipalType: "admin", TokenUse: "access"}},
		{name: "missing claims", claims: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUserAccessClaims(tt.claims); got != tt.want {
				t.Fatalf("isUserAccessClaims() = %v, want %v", got, tt.want)
			}
		})
	}
}
