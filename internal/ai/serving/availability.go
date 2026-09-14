package serving

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

type ExcludingCredentialSelector interface {
	SelectCredentialExcluding(context.Context, string, string, []string) (*domain.OAuthCredential, error)
}

func candidateScopes(c *domain.RouteCandidate, cred *domain.OAuthCredential) []routing.FaultScope {
	return routing.CandidateScopes(c, cred)
}

type CredentialCandidateLister interface {
	ListCredentialCandidates(context.Context, string) ([]string, int64, error)
}

// Expand credential identities without decrypting secrets or occupying leases.
// Their pricing remains the immutable group/pool snapshot prepared earlier.
func (s *ExecuteStep) expandCredentialCandidates(ctx context.Context, req *Request) error {
	lister, ok := s.OAuthPool.(CredentialCandidateLister)
	if !ok {
		return nil
	}
	out := []*domain.RouteCandidate{}
	byPool := map[string][]string{}
	for _, c := range req.Candidates {
		if !c.IsPoolRoute() || c.CredentialID != "" {
			out = append(out, c)
			continue
		}
		ids, loaded := byPool[c.PoolID]
		if !loaded {
			var at int64
			var err error
			ids, at, err = lister.ListCredentialCandidates(ctx, c.PoolID)
			if err != nil {
				return err
			}
			byPool[c.PoolID] = ids
			if at > 0 && (req.AvailabilityRetryAt == 0 || at < req.AvailabilityRetryAt) {
				req.AvailabilityRetryAt = at
			}
		}
		if len(ids) == 0 {
			req.recordSkippedCandidate(c, "no_credential")
		}
		for _, id := range ids {
			clone := *c
			clone.CredentialID = id
			clone.CandidateID = domain.CandidateIdentity(c.Key(), id, c.EffectiveUpstreamModel(), c.OperationKey())
			out = append(out, &clone)
			if req.BillingSnapshots != nil {
				if snapshot, exists := req.BillingSnapshots[c.Key()]; exists {
					req.BillingSnapshots[clone.Key()] = snapshot
				}
			}
		}
	}
	req.Candidates = out
	return nil
}

func credentialUseKey(c *domain.RouteCandidate, id string) string {
	return id + "\x00" + c.EffectiveUpstreamModel() + "\x00" + c.OperationKey()
}
func credentialUsed(req *Request, c *domain.RouteCandidate, id string) bool {
	return req.UsedCredentials[id] || req.UsedCredentials[credentialUseKey(c, id)]
}

func scopeOfKind(scopes []routing.FaultScope, kind string) string {
	for _, scope := range scopes {
		if scope.Kind == kind {
			return scope.Key
		}
	}
	return ""
}

func (s *ExecuteStep) candidateAvailability(ctx context.Context, req *Request, c *domain.RouteCandidate, cred *domain.OAuthCredential) (bool, bool, error) {
	if s.Availability == nil {
		return true, false, nil
	}
	states, err := s.Availability.Read(ctx, candidateScopes(c, cred))
	if err != nil {
		return false, false, err
	}
	recovering := false
	var blockedUntil int64
	for _, state := range states {
		if state.Phase == routing.Cooling || state.Busy {
			at := state.RetryAt
			if state.Busy {
				at = state.LeaseUntil
			}
			blockedUntil = max(blockedUntil, at)
		}
		if state.Phase == routing.Recovering {
			recovering = true
		}
	}
	if blockedUntil > 0 {
		if req.AvailabilityRetryAt == 0 || blockedUntil < req.AvailabilityRetryAt {
			req.AvailabilityRetryAt = blockedUntil
		}
		return false, false, nil
	}

	return true, recovering, nil
}

// prepareEligible separates physical availability from tenant preference. It
// never claims a recovery slot and never calls a model endpoint.
func (s *ExecuteStep) prepareEligible(ctx context.Context, req *Request) ([]*domain.RouteCandidate, map[string]bool, error) {
	eligible := make([]*domain.RouteCandidate, 0, len(req.Candidates))
	recovery := map[string]bool{}
	if req.PreparedCredentials == nil {
		req.PreparedCredentials = map[string]*domain.OAuthCredential{}
	}
	if req.UsedCredentials == nil {
		req.UsedCredentials = map[string]bool{}
	}
	for _, c := range req.Candidates {
		if c == nil || req.UsedCandidates[c.Key()] {
			continue
		}
		if capacity, ok := s.UpstreamLimiter.(interface {
			Available(context.Context, string, int) (bool, error)
		}); ok && c.UpstreamConcurrencyLimit != nil {
			ready, err := capacity.Available(ctx, c.EffectiveAccountID(), *c.UpstreamConcurrencyLimit)
			if err != nil {
				return nil, nil, err
			}
			if !ready {
				req.recordSkippedCandidate(c, "upstream_capacity_exhausted")
				req.UsedCandidates[c.Key()] = true
				continue
			}
		}
		var cred *domain.OAuthCredential
		if c.CredentialID != "" {
			if credentialUsed(req, c, c.CredentialID) {
				continue
			}
			cred = &domain.OAuthCredential{ID: c.CredentialID}
		} else if c.IsPoolRoute() && s.OAuthPool != nil {
			cred = req.PreparedCredentials[c.Key()]
			if cred != nil && credentialUsed(req, c, cred.ID) {
				cred = nil
			}
			excluded := []string{}
			suffix := "\x00" + c.EffectiveUpstreamModel() + "\x00" + c.OperationKey()
			for id := range req.UsedCredentials {
				if !strings.Contains(id, "\x00") {
					excluded = append(excluded, id)
				} else if strings.HasSuffix(id, suffix) {
					excluded = append(excluded, strings.TrimSuffix(id, suffix))
				}
			}
			if cred == nil {
				var err error
				if picker, ok := s.OAuthPool.(ExcludingCredentialSelector); ok {
					cred, err = picker.SelectCredentialExcluding(ctx, c.PoolID, c.OAuthStrategy, excluded)
				} else {
					cred, err = s.selectPoolCredential(ctx, req, c)
				}
				if err != nil {
					cred = nil
				}
			}
			if cred == nil {
				req.recordSkippedCandidate(c, "no_credential")
				req.UsedCandidates[c.Key()] = true
				continue
			}
			req.PreparedCredentials[c.Key()] = cred
		}
		ok, trial, err := s.candidateAvailability(ctx, req, c, cred)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			req.recordSkippedCandidate(c, "cooling")
			req.UsedCandidates[c.Key()] = true
			continue
		}
		eligible = append(eligible, c)
		recovery[c.Key()] = trial
	}
	return eligible, recovery, nil
}

func (s *ExecuteStep) acquireAvailability(ctx context.Context, req *Request, c *domain.RouteCandidate) error {
	req.AvailabilityPermit = nil
	req.AvailabilityResult = routing.AvailabilityOutcome{}
	if s.Availability == nil {
		return nil
	}
	p, err := s.Availability.Acquire(ctx, candidateScopes(c, req.SelectedCredential), candidateProbeLease(c))
	if err != nil {
		var denied *routing.AdmissionDenied
		if errors.As(err, &denied) && (req.AvailabilityRetryAt == 0 || denied.RetryAt < req.AvailabilityRetryAt) {
			req.AvailabilityRetryAt = denied.RetryAt
		}
		return err
	}
	req.AvailabilityPermit = p
	return nil
}

func (s *ExecuteStep) finishAvailability(req *Request) {
	if s.Availability == nil || req.AvailabilityPermit == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	states, err := s.Availability.Complete(ctx, req.AvailabilityPermit, req.AvailabilityResult)
	if err != nil {
		zap.L().Error("availability completion failed", requestLogFields(req, zap.Error(err))...)
	}
	if len(req.Attempts) > 0 {
		a := &req.Attempts[len(req.Attempts)-1]
		for _, state := range states {
			if state.Phase == routing.Cooling && state.Epoch > req.AvailabilityPermit.Epochs[state.Scope.Key] {
				a.CooldownEntered = true
				a.RetryAt = state.RetryAt
			}
		}
	}
	req.AvailabilityPermit = nil
}

func availabilityVerdict(req *Request, c *domain.RouteCandidate, out Outcome) routing.AvailabilityOutcome {
	scopes := candidateScopes(c, req.SelectedCredential)
	v := routing.AvailabilityOutcome{Success: out.Status == ResultSuccess, CooldownUntil: out.RetryAt}
	switch out.Status {
	case ResultNetwork:
		v.FailureScope = scopeOfKind(scopes, "transport")
		v.HealthFailure = true
	case ResultServerError, ResultTimeout:
		v.FailureScope = scopeOfKind(scopes, "model")
		v.HealthFailure = true
		var dns *net.DNSError
		var op *net.OpError
		if errors.As(out.Err, &dns) || errors.As(out.Err, &op) && op.Op == "dial" {
			v.FailureScope = scopeOfKind(scopes, "transport")
		}
	case ResultUnauthorized:
		v.FailureScope = scopeOfKind(scopes, "authentication")
		v.Authentication = true
	case ResultRateLimited:
		v.FailureScope = scopeOfKind(scopes, "rate_limit")
		v.CooldownUntil = out.RetryAt
	case ResultModelError:
		v.FailureScope = scopeOfKind(scopes, "model")
	}
	v.Reason = out.Status.String()
	return v
}

func retryAfter(headers http.Header) int64 {
	v := strings.TrimSpace(headers.Get("Retry-After"))
	if n, err := strconv.ParseInt(v, 10, 32); err == nil && n >= 0 {
		return time.Now().Add(time.Duration(n) * time.Second).UnixMilli()
	}
	if t, err := http.ParseTime(v); err == nil && t.After(time.Now()) {
		return t.UnixMilli()
	}
	return 0
}

func setAvailabilityRetryAfter(req *Request) {
	if req.Envelope == nil || req.Envelope.W == nil || req.AvailabilityRetryAt <= 0 {
		return
	}
	seconds := (req.AvailabilityRetryAt - time.Now().UnixMilli() + 999) / 1000
	if seconds < 1 {
		seconds = 1
	}
	req.Envelope.W.Header().Set("Retry-After", fmt.Sprint(seconds))
}

type AttemptRecorder interface {
	RecordAttempt(context.Context, *Request, int) error
}

func (s *ExecuteStep) recordCompletedAttempt(ctx context.Context, req *Request, c *domain.RouteCandidate, index int) {
	a := &req.Attempts[index]
	a.OutcomeFinal = true
	if a.Outcome == ResultSuccess && req.RequestStatus != domain.RequestSuccess {
		a.Outcome = ResultRejected
		a.AvailabilityOutcome = a.Outcome.String()
	}
	if req.IsStream {
		a.FirstOutputMs = req.FirstTokenMs
	}
	if (req.ClientDeliveryState == domain.ClientDeliveryDisconnected || req.ClientDeliveryState == domain.ClientDeliveryWriteFailed) && a.Outcome == ResultSuccess {
		a.Outcome = ResultCanceled
		a.AvailabilityOutcome = ResultCanceled.String()
	}
	if a.AvailabilityOutcome == "" {
		a.AvailabilityOutcome = a.Outcome.String()
	}
	success := a.Outcome == ResultSuccess && req.RequestStatus == domain.RequestSuccess
	failure := a.Outcome == ResultNetwork || a.Outcome == ResultTimeout || a.Outcome == ResultServerError || a.Outcome == ResultUnauthorized || a.Outcome == ResultRateLimited || a.Outcome == ResultModelError
	if stats, ok := s.Stats.(routing.OutcomeStats); ok {
		stats.Observe(context.WithoutCancel(ctx), candidateStatsKey(c, req.IsStream), success, failure, a.FirstOutputMs, a.TotalMs)
	}
	if recorder, ok := s.CompletionRecorder.(AttemptRecorder); ok {
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := recorder.RecordAttempt(recordCtx, req, index); err != nil {
			zap.L().Error("upstream attempt record failed; request seal will retry", requestLogFields(req, zap.Error(err))...)
		}
	}
}
