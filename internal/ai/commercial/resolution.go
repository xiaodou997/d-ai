package commercial

import (
	"context"
	"errors"
	"path"
	"regexp"
	"sort"
	"strings"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

var (
	ErrClientSurfaceNotAllowed = errors.New("client surface is not allowed by any accessible group")
	ErrNoAccessibleGroup       = errors.New("no accessible group")
)

// ResolveAccessibleGroups expands a tenant-owned group catalog for one runtime
// subject. End users see groups marked default-visible or explicitly bound to
// them; binding existence is the authorization and a multiplier override
// replaces the group default rather than multiplying it.
func (s *Service) ResolveAccessibleGroups(ctx context.Context, subject identity.Subject) ([]AccessibleGroup, error) {
	if strings.TrimSpace(subject.TenantID) == "" {
		return nil, newValidationError("tenant_id", "tenant_id is required")
	}

	if forced := strings.TrimSpace(subject.ForcedGroupID); forced != "" {
		return s.resolveForcedGroup(ctx, subject, forced)
	}

	groups, err := s.repo.ListGroups(ctx, subject.TenantID)
	if err != nil {
		return nil, err
	}
	sortGroups(groups)

	visible := make([]AccessibleGroup, 0, len(groups))
	for _, group := range groups {
		if group.Status != StatusActive {
			continue
		}
		visible = append(visible, AccessibleGroup{
			Group:                   group,
			EffectiveUserMultiplier: group.DefaultUserMultiplier,
			UserDefaultVisible:      group.UserDefaultVisible,
		})
	}

	if subject.UserID != "" {
		userBindings, err := s.repo.ListUserBindings(ctx, subject.TenantID, subject.UserID)
		if err != nil {
			return nil, err
		}
		userBindingsByGroup := make(map[string]UserGroupBinding, len(userBindings))
		for _, binding := range userBindings {
			userBindingsByGroup[binding.GroupID] = binding
		}
		userVisible := make([]AccessibleGroup, 0, len(visible))
		for _, item := range visible {
			userBinding, bound := userBindingsByGroup[item.Group.ID]
			if !item.UserDefaultVisible && !bound {
				continue
			}
			if bound {
				item.UserBound = true
				if userBinding.UserMultiplierOverride != nil {
					item.EffectiveUserMultiplier = *userBinding.UserMultiplierOverride
				}
			}
			userVisible = append(userVisible, item)
		}
		visible = userVisible
	}

	if subject.GroupID != "" {
		visibleByID := make(map[string]AccessibleGroup, len(visible))
		for _, item := range visible {
			visibleByID[item.Group.ID] = item
		}
		item, ok := visibleByID[subject.GroupID]
		if ok {
			item.APIKeyBound = true
			visible = []AccessibleGroup{item}
		} else {
			visible = []AccessibleGroup{}
		}
	}

	return visible, nil
}

// resolveForcedGroup is used by apps after their group ownership was checked at
// write time. Tenant-owned invocations may use any active group owned by the
// tenant. User invocations must still satisfy the group's current public or
// explicit-user authorization policy on every execution.
func (s *Service) resolveForcedGroup(ctx context.Context, subject identity.Subject, groupID string) ([]AccessibleGroup, error) {
	group, err := s.repo.GetGroup(ctx, TenantGroupScope{TenantID: subject.TenantID, GroupID: groupID})
	if err != nil {
		return nil, err
	}
	if group.Status != StatusActive {
		return []AccessibleGroup{}, nil
	}

	item := AccessibleGroup{
		Group:                   group,
		EffectiveUserMultiplier: group.DefaultUserMultiplier,
		UserDefaultVisible:      group.UserDefaultVisible,
	}

	if subject.UserID != "" {
		userBindings, err := s.repo.ListUserBindings(ctx, subject.TenantID, subject.UserID)
		if err != nil {
			return nil, err
		}
		for _, binding := range userBindings {
			if binding.GroupID != groupID {
				continue
			}
			item.UserBound = true
			if binding.UserMultiplierOverride != nil {
				item.EffectiveUserMultiplier = *binding.UserMultiplierOverride
			}
			break
		}
		if !item.UserDefaultVisible && !item.UserBound {
			return []AccessibleGroup{}, nil
		}
	}

	return []AccessibleGroup{item}, nil
}

// ResolveDispatch resolves the caller's effective groups and request-model
// dispatch rules into ordered commercial dispatch options.
func (s *Service) ResolveDispatch(
	ctx context.Context,
	subject identity.Subject,
	capability catalog.Capability,
	clientSurface surface.ID,
	requestedModel string,
) ([]DispatchResolution, error) {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return nil, newValidationError("requested_model", "requested_model is required")
	}
	if !surface.IsKnown(clientSurface) {
		return nil, newValidationError("client_surface", "unsupported client_surface")
	}

	groups, err := s.ResolveAccessibleGroups(ctx, subject)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, ErrNoAccessibleGroup
	}
	groupIDs := make([]string, 0, len(groups))
	for _, item := range groups {
		groupIDs = append(groupIDs, item.Group.ID)
	}
	dispatchData, err := s.repo.LoadDispatchData(ctx, subject.TenantID, groupIDs)
	if err != nil {
		return nil, err
	}

	out := make([]DispatchResolution, 0, len(groups))
	acceptedSurface := false
	for _, item := range groups {
		clientSurfaces := dispatchData.ClientSurfaces[item.Group.ID]
		accepted, allowProtocolConversion := resolveGroupClientSurfacePolicy(clientSurfaces, clientSurface, item.Group.AllowProtocolConversion)
		if !accepted {
			continue
		}
		acceptedSurface = true
		item.Group.AllowProtocolConversion = allowProtocolConversion

		rules := dispatchData.Rules[item.Group.ID]
		sortDispatchRules(rules)
		matchedRule := matchDispatchRule(rules, clientSurface, requestedModel)

		resolvedModelID := requestedModel
		if matchedRule != nil {
			resolvedModelID = matchedRule.TargetModelID
		}
		if len(subject.AllowedModels) > 0 && !containsString(subject.AllowedModels, resolvedModelID) {
			continue
		}

		targets := dispatchData.Targets[item.Group.ID]
		targets = filterAndSortActiveTargets(targets)

		out = append(out, DispatchResolution{
			Group:           item,
			RequestedModel:  requestedModel,
			ResolvedModelID: resolvedModelID,
			MatchedRule:     matchedRule,
			Targets:         targets,
		})
	}
	if len(groups) > 0 && !acceptedSurface {
		return nil, ErrClientSurfaceNotAllowed
	}

	return out, nil
}

func resolveGroupClientSurfacePolicy(entries []GroupClientSurface, clientSurface surface.ID, allowProtocolConversion bool) (bool, bool) {
	if len(entries) == 0 {
		return true, allowProtocolConversion
	}
	for _, item := range entries {
		if item.Status != StatusActive {
			continue
		}
		if item.Surface != clientSurface {
			continue
		}
		return true, allowProtocolConversion && item.BridgeEnabled
	}
	return false, false
}

func sortGroups(groups []Group) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].SortOrder != groups[j].SortOrder {
			return groups[i].SortOrder < groups[j].SortOrder
		}
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].ID < groups[j].ID
	})
}

func sortDispatchRules(rules []DispatchRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		if !rules[i].CreatedAt.Equal(rules[j].CreatedAt) {
			return rules[i].CreatedAt.Before(rules[j].CreatedAt)
		}
		return rules[i].ID < rules[j].ID
	})
}

func matchDispatchRule(rules []DispatchRule, clientSurface surface.ID, requestedModel string) *DispatchRule {
	for _, rule := range rules {
		if rule.Status != StatusActive || rule.ClientSurface != clientSurface {
			continue
		}
		if dispatchRuleMatches(rule, requestedModel) {
			copy := rule
			return &copy
		}
	}
	return nil
}

func dispatchRuleMatches(rule DispatchRule, requestedModel string) bool {
	pattern := strings.TrimSpace(rule.MatchValue)
	switch rule.MatchType {
	case DispatchMatchExact:
		return pattern == requestedModel
	case DispatchMatchPrefix:
		return strings.HasPrefix(requestedModel, pattern)
	case DispatchMatchWildcard:
		ok, err := path.Match(pattern, requestedModel)
		return err == nil && ok
	case DispatchMatchRegex:
		re, err := regexp.Compile(pattern)
		return err == nil && re.MatchString(requestedModel)
	default:
		return false
	}
}

func filterAndSortActiveTargets(targets []GroupTarget) []GroupTarget {
	out := make([]GroupTarget, 0, len(targets))
	for _, target := range targets {
		if target.Status == StatusActive {
			out = append(out, target)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
