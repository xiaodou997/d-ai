package service

import (
	"testing"

	"xiaodou/dai/internal/payment"
)

func TestOrderOwnedByScope(t *testing.T) {
	order := &payment.Order{TenantID: "tenant-1", UserID: "user-1"}
	tests := []struct {
		name     string
		tenantID string
		userID   string
		want     bool
	}{
		{name: "tenant order", tenantID: "tenant-1", want: true},
		{name: "user order", tenantID: "tenant-1", userID: "user-1", want: true},
		{name: "other user", tenantID: "tenant-1", userID: "user-2", want: false},
		{name: "other tenant", tenantID: "tenant-2", userID: "user-1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := orderOwnedByScope(order, tt.tenantID, tt.userID); got != tt.want {
				t.Fatalf("orderOwnedByScope() = %v, want %v", got, tt.want)
			}
		})
	}
	if orderOwnedByScope(nil, "tenant-1", "") {
		t.Fatal("nil order must not be owned by any scope")
	}
}
