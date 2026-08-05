package subscription

import (
	"errors"
	"testing"
	"time"
)

func int32ptr(v int32) *int32 { return &v }

func TestEvaluatePurchaseEligibilityRollingBoundaryAndTightenedHistory(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	policy := PurchasePolicy{
		PeriodType:           PurchasePeriodRolling,
		PeriodMaxPurchases:   int32ptr(2),
		RollingWindowHours:   int32ptr(24),
		AllowAdvancePurchase: true,
	}
	facts := PurchaseFacts{
		Now: now,
		PaidOrderTimes: []time.Time{
			now.Add(-25 * time.Hour),
			now.Add(-23 * time.Hour),
			now.Add(-time.Hour),
			now.Add(-24 * time.Hour), // exact boundary no longer consumes capacity
		},
		MaxQueue: 2,
	}

	decision, err := EvaluatePurchaseEligibility(policy, facts)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.PrimaryReason != PurchaseRollingLimitReached {
		t.Fatalf("decision = %+v", decision)
	}
	wantRetry := now.Add(time.Hour)
	if decision.RetryAt == nil || !decision.RetryAt.Equal(wantRetry) {
		t.Fatalf("retry_at = %v, want %v", decision.RetryAt, wantRetry)
	}
}

func TestEvaluatePurchaseEligibilityCalendarMonthUsesCivilTime(t *testing.T) {
	loc, err := time.LoadLocation("Australia/Perth")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 2, 28, 23, 0, 0, 0, loc)
	policy := PurchasePolicy{
		PeriodType:           PurchasePeriodCalendar,
		PeriodMaxPurchases:   int32ptr(1),
		CalendarUnit:         CalendarUnitMonth,
		CalendarTimezone:     "Australia/Perth",
		AllowAdvancePurchase: true,
	}
	decision, err := EvaluatePurchaseEligibility(policy, PurchaseFacts{
		Now:            now,
		PaidOrderTimes: []time.Time{time.Date(2026, 2, 1, 0, 0, 0, 0, loc)},
		MaxQueue:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	if decision.RetryAt == nil || !decision.RetryAt.Equal(want) {
		t.Fatalf("retry_at = %v, want %v", decision.RetryAt, want)
	}
}

func TestEvaluatePurchaseEligibilityCombinesPermanentAndLiveRules(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	expires := now.Add(3 * time.Hour)
	policy := PurchasePolicy{
		PeriodType:           PurchasePeriodNone,
		LifetimeMaxPurchases: int32ptr(1),
		AllowAdvancePurchase: false,
	}
	decision, err := EvaluatePurchaseEligibility(policy, PurchaseFacts{
		Now:                     now,
		PaidOrderTimes:          []time.Time{now.Add(-time.Hour)},
		ActiveSamePlanExpiresAt: &expires,
		MaxQueue:                2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || len(decision.BlockingRules) != 2 {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.PrimaryReason != PurchaseAdvanceNotAllowed {
		t.Fatalf("primary = %s", decision.PrimaryReason)
	}
	if decision.RetryAt != nil {
		t.Fatalf("permanent lifetime blocker must clear retry_at: %v", decision.RetryAt)
	}
}

func TestValidatePurchasePolicyRejectsAmbiguousMonth(t *testing.T) {
	err := ValidatePurchasePolicy(PurchasePolicy{
		PeriodType:           PurchasePeriodRolling,
		PeriodMaxPurchases:   int32ptr(1),
		RollingWindowHours:   int32ptr(720),
		CalendarUnit:         CalendarUnitMonth,
		AllowAdvancePurchase: true,
	})
	if err == nil {
		t.Fatal("expected rolling/calendar field conflict")
	}
}

func TestValidatePurchasePolicyRejectsRollingDurationOverflow(t *testing.T) {
	policy := PurchasePolicy{
		PeriodType:           PurchasePeriodRolling,
		PeriodMaxPurchases:   int32ptr(1),
		RollingWindowHours:   int32ptr(MaxRollingWindowHours + 1),
		AllowAdvancePurchase: true,
	}
	if err := ValidatePurchasePolicy(policy); !errors.Is(err, ErrPurchasePolicyInvalid) {
		t.Fatalf("ValidatePurchasePolicy() error = %v, want ErrPurchasePolicyInvalid", err)
	}
}

func TestPurchaseDeniedErrorPreservesOpenOrderCompatibility(t *testing.T) {
	err := &PurchaseDeniedError{Decision: PurchaseDecision{
		Allowed:       false,
		PrimaryReason: PurchaseOrderProcessing,
		BlockingRules: []PurchaseRuleDecision{{Reason: PurchaseOrderProcessing}},
	}}
	if !errors.Is(err, ErrPlanAlreadyQueued) {
		t.Fatal("an open order must retain the legacy already-queued error contract")
	}
}

func TestCalendarPeriodBoundsRespectDSTAndMondayWeek(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, loc)
	dayStart, dayEnd, err := calendarPeriodBounds(now, CalendarUnitDay, loc.String())
	if err != nil {
		t.Fatal(err)
	}
	if got := dayEnd.Sub(dayStart); got != 23*time.Hour {
		t.Fatalf("DST transition day duration = %v, want 23h", got)
	}
	weekStart, weekEnd, err := calendarPeriodBounds(now, CalendarUnitWeek, loc.String())
	if err != nil {
		t.Fatal(err)
	}
	if weekStart.Weekday() != time.Monday || weekStart.Day() != 2 || weekEnd.Day() != 9 {
		t.Fatalf("week bounds = %v .. %v, want Monday Mar 2 .. Monday Mar 9", weekStart, weekEnd)
	}
}
