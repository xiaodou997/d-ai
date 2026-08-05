package serving

import "xiaodou/dai/internal/ai/domain"

// SubscriptionDebitMicro meters an admitted subscription request from the
// immutable plan snapshot. User multipliers never affect package quota usage.
func SubscriptionDebitMicro(req *Request) (micro int64, ok bool) {
	base := req.BillingResult.RetailBaseMicro
	if base <= 0 || req.Candidate == nil {
		return 0, false
	}
	weight, present := req.SubscriptionGroupQuotaDebitMultipliers[req.Candidate.GroupID]
	if !present || weight <= 0 {
		return 0, false
	}
	return domain.ScaleInt64HalfUp(base, weight), true
}
