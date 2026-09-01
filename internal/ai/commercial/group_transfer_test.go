package commercial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/core/surface"
)

type groupTransferRepoStub struct {
	snapshots   []GroupConfigurationSnapshot
	environment GroupImportEnvironment
	apply       func(PlannedGroupImport) (AppliedGroupImport, error)
	applied     []PlannedGroupImport
	groupNames  []string
}

func (s *groupTransferRepoStub) SnapshotGroupConfigurations(context.Context, string, []string) ([]GroupConfigurationSnapshot, error) {
	return s.snapshots, nil
}

func (s *groupTransferRepoStub) LoadGroupImportEnvironment(_ context.Context, _ string, groupNames, _ []string) (GroupImportEnvironment, error) {
	s.groupNames = append([]string{}, groupNames...)
	return s.environment, nil
}

func TestGroupTransferPreviewDefaultsConflictsAndDisablesNewGroups(t *testing.T) {
	repo := &groupTransferRepoStub{environment: GroupImportEnvironment{
		ExistingByName: map[string]GroupConfigurationSnapshot{
			"现有组": {
				GroupID:       "existing-1",
				PriceBookID:   "price-current",
				ActiveTargets: 1,
				Configuration: GroupTransferGroup{Name: "现有组", Status: StatusActive},
			},
		},
		PriceBooks: map[string]GroupImportPriceBook{
			"price-new": {
				ID:     "price-new",
				Status: StatusActive,
				Models: map[string][]string{"gpt-5.5": {"chat"}},
			},
		},
	}}
	svc := NewGroupTransferService(repo, GroupTransferOptions{})
	bundle := GroupTransferBundle{
		SchemaVersion: GroupTransferSchemaVersion,
		BundleID:      "bundle-preview",
		ExportedAt:    "2026-07-14T08:30:00Z",
		Groups: []GroupTransferGroup{
			{Name: "现有组", DefaultUserMultiplier: 1, Status: StatusActive, ClientSurfacePolicy: GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll}},
			{
				Name:                  "新分组",
				DefaultUserMultiplier: 1,
				Status:                StatusActive,
				ClientSurfacePolicy:   GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
				DispatchRules: []GroupTransferDispatchRule{{
					ClientSurface: surface.OpenAIResponses,
					MatchType:     DispatchMatchExact,
					MatchValue:    "gpt-latest",
					TargetModelID: "gpt-5.5",
					Priority:      10,
					Status:        StatusActive,
				}},
			},
		},
	}

	preview, err := svc.Preview(context.Background(), "tenant-1", GroupImportRequest{
		Bundle: bundle,
		Choices: []GroupImportChoice{{
			SourceName:  "新分组",
			Action:      GroupImportActionCreate,
			PriceBookID: "price-new",
		}},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(preview.Items))
	}
	existing := preview.Items[0]
	if existing.Action != GroupImportActionSkip || existing.PriceBookID != "price-current" || existing.TargetGroupID != "existing-1" {
		t.Fatalf("unexpected existing plan: %+v", existing)
	}
	created := preview.Items[1]
	if created.Action != GroupImportActionCreate || created.PriceBookID != "price-new" || created.AppliedStatus != StatusDisabled {
		t.Fatalf("unexpected create plan: %+v", created)
	}
	if preview.Summary.Create != 1 || preview.Summary.Skip != 1 || preview.Summary.Error != 0 {
		t.Fatalf("unexpected summary: %+v", preview.Summary)
	}
}

func TestGroupTransferPreviewKeepsExistingPriceBookAndDisablesMissingModels(t *testing.T) {
	repo := &groupTransferRepoStub{environment: GroupImportEnvironment{
		ExistingByName: map[string]GroupConfigurationSnapshot{
			"现有组": {
				GroupID:       "existing-1",
				PriceBookID:   "price-current",
				ActiveTargets: 1,
				Configuration: GroupTransferGroup{Name: "现有组", Status: StatusActive},
			},
		},
		PriceBooks: map[string]GroupImportPriceBook{
			"price-current": {ID: "price-current", Status: StatusActive, Models: map[string][]string{"gpt-4o": {"chat"}, "embed-model": {"embedding"}}},
		},
	}}
	svc := NewGroupTransferService(repo, GroupTransferOptions{})
	bundle := GroupTransferBundle{
		SchemaVersion: GroupTransferSchemaVersion,
		BundleID:      "bundle-missing-model",
		ExportedAt:    "2026-07-14T08:30:00Z",
		Groups: []GroupTransferGroup{{
			Name:                  "现有组",
			DefaultUserMultiplier: 1,
			Status:                StatusActive,
			ClientSurfacePolicy:   GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
			DispatchRules: []GroupTransferDispatchRule{{
				ClientSurface: surface.OpenAIResponses,
				MatchType:     DispatchMatchExact,
				MatchValue:    "gpt-latest",
				TargetModelID: "gpt-5.5",
				Priority:      10,
				Status:        StatusActive,
			}, {
				ClientSurface: surface.OpenAIResponses,
				MatchType:     DispatchMatchExact,
				MatchValue:    "wrong-capability",
				TargetModelID: "embed-model",
				Priority:      20,
				Status:        StatusActive,
			}},
		}},
	}

	preview, err := svc.Preview(context.Background(), "tenant-1", GroupImportRequest{
		Bundle:  bundle,
		Choices: []GroupImportChoice{{SourceName: "现有组", Action: GroupImportActionUpdate}},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	item := preview.Items[0]
	if item.PriceBookID != "price-current" {
		t.Fatalf("PriceBookID = %q, want current price book", item.PriceBookID)
	}
	if item.AppliedStatus != StatusDisabled {
		t.Fatalf("AppliedStatus = %q, want disabled", item.AppliedStatus)
	}
	if len(item.MissingModels) != 1 || item.MissingModels[0] != "gpt-5.5" {
		t.Fatalf("MissingModels = %#v, want gpt-5.5", item.MissingModels)
	}
	if len(item.Warnings) < 2 || !strings.Contains(strings.Join(item.Warnings, " "), "能力") {
		t.Fatalf("Warnings = %#v, want model and capability warnings", item.Warnings)
	}
}

func TestGroupTransferPreviewRejectsInvalidAndDuplicateDispatchRules(t *testing.T) {
	repo := &groupTransferRepoStub{environment: GroupImportEnvironment{
		ExistingByName: map[string]GroupConfigurationSnapshot{},
		PriceBooks: map[string]GroupImportPriceBook{
			"price-1": {ID: "price-1", Status: StatusActive, Models: map[string][]string{"gpt-5.5": {"chat"}}},
		},
	}}
	svc := NewGroupTransferService(repo, GroupTransferOptions{})
	bundle := GroupTransferBundle{
		SchemaVersion: GroupTransferSchemaVersion,
		BundleID:      "bundle-invalid-rules",
		ExportedAt:    "2026-07-14T08:30:00Z",
		Groups: []GroupTransferGroup{{
			Name:                  "规则异常组",
			DefaultUserMultiplier: 1,
			Status:                StatusActive,
			ClientSurfacePolicy:   GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
			DispatchRules: []GroupTransferDispatchRule{
				{ClientSurface: surface.OpenAIResponses, MatchType: DispatchMatchExact, MatchValue: "gpt-latest", TargetModelID: "gpt-5.5", Priority: 10, Status: StatusActive},
				{ClientSurface: surface.OpenAIResponses, MatchType: DispatchMatchExact, MatchValue: "gpt-latest", TargetModelID: "gpt-5.5", Priority: 20, Status: StatusActive},
				{ClientSurface: surface.OpenAIResponses, MatchType: DispatchMatchRegex, MatchValue: "[", TargetModelID: "gpt-5.5", Priority: 30, Status: StatusActive},
			},
		}},
	}

	preview, err := svc.Preview(context.Background(), "tenant-1", GroupImportRequest{
		Bundle:  bundle,
		Choices: []GroupImportChoice{{SourceName: "规则异常组", Action: GroupImportActionCreate, PriceBookID: "price-1"}},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	item := preview.Items[0]
	if len(item.Errors) < 2 {
		t.Fatalf("Errors = %#v, want duplicate and regex errors", item.Errors)
	}
	joined := strings.Join(item.Errors, " ")
	if !strings.Contains(joined, "重复") || !strings.Contains(joined, "正则") {
		t.Fatalf("Errors = %#v, want duplicate and regex messages", item.Errors)
	}
	if preview.Summary.Error != 1 || preview.Summary.Create != 0 {
		t.Fatalf("unexpected summary: %+v", preview.Summary)
	}
}

func TestValidateTransferGroupRejectsInvalidRoutePolicy(t *testing.T) {
	errors := validateTransferGroup(GroupTransferGroup{
		Name:                  "bad-policy",
		DefaultUserMultiplier: 1,
		RoutePolicy:           RoutePolicyCost,
		Status:                StatusActive,
		ClientSurfacePolicy:   GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
	})
	if len(errors) != 0 {
		t.Fatalf("valid route policy rejected, errors = %#v", errors)
	}
	invalid := validateTransferGroup(GroupTransferGroup{
		Name:                  "bad-policy",
		DefaultUserMultiplier: 1,
		RoutePolicy:           RoutePolicy("invalid"),
		Status:                StatusActive,
		ClientSurfacePolicy:   GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
	})
	if len(invalid) == 0 {
		t.Fatal("invalid route policy was accepted")
	}
}

func (s *groupTransferRepoStub) ApplyGroupImport(_ context.Context, _ string, item PlannedGroupImport) (AppliedGroupImport, error) {
	s.applied = append(s.applied, item)
	if s.apply != nil {
		return s.apply(item)
	}
	return AppliedGroupImport{GroupID: "applied-" + item.Source.Name, Status: item.Preview.AppliedStatus}, nil
}

func TestGroupImportPriceBookSupportsMultipleCapabilities(t *testing.T) {
	priceBook := GroupImportPriceBook{Models: map[string][]string{
		"shared-model": {"embedding", "chat"},
	}}
	if exists, compatible := priceBook.SupportsModel("shared-model", surface.OpenAIResponses); !exists || !compatible {
		t.Fatalf("chat capability = exists %t compatible %t, want true/true", exists, compatible)
	}
	if exists, compatible := priceBook.SupportsModel("shared-model", surface.OpenAIImages); !exists || compatible {
		t.Fatalf("image capability = exists %t compatible %t, want true/false", exists, compatible)
	}
}

func TestGroupTransferExportOmitsEnvironmentReferences(t *testing.T) {
	fixedTime := time.Date(2026, time.July, 14, 8, 30, 0, 0, time.UTC)
	repo := &groupTransferRepoStub{snapshots: []GroupConfigurationSnapshot{{
		GroupID:       "group-1",
		PriceBookID:   "price-book-secret",
		ActiveTargets: 2,
		Configuration: GroupTransferGroup{
			Name:                    "高级组",
			Description:             "高优先级模型",
			DefaultUserMultiplier:   1.25,
			AllowProtocolConversion: true,
			SortOrder:               20,
			Status:                  StatusActive,
			ClientSurfacePolicy: GroupTransferClientSurfacePolicy{
				Mode:            GroupClientSurfacePolicyRestricted,
				AllowedSurfaces: []surface.ID{surface.OpenAIResponses},
			},
			DispatchRules: []GroupTransferDispatchRule{{
				ClientSurface: surface.OpenAIResponses,
				MatchType:     DispatchMatchExact,
				MatchValue:    "gpt-latest",
				TargetModelID: "gpt-5.5",
				Priority:      10,
				Status:        StatusActive,
				Notes:         "主入口",
			}},
		},
	}}}
	svc := NewGroupTransferService(repo, GroupTransferOptions{
		Now:   func() time.Time { return fixedTime },
		NewID: func() string { return "bundle-1" },
	})

	bundle, err := svc.Export(context.Background(), "tenant-1", []string{"group-1"})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if bundle.SchemaVersion != GroupTransferSchemaVersion || bundle.BundleID != "bundle-1" || bundle.ExportedAt != "2026-07-14T08:30:00Z" {
		t.Fatalf("unexpected bundle metadata: %+v", bundle)
	}
	if len(bundle.Groups) != 1 || bundle.Groups[0].Name != "高级组" || len(bundle.Groups[0].DispatchRules) != 1 {
		t.Fatalf("unexpected exported groups: %+v", bundle.Groups)
	}

	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	jsonText := string(payload)
	for _, forbidden := range []string{
		"price_book", "price-book-secret", "group_id", "revision", "active_targets",
		"created_at", "updated_at", "upstream", "tenant", "authorization",
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("export contains forbidden value %q: %s", forbidden, jsonText)
		}
	}
	for _, required := range []string{"client_surface_policy", "dispatch_rules", "gpt-latest", "gpt-5.5", "default_user_multiplier"} {
		if !strings.Contains(jsonText, required) {
			t.Fatalf("export is missing %q: %s", required, jsonText)
		}
	}
}

func TestGroupTransferRejectsLegacySchemaVersion(t *testing.T) {
	bundle := GroupTransferBundle{
		SchemaVersion: 1,
		BundleID:      "legacy-bundle",
		ExportedAt:    "2026-07-14T08:30:00Z",
		Groups: []GroupTransferGroup{{
			Name:                  "旧版分组",
			DefaultUserMultiplier: 1,
			Status:                StatusActive,
			ClientSurfacePolicy:   GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
		}},
	}
	_, err := NewGroupTransferService(&groupTransferRepoStub{}, GroupTransferOptions{}).Preview(
		context.Background(),
		"tenant-1",
		GroupImportRequest{Bundle: bundle},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported schema_version 1") {
		t.Fatalf("Preview() error = %v, want legacy schema rejection", err)
	}
}

func TestGroupTransferExportEnforcesBundleLimits(t *testing.T) {
	t.Run("groups", func(t *testing.T) {
		ids := make([]string, GroupTransferMaxGroups+1)
		for index := range ids {
			ids[index] = fmt.Sprintf("group-%d", index)
		}
		_, err := NewGroupTransferService(&groupTransferRepoStub{}, GroupTransferOptions{}).Export(context.Background(), "tenant-1", ids)
		if err == nil || !strings.Contains(err.Error(), "groups") {
			t.Fatalf("Export() error = %v, want group limit", err)
		}
	})

	t.Run("rules", func(t *testing.T) {
		repo := &groupTransferRepoStub{snapshots: []GroupConfigurationSnapshot{{
			GroupID: "group-1",
			Configuration: GroupTransferGroup{
				Name:          "large-rules",
				DispatchRules: make([]GroupTransferDispatchRule, GroupTransferMaxRules+1),
			},
		}}}
		_, err := NewGroupTransferService(repo, GroupTransferOptions{}).Export(context.Background(), "tenant-1", []string{"group-1"})
		if err == nil || !strings.Contains(err.Error(), "rules") {
			t.Fatalf("Export() error = %v, want rule limit", err)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		repo := &groupTransferRepoStub{snapshots: []GroupConfigurationSnapshot{{
			GroupID: "group-1",
			Configuration: GroupTransferGroup{
				Name:        "large-file",
				Description: strings.Repeat("x", GroupTransferMaxBytes),
			},
		}}}
		_, err := NewGroupTransferService(repo, GroupTransferOptions{}).Export(context.Background(), "tenant-1", []string{"group-1"})
		if err == nil || !strings.Contains(err.Error(), "bytes") {
			t.Fatalf("Export() error = %v, want byte limit", err)
		}
	})
}

func TestGroupTransferPreviewLoadsCopyTargetConflicts(t *testing.T) {
	repo := &groupTransferRepoStub{environment: GroupImportEnvironment{
		ExistingByName: map[string]GroupConfigurationSnapshot{
			"已有目标": {GroupID: "existing-target", Configuration: GroupTransferGroup{Name: "已有目标"}},
		},
		PriceBooks: map[string]GroupImportPriceBook{
			"price-1": {ID: "price-1", Status: StatusActive, Models: map[string][]string{}},
		},
	}}
	bundle := GroupTransferBundle{
		SchemaVersion: GroupTransferSchemaVersion,
		BundleID:      "bundle-copy-conflict",
		ExportedAt:    "2026-07-14T08:30:00Z",
		Groups: []GroupTransferGroup{{
			Name: "源分组", DefaultUserMultiplier: 1, Status: StatusActive,
			ClientSurfacePolicy: GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll},
		}},
	}
	preview, err := NewGroupTransferService(repo, GroupTransferOptions{}).Preview(context.Background(), "tenant-1", GroupImportRequest{
		Bundle: bundle,
		Choices: []GroupImportChoice{{
			SourceName: "源分组", Action: GroupImportActionCopy, TargetName: "已有目标", PriceBookID: "price-1",
		}},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Items[0].Errors) == 0 || !strings.Contains(strings.Join(preview.Items[0].Errors, " "), "已存在") {
		t.Fatalf("Errors = %#v, want target-name conflict", preview.Items[0].Errors)
	}
	if !containsTransferString(repo.groupNames, "已有目标") {
		t.Fatalf("loaded group names = %#v, want copy target", repo.groupNames)
	}
}

func containsTransferString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestGroupTransferImportAppliesEachGroupIndependently(t *testing.T) {
	repo := &groupTransferRepoStub{environment: GroupImportEnvironment{
		ExistingByName: map[string]GroupConfigurationSnapshot{},
		PriceBooks: map[string]GroupImportPriceBook{
			"price-1": {ID: "price-1", Status: StatusActive, Models: map[string][]string{}},
		},
	}}
	repo.apply = func(item PlannedGroupImport) (AppliedGroupImport, error) {
		if item.Source.Name == "失败组" {
			return AppliedGroupImport{}, errors.New("database unavailable")
		}
		return AppliedGroupImport{GroupID: "created-1", Status: item.Preview.AppliedStatus}, nil
	}
	svc := NewGroupTransferService(repo, GroupTransferOptions{})
	bundle := GroupTransferBundle{
		SchemaVersion: GroupTransferSchemaVersion,
		BundleID:      "bundle-import",
		ExportedAt:    "2026-07-14T08:30:00Z",
		Groups: []GroupTransferGroup{
			{Name: "成功组", DefaultUserMultiplier: 1, Status: StatusActive, ClientSurfacePolicy: GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll}},
			{Name: "失败组", DefaultUserMultiplier: 1, Status: StatusActive, ClientSurfacePolicy: GroupTransferClientSurfacePolicy{Mode: GroupClientSurfacePolicyAll}},
		},
	}
	choices := []GroupImportChoice{
		{SourceName: "成功组", Action: GroupImportActionCreate, PriceBookID: "price-1"},
		{SourceName: "失败组", Action: GroupImportActionCreate, PriceBookID: "price-1"},
	}
	_, err := svc.Preview(context.Background(), "tenant-1", GroupImportRequest{Bundle: bundle, Choices: choices})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	result, err := svc.Import(context.Background(), "tenant-1", GroupImportRequest{
		Bundle:  bundle,
		Choices: choices,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(repo.applied) != 2 {
		t.Fatalf("ApplyGroupImport calls = %d, want 2", len(repo.applied))
	}
	if result.Summary.Success != 1 || result.Summary.Error != 1 || result.Summary.Skip != 0 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if result.Items[0].GroupID != "created-1" || result.Items[0].Status != StatusDisabled || result.Items[0].Result != "success" {
		t.Fatalf("unexpected success result: %+v", result.Items[0])
	}
	if result.Items[1].Result != "error" || !strings.Contains(result.Items[1].Error, "database unavailable") {
		t.Fatalf("unexpected error result: %+v", result.Items[1])
	}
}

func TestGroupTransferRejectsUnknownFileFields(t *testing.T) {
	var bundle GroupTransferBundle
	raw := `{
			"schema_version":2,
		"bundle_id":"bundle-unknown",
		"exported_at":"2026-07-14T08:30:00Z",
		"future_bundle":true,
		"groups":[{
			"name":"兼容组",
			"allow_protocol_conversion":false,
			"sort_order":0,
			"status":"active",
			"future_group":"ignored",
			"client_surface_policy":{"mode":"all","allowed_surfaces":[],"future_policy":1},
			"dispatch_rules":[{
				"client_surface":"openai_responses",
				"match_type":"exact",
				"match_value":"latest",
				"target_model_code":"gpt-5.5",
				"priority":10,
				"status":"active",
				"future_rule":{}
			}]
		}]
	}`
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	repo := &groupTransferRepoStub{environment: GroupImportEnvironment{
		ExistingByName: map[string]GroupConfigurationSnapshot{},
		PriceBooks: map[string]GroupImportPriceBook{
			"price-1": {ID: "price-1", Status: StatusActive, Models: map[string][]string{"gpt-5.5": {"chat"}}},
		},
	}}
	_, err := NewGroupTransferService(repo, GroupTransferOptions{}).Preview(context.Background(), "tenant-1", GroupImportRequest{
		Bundle:  bundle,
		Choices: []GroupImportChoice{{SourceName: "兼容组", Action: GroupImportActionCreate, PriceBookID: "price-1"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported fields") {
		t.Fatalf("Preview() error = %v, want unknown-field rejection", err)
	}
}
