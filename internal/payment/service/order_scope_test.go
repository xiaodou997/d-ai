package service

import (
	"testing"

	"xiaodou/dai/internal/payment"
)

func TestOrderOwnedByScope(t *testing.T) {
	tests := []struct {
		name     string
		order    *payment.Order
		scene    string
		tenantID string
		userID   string
		want     bool
	}{
		{
			name:     "tenant order",
			order:    &payment.Order{Scene: payment.SceneTenantTopup, TenantID: "tenant-1"},
			scene:    payment.SceneTenantTopup,
			tenantID: "tenant-1",
			want:     true,
		},
		{
			name:     "tenant cannot read same tenant user order",
			order:    &payment.Order{Scene: payment.SceneUserTopup, TenantID: "tenant-1", UserID: "user-1"},
			scene:    payment.SceneTenantTopup,
			tenantID: "tenant-1",
			want:     false,
		},
		{
			name:     "user order",
			order:    &payment.Order{Scene: payment.SceneUserTopup, TenantID: "tenant-1", UserID: "user-1"},
			scene:    payment.SceneUserTopup,
			tenantID: "tenant-1",
			userID:   "user-1",
			want:     true,
		},
		{
			name:     "user cannot read tenant order",
			order:    &payment.Order{Scene: payment.SceneTenantTopup, TenantID: "tenant-1"},
			scene:    payment.SceneUserTopup,
			tenantID: "tenant-1",
			userID:   "user-1",
			want:     false,
		},
		{
			name:     "other user",
			order:    &payment.Order{Scene: payment.SceneUserTopup, TenantID: "tenant-1", UserID: "user-1"},
			scene:    payment.SceneUserTopup,
			tenantID: "tenant-1",
			userID:   "user-2",
			want:     false,
		},
		{
			name:     "other tenant",
			order:    &payment.Order{Scene: payment.SceneUserTopup, TenantID: "tenant-1", UserID: "user-1"},
			scene:    payment.SceneUserTopup,
			tenantID: "tenant-2",
			userID:   "user-1",
			want:     false,
		},
		{
			name:     "unknown scene",
			order:    &payment.Order{Scene: "unknown", TenantID: "tenant-1"},
			scene:    "unknown",
			tenantID: "tenant-1",
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := orderOwnedByScope(tt.order, tt.scene, tt.tenantID, tt.userID); got != tt.want {
				t.Fatalf("orderOwnedByScope() = %v, want %v", got, tt.want)
			}
		})
	}
	if orderOwnedByScope(nil, payment.SceneTenantTopup, "tenant-1", "") {
		t.Fatal("nil order must not be owned by any scope")
	}
}
