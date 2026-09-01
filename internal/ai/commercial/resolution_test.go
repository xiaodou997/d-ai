package commercial

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

func TestResolveAccessibleGroupsHonorsUserAndAPIKeyGroup(t *testing.T) {
	t.Parallel()

	userMultiplier := 2.2
	repo := &resolutionRepoStub{
		groups: []Group{
			{ID: "g-public", Name: "Public", DefaultUserMultiplier: 1.0, UserDefaultVisible: false, SortOrder: 10, Status: StatusActive},
			{ID: "g-private", Name: "Private", DefaultUserMultiplier: 1.1, UserDefaultVisible: false, SortOrder: 20, Status: StatusActive},
			{ID: "g-other", Name: "Other", DefaultUserMultiplier: 1.4, UserDefaultVisible: true, SortOrder: 30, Status: StatusActive},
		},
		userBindings: map[string][]UserGroupBinding{
			"tenant-1:user-1": {
				{TenantID: "tenant-1", UserID: "user-1", GroupID: "g-private", UserMultiplierOverride: &userMultiplier},
			},
		},
	}
	service := NewService(repo)

	groups, err := service.ResolveAccessibleGroups(context.Background(), identity.Subject{
		TenantID: "tenant-1",
		UserID:   "user-1",
		GroupID:  "g-private",
	})
	if err != nil {
		t.Fatalf("ResolveAccessibleGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Group.ID != "g-private" {
		t.Fatalf("group id = %q", groups[0].Group.ID)
	}
	if groups[0].EffectiveUserMultiplier != userMultiplier {
		t.Fatalf("effective multiplier = %v", groups[0].EffectiveUserMultiplier)
	}
	if !groups[0].UserBound || !groups[0].APIKeyBound {
		t.Fatalf("binding flags = user:%v apikey:%v", groups[0].UserBound, groups[0].APIKeyBound)
	}
}

// 强制分组只跳过普通请求的组排序；用户身份仍必须满足公开或显式授权。
func TestResolveForcedGroupRequiresUserVisibilityAndKeepsTenantAppAccess(t *testing.T) {
	t.Parallel()

	repo := &resolutionRepoStub{
		groups: []Group{
			{ID: "g-hidden", Name: "Hidden", DefaultUserMultiplier: 2.5, UserDefaultVisible: false, SortOrder: 10, Status: StatusActive},
			{ID: "g-bound", Name: "Bound", DefaultUserMultiplier: 1.8, UserDefaultVisible: false, SortOrder: 20, Status: StatusActive},
		},
	}
	service := NewService(repo)

	// 用户身份不能通过应用的 forced group 绕过专属分组授权。
	groups, err := service.ResolveAccessibleGroups(context.Background(), identity.Subject{
		TenantID:      "tenant-1",
		UserID:        "user-1",
		ForcedGroupID: "g-hidden",
	})
	if err != nil || len(groups) != 0 {
		t.Fatalf("forced hidden group = %#v, err=%v; want no access", groups, err)
	}

	// 租户拥有的应用没有终端用户授权关系，仍可使用租户自己的分组。
	groups, err = service.ResolveAccessibleGroups(context.Background(), identity.Subject{
		TenantID:      "tenant-1",
		ForcedGroupID: "g-hidden",
	})
	if err != nil || len(groups) != 1 || groups[0].Group.ID != "g-hidden" {
		t.Fatalf("tenant forced group = %#v, err=%v", groups, err)
	}

	// 用户绑定后可使用专属分组，并继承分组默认倍率。
	repo.userBindings = map[string][]UserGroupBinding{
		"tenant-1:user-1": {{TenantID: "tenant-1", UserID: "user-1", GroupID: "g-bound"}},
	}
	groups, err = service.ResolveAccessibleGroups(context.Background(), identity.Subject{
		TenantID:      "tenant-1",
		UserID:        "user-1",
		ForcedGroupID: "g-bound",
	})
	if err != nil {
		t.Fatalf("ResolveAccessibleGroups(forced bound): %v", err)
	}
	if err != nil || len(groups) != 1 || groups[0].Group.ID != "g-bound" {
		t.Fatalf("forced bound group = %#v, err=%v", groups, err)
	}
	if groups[0].EffectiveUserMultiplier != 1.8 {
		t.Fatalf("forced bound multiplier = %v, want group default 1.8", groups[0].EffectiveUserMultiplier)
	}

	// 停用分组对租户应用和用户调用都不可用。
	repo.groups[0].Status = StatusDisabled
	groups, err = service.ResolveAccessibleGroups(context.Background(), identity.Subject{
		TenantID:      "tenant-1",
		ForcedGroupID: "g-hidden",
	})
	if err != nil {
		t.Fatalf("ResolveAccessibleGroups(forced disabled): %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("disabled forced group should resolve to empty, got %#v", groups)
	}
}

func TestResolveDispatchAppliesRuleAndAllowedModels(t *testing.T) {
	t.Parallel()

	repo := &resolutionRepoStub{
		groups: []Group{
			{ID: "g1", Name: "Main", DefaultUserMultiplier: 1, AllowProtocolConversion: true, SortOrder: 10, Status: StatusActive},
		},
		dispatchRules: map[string][]DispatchRule{
			"g1": {
				{
					ID:            "r1",
					GroupID:       "g1",
					ClientSurface: surface.AnthropicMessages,
					MatchType:     DispatchMatchExact,
					MatchValue:    "opus-4.7",
					TargetModelID: "gpt-5.4",
					Status:        StatusActive,
					Priority:      1,
				},
			},
		},
		groupClientSurfaces: map[string][]GroupClientSurface{
			"g1": {
				{GroupID: "g1", Surface: surface.AnthropicMessages, BridgeEnabled: true, Status: StatusActive},
			},
		},
		groupTargets: map[string][]GroupTarget{
			"g1": {
				{ID: "t1", GroupID: "g1", TargetKind: TargetKindDirectUpstream, TargetID: "up-1", Status: StatusActive},
			},
		},
	}
	service := NewService(repo)

	options, err := service.ResolveDispatch(
		context.Background(),
		identity.Subject{
			TenantID:      "tenant-1",
			AllowedModels: []string{"gpt-5.4"},
		},
		catalog.CapabilityChat,
		surface.AnthropicMessages,
		"opus-4.7",
	)
	if err != nil {
		t.Fatalf("ResolveDispatch: %v", err)
	}
	if len(options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(options))
	}
	if options[0].ResolvedModelID != "gpt-5.4" {
		t.Fatalf("resolved model = %q", options[0].ResolvedModelID)
	}
	if options[0].MatchedRule == nil || options[0].MatchedRule.ID != "r1" {
		t.Fatalf("matched rule = %#v", options[0].MatchedRule)
	}
	if len(options[0].Targets) != 1 || options[0].Targets[0].TargetID != "up-1" {
		t.Fatalf("targets = %#v", options[0].Targets)
	}
	if !options[0].Group.Group.AllowProtocolConversion {
		t.Fatalf("allow protocol conversion should stay enabled when client surface bridge is enabled")
	}
}

func TestResolveDispatchHonorsGroupClientSurfaces(t *testing.T) {
	t.Parallel()

	repo := &resolutionRepoStub{
		groups: []Group{
			{ID: "g1", Name: "Main", DefaultUserMultiplier: 1, SortOrder: 10, Status: StatusActive},
		},
		groupClientSurfaces: map[string][]GroupClientSurface{
			"g1": {
				{GroupID: "g1", Surface: surface.AnthropicMessages, BridgeEnabled: false, Status: StatusActive},
			},
		},
		groupTargets: map[string][]GroupTarget{
			"g1": {
				{ID: "t1", GroupID: "g1", TargetKind: TargetKindDirectUpstream, TargetID: "up-1", Status: StatusActive},
			},
		},
	}
	service := NewService(repo)

	options, err := service.ResolveDispatch(
		context.Background(),
		identity.Subject{TenantID: "tenant-1"},
		catalog.CapabilityChat,
		surface.OpenAIChat,
		"opus-4.7",
	)
	if !errors.Is(err, ErrClientSurfaceNotAllowed) {
		t.Fatalf("ResolveDispatch() error = %v, want ErrClientSurfaceNotAllowed", err)
	}
	if options != nil {
		t.Fatalf("unsupported client surface options = %#v, want nil", options)
	}
}

func TestResolveDispatchUsesSelectedAPIKeyGroupSurface(t *testing.T) {
	t.Parallel()

	repo := &resolutionRepoStub{
		groups: []Group{
			{ID: "chat-denied", Name: "Images", DefaultUserMultiplier: 1, SortOrder: 10, Status: StatusActive},
			{ID: "chat-allowed", Name: "Chat", DefaultUserMultiplier: 1, SortOrder: 20, Status: StatusActive},
		},
		groupClientSurfaces: map[string][]GroupClientSurface{
			"chat-denied": {
				{GroupID: "chat-denied", Surface: surface.OpenAIImages, BridgeEnabled: true, Status: StatusActive},
			},
			"chat-allowed": {
				{GroupID: "chat-allowed", Surface: surface.OpenAIChat, BridgeEnabled: true, Status: StatusActive},
			},
		},
		groupTargets: map[string][]GroupTarget{
			"chat-denied": {
				{ID: "image-target", GroupID: "chat-denied", TargetKind: TargetKindDirectUpstream, TargetID: "up-image", Status: StatusActive},
			},
			"chat-allowed": {
				{ID: "chat-target", GroupID: "chat-allowed", TargetKind: TargetKindDirectUpstream, TargetID: "up-chat", Status: StatusActive},
			},
		},
	}
	service := NewService(repo)

	options, err := service.ResolveDispatch(
		context.Background(),
		identity.Subject{
			TenantID: "tenant-1",
			GroupID:  "chat-allowed",
		},
		catalog.CapabilityChat,
		surface.OpenAIChat,
		"gpt-5.4",
	)
	if err != nil {
		t.Fatalf("ResolveDispatch() error = %v", err)
	}
	if len(options) != 1 || options[0].Group.Group.ID != "chat-allowed" {
		t.Fatalf("dispatch options = %#v, want only chat-allowed", options)
	}
	if !options[0].Group.APIKeyBound {
		t.Fatalf("allowed group should retain API key binding metadata")
	}
	if repo.dispatchLoadCalls != 1 {
		t.Fatalf("dispatch configuration loads = %d, want one batch load", repo.dispatchLoadCalls)
	}
}

func TestGetGroupClientSurfacePolicyDefaultsToAll(t *testing.T) {
	t.Parallel()

	service := NewService(&resolutionRepoStub{})
	policy, err := service.GetGroupClientSurfacePolicy(context.Background(), TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"})
	if err != nil {
		t.Fatalf("GetGroupClientSurfacePolicy() error = %v", err)
	}
	if policy.Mode != GroupClientSurfacePolicyAll {
		t.Fatalf("policy mode = %q, want %q", policy.Mode, GroupClientSurfacePolicyAll)
	}
	if len(policy.AllowedSurfaces) != 0 {
		t.Fatalf("allowed surfaces = %#v, want empty", policy.AllowedSurfaces)
	}
}

func TestGetGroupClientSurfacePolicyRequiresExistingGroup(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("group not found")
	service := NewService(&resolutionRepoStub{getGroupErr: wantErr})
	if _, err := service.GetGroupClientSurfacePolicy(context.Background(), TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}); !errors.Is(err, wantErr) {
		t.Fatalf("GetGroupClientSurfacePolicy() error = %v, want %v", err, wantErr)
	}
}

func TestReplaceGroupClientSurfacePolicyRoundTripsModes(t *testing.T) {
	t.Parallel()

	repo := &resolutionRepoStub{
		groupClientSurfaces: map[string][]GroupClientSurface{
			"group-1": {
				{GroupID: "group-1", Surface: surface.OpenAIImages, BridgeEnabled: false, Status: StatusActive},
			},
		},
	}
	service := NewService(repo)
	policy, err := service.ReplaceGroupClientSurfacePolicy(context.Background(), TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}, GroupClientSurfacePolicyWrite{
		Mode:            GroupClientSurfacePolicyRestricted,
		AllowedSurfaces: []surface.ID{surface.OpenAIImages},
	})
	if err != nil {
		t.Fatalf("ReplaceGroupClientSurfacePolicy(restricted) error = %v", err)
	}
	if policy.Mode != GroupClientSurfacePolicyRestricted || len(policy.AllowedSurfaces) != 1 || policy.AllowedSurfaces[0] != surface.OpenAIImages {
		t.Fatalf("restricted policy = %#v", policy)
	}
	if !groupSurfaceBridgeEnabled(t, repo.groupClientSurfaces["group-1"], surface.OpenAIChat) {
		t.Fatal("a newly tracked surface should preserve the group-level protocol conversion behavior")
	}
	if groupSurfaceBridgeEnabled(t, repo.groupClientSurfaces["group-1"], surface.OpenAIImages) {
		t.Fatalf("restricted policy replacement should preserve the existing disabled bridge setting")
	}

	policy, err = service.ReplaceGroupClientSurfacePolicy(context.Background(), TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}, GroupClientSurfacePolicyWrite{
		Mode: GroupClientSurfacePolicyAll,
	})
	if err != nil {
		t.Fatalf("ReplaceGroupClientSurfacePolicy(all) error = %v", err)
	}
	if policy.Mode != GroupClientSurfacePolicyAll || len(policy.AllowedSurfaces) != 0 {
		t.Fatalf("all policy = %#v", policy)
	}
	if groupSurfaceBridgeEnabled(t, repo.groupClientSurfaces["group-1"], surface.OpenAIImages) {
		t.Fatalf("all policy replacement should preserve the existing disabled bridge setting")
	}
}

func TestReplaceGroupClientSurfacePolicyValidatesRestrictedSelection(t *testing.T) {
	t.Parallel()

	repo := &resolutionRepoStub{}
	service := NewService(repo)
	if _, err := service.ReplaceGroupClientSurfacePolicy(context.Background(), TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}, GroupClientSurfacePolicyWrite{
		Mode: GroupClientSurfacePolicyRestricted,
	}); err == nil {
		t.Fatalf("ReplaceGroupClientSurfacePolicy() error = nil, want empty restricted selection validation error")
	}

	policy, err := service.ReplaceGroupClientSurfacePolicy(context.Background(), TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}, GroupClientSurfacePolicyWrite{
		Mode:            GroupClientSurfacePolicyRestricted,
		AllowedSurfaces: []surface.ID{surface.OpenAIImages, surface.OpenAIImages},
	})
	if err != nil {
		t.Fatalf("ReplaceGroupClientSurfacePolicy() error = %v", err)
	}
	if len(policy.AllowedSurfaces) != 1 || policy.AllowedSurfaces[0] != surface.OpenAIImages {
		t.Fatalf("deduplicated policy = %#v, want one openai_images surface", policy)
	}
}

func groupSurfaceBridgeEnabled(t *testing.T, entries []GroupClientSurface, clientSurface surface.ID) bool {
	t.Helper()
	for _, entry := range entries {
		if entry.Surface == clientSurface {
			return entry.BridgeEnabled
		}
	}
	t.Fatalf("surface %q not found in %#v", clientSurface, entries)
	return false
}

type resolutionRepoStub struct {
	groups              []Group
	groupClientSurfaces map[string][]GroupClientSurface
	groupTargets        map[string][]GroupTarget
	dispatchRules       map[string][]DispatchRule
	userBindings        map[string][]UserGroupBinding
	getGroupErr         error
	dispatchLoadCalls   int
}

func (s *resolutionRepoStub) CreateGroup(context.Context, string, GroupWrite) (Group, error) {
	return Group{}, nil
}

func (s *resolutionRepoStub) ListGroups(context.Context, string) ([]Group, error) {
	return append([]Group(nil), s.groups...), nil
}

func (s *resolutionRepoStub) GetGroup(_ context.Context, scope TenantGroupScope) (Group, error) {
	if s.getGroupErr != nil {
		return Group{}, s.getGroupErr
	}
	for _, group := range s.groups {
		if group.ID == scope.GroupID {
			return group, nil
		}
	}
	return Group{ID: scope.GroupID}, nil
}

func (s *resolutionRepoStub) UpdateGroup(context.Context, TenantGroupScope, GroupWrite) (Group, error) {
	return Group{}, nil
}

func (s *resolutionRepoStub) UpdateGroupRoutePolicy(context.Context, TenantGroupScope, GroupRoutePolicyWrite) (Group, error) {
	return Group{}, nil
}

func (s *resolutionRepoStub) UpdateGroupStatus(context.Context, TenantGroupScope, Status) (Group, error) {
	return Group{}, nil
}

func (s *resolutionRepoStub) DeleteGroup(context.Context, TenantGroupScope) error {
	return nil
}

func (s *resolutionRepoStub) LoadDispatchData(_ context.Context, _ string, groupIDs []string) (DispatchData, error) {
	s.dispatchLoadCalls++
	out := DispatchData{
		ClientSurfaces: make(map[string][]GroupClientSurface, len(groupIDs)),
		Rules:          make(map[string][]DispatchRule, len(groupIDs)),
		Targets:        make(map[string][]GroupTarget, len(groupIDs)),
	}
	for _, groupID := range groupIDs {
		out.ClientSurfaces[groupID] = append([]GroupClientSurface(nil), s.groupClientSurfaces[groupID]...)
		out.Rules[groupID] = append([]DispatchRule(nil), s.dispatchRules[groupID]...)
		out.Targets[groupID] = append([]GroupTarget(nil), s.groupTargets[groupID]...)
	}
	return out, nil
}

func (s *resolutionRepoStub) ListGroupClientSurfaces(_ context.Context, scope TenantGroupScope) ([]GroupClientSurface, error) {
	return append([]GroupClientSurface(nil), s.groupClientSurfaces[scope.GroupID]...), nil
}

func (s *resolutionRepoStub) ReplaceGroupClientSurfaces(_ context.Context, scope TenantGroupScope, entries []GroupClientSurfaceWrite) error {
	if s.groupClientSurfaces == nil {
		s.groupClientSurfaces = make(map[string][]GroupClientSurface)
	}
	replaced := make([]GroupClientSurface, 0, len(entries))
	for _, entry := range entries {
		replaced = append(replaced, GroupClientSurface{
			GroupID:       scope.GroupID,
			Surface:       surface.ID(entry.Surface),
			BridgeEnabled: entry.BridgeEnabled,
			Status:        entry.Status,
		})
	}
	s.groupClientSurfaces[scope.GroupID] = replaced
	return nil
}

func (s *resolutionRepoStub) AddGroupTarget(context.Context, TenantGroupScope, GroupTargetWrite) (GroupTarget, error) {
	return GroupTarget{}, nil
}

func (s *resolutionRepoStub) ListGroupTargets(_ context.Context, scope TenantGroupScope) ([]GroupTarget, error) {
	return append([]GroupTarget(nil), s.groupTargets[scope.GroupID]...), nil
}

func (s *resolutionRepoStub) ListGroupTargetDetails(context.Context, TenantGroupScope) ([]GroupTargetDetail, error) {
	return nil, nil
}

func (s *resolutionRepoStub) ListGroupTargetsByTarget(context.Context, TargetKind, string) ([]GroupTargetDetail, error) {
	return nil, nil
}

func (s *resolutionRepoStub) GetGroupTargetDetail(context.Context, TenantGroupScope, string) (GroupTargetDetail, error) {
	return GroupTargetDetail{}, nil
}

func (s *resolutionRepoStub) UpdateGroupTarget(context.Context, TenantGroupScope, string, GroupTargetWrite) (GroupTarget, error) {
	return GroupTarget{}, nil
}

func (s *resolutionRepoStub) DeleteGroupTarget(context.Context, TenantGroupScope, string) error {
	return nil
}

func (s *resolutionRepoStub) ReplaceGroupTargets(context.Context, TenantGroupScope, GroupTargetBatchWrite) (GroupTargetBatchResult, error) {
	return GroupTargetBatchResult{}, nil
}

func (s *resolutionRepoStub) AddDispatchRule(context.Context, TenantGroupScope, DispatchRuleWrite) (DispatchRule, error) {
	return DispatchRule{}, nil
}

func (s *resolutionRepoStub) ListDispatchRules(_ context.Context, scope TenantGroupScope) ([]DispatchRule, error) {
	return append([]DispatchRule(nil), s.dispatchRules[scope.GroupID]...), nil
}

func (s *resolutionRepoStub) UpdateDispatchRule(context.Context, TenantGroupScope, string, DispatchRuleWrite) (DispatchRule, error) {
	return DispatchRule{}, nil
}

func (s *resolutionRepoStub) UpdateDispatchRuleStatus(context.Context, TenantGroupScope, string, Status) (DispatchRule, error) {
	return DispatchRule{}, nil
}

func (s *resolutionRepoStub) DeleteDispatchRule(context.Context, TenantGroupScope, string) error {
	return nil
}

func (s *resolutionRepoStub) PreviewDispatch(context.Context, TenantGroupScope, string, surface.ID) (DispatchPreview, error) {
	return DispatchPreview{}, nil
}

func (s *resolutionRepoStub) ListDispatchModels(context.Context, TenantGroupScope, surface.ID) ([]DispatchModel, error) {
	return nil, nil
}

func (s *resolutionRepoStub) UpsertUserBinding(_ context.Context, in UserGroupBindingWrite) (UserGroupBinding, error) {
	return UserGroupBinding{TenantID: in.TenantID, UserID: in.UserID, GroupID: in.GroupID}, nil
}

func (s *resolutionRepoStub) ListUserBindings(_ context.Context, tenantID, userID string) ([]UserGroupBinding, error) {
	return append([]UserGroupBinding(nil), s.userBindings[tenantID+":"+userID]...), nil
}

func (s *resolutionRepoStub) DeleteUserBinding(context.Context, string, string, string) error {
	return nil
}

func (s *resolutionRepoStub) CreateLimitPolicy(context.Context, LimitPolicyWrite) (LimitPolicy, error) {
	return LimitPolicy{}, nil
}

func (s *resolutionRepoStub) ListLimitPolicies(context.Context, LimitPolicyFilter) ([]LimitPolicy, error) {
	return nil, nil
}

func (s *resolutionRepoStub) UpdateLimitPolicy(context.Context, string, LimitPolicyWrite) (LimitPolicy, error) {
	return LimitPolicy{}, nil
}

func (s *resolutionRepoStub) UpdateLimitPolicyStatus(context.Context, string, Status) (LimitPolicy, error) {
	return LimitPolicy{}, nil
}

func (s *resolutionRepoStub) DeleteLimitPolicies(context.Context, LimitPolicyFilter) error {
	return nil
}
