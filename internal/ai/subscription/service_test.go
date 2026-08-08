package subscription

import (
	"testing"
)

func TestValidatePlanShape(t *testing.T) {
	w7d := int64(123)

	tests := []struct {
		name         string
		durationDays int32
		price        int64
		totalLimit   int64
		window7d     *int64
		wantErr      error
	}{
		{
			name:         "short plan without 7d window",
			durationDays: 3,
			price:        100,
			totalLimit:   1_000,
		},
		{
			name:         "short plan with 7d window",
			durationDays: 3,
			price:        100,
			totalLimit:   1_000,
			window7d:     &w7d,
			wantErr:      ErrPlanWindow7dInvalid,
		},
		{
			name:         "non-positive quota",
			durationDays: 7,
			price:        0,
			totalLimit:   1_000,
			wantErr:      ErrPlanQuotaInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePlanShape(tc.durationDays, tc.price, tc.totalLimit, nil, tc.window7d)
			if err != tc.wantErr {
				t.Fatalf("validatePlanShape() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestClassifyLiveSubscriptionsUsesEarliestPending(t *testing.T) {
	live := []Subscription{
		{ID: "pending-first", Status: SubPending},
		{ID: "pending-second", Status: SubPending},
		{ID: "active", Status: SubActive},
	}
	active, pending := classifyLiveSubscriptions(live)
	if active == nil || active.ID != "active" {
		t.Fatalf("active = %#v", active)
	}
	if pending == nil || pending.ID != "pending-first" {
		t.Fatalf("earliest pending = %#v", pending)
	}
}
