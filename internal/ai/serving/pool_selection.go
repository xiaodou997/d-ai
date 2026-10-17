package serving

import (
	"context"
	"errors"

	"xiaodou/dai/internal/ai/domain"
)

// EligibleCredentialSelector applies the pool policy only to identities already
// admitted by the common availability, priority and protocol filters.
type EligibleCredentialSelector interface {
	SelectEligibleCredential(context.Context, string, string, []string) (string, error)
}

func poolTarget(c *domain.RouteCandidate) *domain.RouteCandidate {
	clone := *c
	clone.CredentialID = ""
	clone.CandidateID = domain.CandidateIdentity(c.RouteID, "pool:"+c.PoolID, c.EffectiveUpstreamModel(), c.OperationKey())
	return &clone
}

// Keep each pool target at one vote when comparing it with direct endpoints.
// The actual credential is selected after the group route policy has won.
func (s *ExecuteStep) poolSelectionTargets(tier []*domain.RouteCandidate) ([]*domain.RouteCandidate, map[string][]*domain.RouteCandidate) {
	members := map[string][]*domain.RouteCandidate{}
	if _, ok := s.OAuthPool.(EligibleCredentialSelector); !ok {
		return tier, members
	}
	targets := make([]*domain.RouteCandidate, 0, len(tier))
	for _, c := range tier {
		if !c.IsPoolRoute() || c.CredentialID == "" {
			targets = append(targets, c)
			continue
		}
		target := poolTarget(c)
		if len(members[target.Key()]) == 0 {
			targets = append(targets, target)
		}
		members[target.Key()] = append(members[target.Key()], c)
	}
	for _, target := range targets {
		if options := members[target.Key()]; len(options) > 0 {
			target.PoolCredentialCount = len(options)
		}
	}
	return targets, members
}

func (s *ExecuteStep) resolvePoolSelection(ctx context.Context, req *Request, selected *domain.RouteCandidate, members map[string][]*domain.RouteCandidate) *domain.RouteCandidate {
	options := members[selected.Key()]
	if len(options) == 0 {
		return selected
	}
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.CredentialID)
	}
	id, err := s.OAuthPool.(EligibleCredentialSelector).SelectEligibleCredential(ctx, selected.PoolID, selected.OAuthStrategy, ids)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			for _, option := range options {
				req.UsedCandidates[option.Key()] = true
			}
		} else {
			req.SelectionError = err
		}
		return nil
	}
	for _, option := range options {
		if option.CredentialID == id {
			return option
		}
	}
	req.SelectionError = errors.New("pool selector returned an ineligible credential")
	return nil
}
