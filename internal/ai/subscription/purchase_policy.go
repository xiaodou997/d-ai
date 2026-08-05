package subscription

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

const (
	PurchasePeriodNone     = "none"
	PurchasePeriodRolling  = "rolling"
	PurchasePeriodCalendar = "calendar"
	// MaxRollingWindowHours caps rolling windows at 100 years. Besides keeping
	// policies operationally meaningful, it prevents time.Duration overflow.
	MaxRollingWindowHours int32 = 100 * 365 * 24

	CalendarUnitDay   = "day"
	CalendarUnitWeek  = "week"
	CalendarUnitMonth = "month"
)

type PurchaseBlockReason string

const (
	PurchaseAllowed              PurchaseBlockReason = ""
	PurchaseOrderProcessing      PurchaseBlockReason = "purchase_order_processing"
	PurchasePlanAlreadyQueued    PurchaseBlockReason = "purchase_plan_already_queued"
	PurchaseQueueFull            PurchaseBlockReason = "subscription_queue_full"
	PurchaseAdvanceNotAllowed    PurchaseBlockReason = "advance_purchase_not_allowed"
	PurchaseLifetimeLimitReached PurchaseBlockReason = "purchase_lifetime_limit_reached"
	PurchaseRollingLimitReached  PurchaseBlockReason = "purchase_rolling_limit_reached"
	PurchaseCalendarLimitReached PurchaseBlockReason = "purchase_calendar_limit_reached"
)

var ErrPurchasePolicyInvalid = errors.New("subscription: purchase policy invalid")

// PurchasePolicy is a conjunction of an optional lifetime cap, one optional
// period cap, and the plan's advance-purchase rule. A zero-value policy is the
// backwards-compatible unlimited policy with advance purchase enabled after
// NormalizePurchasePolicy.
type PurchasePolicy struct {
	LifetimeMaxPurchases *int32    `json:"lifetime_max_purchases,omitempty"`
	PeriodType           string    `json:"period_type"`
	PeriodMaxPurchases   *int32    `json:"period_max_purchases,omitempty"`
	RollingWindowHours   *int32    `json:"rolling_window_hours,omitempty"`
	CalendarUnit         string    `json:"calendar_unit,omitempty"`
	CalendarTimezone     string    `json:"calendar_timezone,omitempty"`
	AllowAdvancePurchase bool      `json:"allow_advance_purchase"`
	Version              int64     `json:"version"`
	CreatedAt            time.Time `json:"-"`
	UpdatedAt            time.Time `json:"-"`
}

type PurchasePolicyRevision struct {
	PlanID    string
	Version   int64
	Policy    PurchasePolicy
	ChangedBy string
	ChangedAt time.Time
}

func DefaultPurchasePolicy() PurchasePolicy {
	return PurchasePolicy{
		PeriodType:           PurchasePeriodNone,
		AllowAdvancePurchase: true,
		Version:              1,
	}
}

// NormalizePurchasePolicy fills transport/storage defaults without weakening
// explicit restrictions. New plans should call it before validation.
func NormalizePurchasePolicy(p PurchasePolicy) PurchasePolicy {
	if p.PeriodType == "" && p.LifetimeMaxPurchases == nil && p.PeriodMaxPurchases == nil &&
		p.RollingWindowHours == nil && p.CalendarUnit == "" && p.CalendarTimezone == "" && p.Version == 0 {
		return DefaultPurchasePolicy()
	}
	if p.PeriodType == "" {
		p.PeriodType = PurchasePeriodNone
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	return p
}

func ValidatePurchasePolicy(p PurchasePolicy) error {
	p = NormalizePurchasePolicy(p)
	if p.LifetimeMaxPurchases != nil && *p.LifetimeMaxPurchases <= 0 {
		return fmt.Errorf("%w: lifetime_max_purchases must be positive", ErrPurchasePolicyInvalid)
	}
	switch p.PeriodType {
	case PurchasePeriodNone:
		if p.PeriodMaxPurchases != nil || p.RollingWindowHours != nil || p.CalendarUnit != "" || p.CalendarTimezone != "" {
			return fmt.Errorf("%w: period fields require a period type", ErrPurchasePolicyInvalid)
		}
	case PurchasePeriodRolling:
		if p.PeriodMaxPurchases == nil || *p.PeriodMaxPurchases <= 0 || p.RollingWindowHours == nil || *p.RollingWindowHours <= 0 {
			return fmt.Errorf("%w: rolling period requires positive max purchases and window hours", ErrPurchasePolicyInvalid)
		}
		if *p.RollingWindowHours > MaxRollingWindowHours {
			return fmt.Errorf("%w: rolling_window_hours must not exceed %d", ErrPurchasePolicyInvalid, MaxRollingWindowHours)
		}
		if p.CalendarUnit != "" || p.CalendarTimezone != "" {
			return fmt.Errorf("%w: rolling period cannot have calendar fields", ErrPurchasePolicyInvalid)
		}
	case PurchasePeriodCalendar:
		if p.PeriodMaxPurchases == nil || *p.PeriodMaxPurchases <= 0 || p.RollingWindowHours != nil {
			return fmt.Errorf("%w: calendar period requires positive max purchases", ErrPurchasePolicyInvalid)
		}
		switch p.CalendarUnit {
		case CalendarUnitDay, CalendarUnitWeek, CalendarUnitMonth:
		default:
			return fmt.Errorf("%w: calendar_unit must be day/week/month", ErrPurchasePolicyInvalid)
		}
		if p.CalendarTimezone == "" {
			return fmt.Errorf("%w: calendar_timezone is required", ErrPurchasePolicyInvalid)
		}
		if _, err := time.LoadLocation(p.CalendarTimezone); err != nil {
			return fmt.Errorf("%w: unknown calendar timezone", ErrPurchasePolicyInvalid)
		}
	default:
		return fmt.Errorf("%w: period_type must be none/rolling/calendar", ErrPurchasePolicyInvalid)
	}
	return nil
}

type PurchaseFacts struct {
	Now                     time.Time
	PaidOrderTimes          []time.Time
	OpenOrderCount          int
	OpenSamePlanOrder       bool
	LiveSubscriptionCount   int
	PendingSamePlan         bool
	ActiveSamePlanExpiresAt *time.Time
	MaxQueue                int
}

type PurchaseRuleDecision struct {
	Reason  PurchaseBlockReason
	RetryAt *time.Time
	Limit   *int32
	Used    int32
}

type PurchaseDecision struct {
	Allowed       bool
	PrimaryReason PurchaseBlockReason
	BlockingRules []PurchaseRuleDecision
	RetryAt       *time.Time
}

type PurchaseDeniedError struct {
	Decision PurchaseDecision
}

func (e *PurchaseDeniedError) Error() string {
	return "subscription: purchase denied: " + string(e.Decision.PrimaryReason)
}

func (e *PurchaseDeniedError) Is(target error) bool {
	for _, rule := range e.Decision.BlockingRules {
		switch {
		case target == ErrQueueFull && rule.Reason == PurchaseQueueFull:
			return true
		case target == ErrPlanAlreadyQueued &&
			(rule.Reason == PurchasePlanAlreadyQueued || rule.Reason == PurchaseOrderProcessing):
			return true
		}
	}
	return false
}

// EvaluatePurchaseEligibility is the single policy interface used by purchase
// reservation and customer storefront reads.
func EvaluatePurchaseEligibility(policy PurchasePolicy, facts PurchaseFacts) (PurchaseDecision, error) {
	policy = NormalizePurchasePolicy(policy)
	if err := ValidatePurchasePolicy(policy); err != nil {
		return PurchaseDecision{}, err
	}
	decision := PurchaseDecision{Allowed: true}
	add := func(rule PurchaseRuleDecision) {
		decision.Allowed = false
		decision.BlockingRules = append(decision.BlockingRules, rule)
		if decision.PrimaryReason == PurchaseAllowed {
			decision.PrimaryReason = rule.Reason
		}
		if rule.RetryAt != nil && (decision.RetryAt == nil || rule.RetryAt.After(*decision.RetryAt)) {
			retryAt := *rule.RetryAt
			decision.RetryAt = &retryAt
		}
	}

	if facts.OpenSamePlanOrder {
		add(PurchaseRuleDecision{Reason: PurchaseOrderProcessing})
	}
	if facts.PendingSamePlan {
		add(PurchaseRuleDecision{Reason: PurchasePlanAlreadyQueued})
	}
	if facts.MaxQueue > 0 && facts.LiveSubscriptionCount+facts.OpenOrderCount >= 1+facts.MaxQueue {
		add(PurchaseRuleDecision{Reason: PurchaseQueueFull})
	}
	if !policy.AllowAdvancePurchase && facts.ActiveSamePlanExpiresAt != nil {
		add(PurchaseRuleDecision{Reason: PurchaseAdvanceNotAllowed, RetryAt: facts.ActiveSamePlanExpiresAt})
	}
	if policy.LifetimeMaxPurchases != nil && len(facts.PaidOrderTimes) >= int(*policy.LifetimeMaxPurchases) {
		add(PurchaseRuleDecision{
			Reason: PurchaseLifetimeLimitReached,
			Limit:  policy.LifetimeMaxPurchases,
			Used:   int32(len(facts.PaidOrderTimes)),
		})
	}

	switch policy.PeriodType {
	case PurchasePeriodRolling:
		window := time.Duration(*policy.RollingWindowHours) * time.Hour
		threshold := facts.Now.Add(-window)
		recent := make([]time.Time, 0, len(facts.PaidOrderTimes))
		for _, purchasedAt := range facts.PaidOrderTimes {
			// At the exact boundary now >= purchasedAt+window, capacity is free.
			if purchasedAt.After(threshold) && !purchasedAt.After(facts.Now) {
				recent = append(recent, purchasedAt)
			}
		}
		if len(recent) >= int(*policy.PeriodMaxPurchases) {
			sort.Slice(recent, func(i, j int) bool { return recent[i].Before(recent[j]) })
			idx := len(recent) - int(*policy.PeriodMaxPurchases)
			retryAt := recent[idx].Add(window)
			add(PurchaseRuleDecision{
				Reason:  PurchaseRollingLimitReached,
				RetryAt: &retryAt,
				Limit:   policy.PeriodMaxPurchases,
				Used:    int32(len(recent)),
			})
		}
	case PurchasePeriodCalendar:
		start, end, err := calendarPeriodBounds(facts.Now, policy.CalendarUnit, policy.CalendarTimezone)
		if err != nil {
			return PurchaseDecision{}, err
		}
		var used int32
		for _, purchasedAt := range facts.PaidOrderTimes {
			if !purchasedAt.Before(start) && purchasedAt.Before(end) {
				used++
			}
		}
		if used >= *policy.PeriodMaxPurchases {
			add(PurchaseRuleDecision{
				Reason:  PurchaseCalendarLimitReached,
				RetryAt: &end,
				Limit:   policy.PeriodMaxPurchases,
				Used:    used,
			})
		}
	}

	// A permanent blocker must not advertise a misleading retry time.
	for _, rule := range decision.BlockingRules {
		if rule.Reason == PurchaseLifetimeLimitReached || rule.Reason == PurchasePlanAlreadyQueued || rule.Reason == PurchaseQueueFull || rule.Reason == PurchaseOrderProcessing {
			decision.RetryAt = nil
			break
		}
	}
	return decision, nil
}

func calendarPeriodBounds(now time.Time, unit, timezone string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: unknown calendar timezone", ErrPurchasePolicyInvalid)
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	switch unit {
	case CalendarUnitDay:
		return start, start.AddDate(0, 0, 1), nil
	case CalendarUnitWeek:
		daysSinceMonday := (int(start.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -daysSinceMonday)
		return start, start.AddDate(0, 0, 7), nil
	case CalendarUnitMonth:
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid calendar unit", ErrPurchasePolicyInvalid)
	}
}
