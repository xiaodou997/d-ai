package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/payment"
)

func TestPaymentScopeUsesCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		claims     *auth.Claims
		wantScene  string
		wantTenant string
		wantUser   string
		wantOK     bool
	}{
		{"tenant", &auth.Claims{UserID: "tu-1", TenantID: "t-1", UserType: 3}, payment.SceneTenantTopup, "t-1", "", true},
		{"customer", &auth.Claims{UserID: "u-1", TenantID: "t-1", UserType: 4}, payment.SceneUserTopup, "t-1", "u-1", true},
		{"admin", &auth.Claims{UserID: "a-1", UserType: 2}, "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), userClaimsCtxKey, tt.claims)
			scene, tenant, user, ok := sceneAndScopeFromClaims(ctx)
			if scene != tt.wantScene || tenant != tt.wantTenant || user != tt.wantUser || ok != tt.wantOK {
				t.Fatalf("scope = %q/%q/%q/%v, want %q/%q/%q/%v", scene, tenant, user, ok, tt.wantScene, tt.wantTenant, tt.wantUser, tt.wantOK)
			}
		})
	}
}
