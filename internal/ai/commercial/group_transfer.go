package commercial

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"xiaodou/dai/internal/ai/core/surface"
)

const GroupTransferSchemaVersion = 3

const (
	GroupTransferMaxGroups = 500
	GroupTransferMaxRules  = 10_000
	GroupTransferMaxBytes  = 5 << 20
)

type GroupTransferBundle struct {
	SchemaVersion int                  `json:"schema_version"`
	BundleID      string               `json:"bundle_id"`
	ExportedAt    string               `json:"exported_at"`
	Groups        []GroupTransferGroup `json:"groups"`
	UnknownFields []string             `json:"-"`
	RawSize       int                  `json:"-"`
	_             struct{}             `json:"-" additionalProperties:"true"`
}

type GroupTransferGroup struct {
	Name                    string                           `json:"name"`
	Description             string                           `json:"description,omitempty"`
	DefaultUserMultiplier   float64                          `json:"default_user_multiplier"`
	UserDefaultVisible      bool                             `json:"user_default_visible"`
	AllowProtocolConversion bool                             `json:"allow_protocol_conversion"`
	RouteStrategy           RouteStrategy                    `json:"route_strategy" enum:"failover,weighted,adaptive"`
	RouteObjective          RouteObjective                   `json:"route_objective" enum:"balanced,cost,latency,stability"`
	SortOrder               int                              `json:"sort_order"`
	Status                  Status                           `json:"status" enum:"active,disabled"`
	ClientSurfacePolicy     GroupTransferClientSurfacePolicy `json:"client_surface_policy"`
	DispatchRules           []GroupTransferDispatchRule      `json:"dispatch_rules"`
	UnknownFields           []string                         `json:"-"`
	_                       struct{}                         `json:"-" additionalProperties:"true"`
}

type GroupTransferClientSurfacePolicy struct {
	Mode            GroupClientSurfacePolicyMode `json:"mode" enum:"all,restricted"`
	AllowedSurfaces []surface.ID                 `json:"allowed_surfaces" enum:"openai_chat,openai_responses,openai_embeddings,anthropic_messages,gemini_text,gemini_embeddings,openai_images,gemini_images"`
	UnknownFields   []string                     `json:"-"`
	_               struct{}                     `json:"-" additionalProperties:"true"`
}

type GroupTransferDispatchRule struct {
	ClientSurface surface.ID        `json:"client_surface" enum:"openai_chat,openai_responses,openai_embeddings,anthropic_messages,gemini_text,gemini_embeddings,openai_images,gemini_images"`
	MatchType     DispatchMatchType `json:"match_type" enum:"exact,prefix,wildcard,regex"`
	MatchValue    string            `json:"match_value"`
	TargetModelID string            `json:"target_model_code"`
	Priority      int               `json:"priority"`
	Status        Status            `json:"status" enum:"active,disabled"`
	Notes         string            `json:"notes,omitempty"`
	UnknownFields []string          `json:"-"`
	_             struct{}          `json:"-" additionalProperties:"true"`
}

type GroupConfigurationSnapshot struct {
	GroupID       string
	PriceBookID   string
	ActiveTargets int
	Configuration GroupTransferGroup
}

type GroupImportPriceBook struct {
	ID     string
	Status Status
	Models map[string][]string
}

func (p GroupImportPriceBook) SupportsModel(modelCode string, clientSurface surface.ID) (exists, compatible bool) {
	capabilities, exists := p.Models[modelCode]
	if !exists {
		return false, false
	}
	expected := transferCapabilityForSurface(clientSurface)
	if expected == "" {
		return true, true
	}
	for _, capability := range capabilities {
		if capability == expected {
			return true, true
		}
	}
	return true, false
}

type GroupImportEnvironment struct {
	ExistingByName map[string]GroupConfigurationSnapshot
	PriceBooks     map[string]GroupImportPriceBook
}

type GroupImportAction string

const (
	GroupImportActionSkip   GroupImportAction = "skip"
	GroupImportActionCreate GroupImportAction = "create"
	GroupImportActionUpdate GroupImportAction = "update"
	GroupImportActionCopy   GroupImportAction = "copy"
)

type GroupImportChoice struct {
	SourceName  string            `json:"source_name"`
	Action      GroupImportAction `json:"action" enum:"skip,create,update,copy"`
	TargetName  string            `json:"target_name,omitempty"`
	PriceBookID string            `json:"price_book_id,omitempty"`
}

type GroupImportRequest struct {
	Bundle  GroupTransferBundle `json:"bundle"`
	Choices []GroupImportChoice `json:"choices,omitempty"`
}

type GroupImportPreviewItem struct {
	SourceName    string            `json:"source_name"`
	TargetName    string            `json:"target_name"`
	Action        GroupImportAction `json:"action"`
	TargetGroupID string            `json:"target_group_id,omitempty"`
	PriceBookID   string            `json:"price_book_id,omitempty"`
	SourceStatus  Status            `json:"source_status"`
	AppliedStatus Status            `json:"applied_status"`
	Warnings      []string          `json:"warnings"`
	Errors        []string          `json:"errors"`
	MissingModels []string          `json:"missing_models"`
}

type GroupImportPreviewSummary struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Skip   int `json:"skip"`
	Error  int `json:"error"`
}

type GroupImportPreview struct {
	BundleID string                    `json:"bundle_id"`
	Items    []GroupImportPreviewItem  `json:"items"`
	Summary  GroupImportPreviewSummary `json:"summary"`
	Warnings []string                  `json:"warnings"`
}

type PlannedGroupImport struct {
	Source  GroupTransferGroup
	Preview GroupImportPreviewItem
}

type AppliedGroupImport struct {
	GroupID string
	Status  Status
}

type GroupImportResultItem struct {
	SourceName string            `json:"source_name"`
	TargetName string            `json:"target_name"`
	Action     GroupImportAction `json:"action"`
	GroupID    string            `json:"group_id,omitempty"`
	Status     Status            `json:"status,omitempty"`
	Result     string            `json:"result"`
	Error      string            `json:"error,omitempty"`
}

type GroupImportResultSummary struct {
	Success int `json:"success"`
	Skip    int `json:"skip"`
	Error   int `json:"error"`
}

type GroupImportResult struct {
	BundleID string                   `json:"bundle_id"`
	Items    []GroupImportResultItem  `json:"items"`
	Summary  GroupImportResultSummary `json:"summary"`
}

type GroupTransferRepository interface {
	SnapshotGroupConfigurations(ctx context.Context, tenantID string, groupIDs []string) ([]GroupConfigurationSnapshot, error)
	LoadGroupImportEnvironment(ctx context.Context, tenantID string, groupNames, priceBookIDs []string) (GroupImportEnvironment, error)
	ApplyGroupImport(ctx context.Context, tenantID string, item PlannedGroupImport) (AppliedGroupImport, error)
}

type GroupTransferOptions struct {
	Now   func() time.Time
	NewID func() string
}

type GroupTransferService struct {
	repo  GroupTransferRepository
	now   func() time.Time
	newID func() string
}

func NewGroupTransferService(repo GroupTransferRepository, options GroupTransferOptions) *GroupTransferService {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	newID := options.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	return &GroupTransferService{repo: repo, now: now, newID: newID}
}

func (s *GroupTransferService) Export(ctx context.Context, tenantID string, groupIDs []string) (GroupTransferBundle, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return GroupTransferBundle{}, newValidationError("tenant_id", "tenant_id is required")
	}
	ids := uniqueTransferStrings(groupIDs)
	if len(ids) == 0 {
		return GroupTransferBundle{}, newValidationError("group_ids", "at least one group is required")
	}
	if len(ids) > GroupTransferMaxGroups {
		return GroupTransferBundle{}, newValidationError("group_ids", fmt.Sprintf("cannot exceed %d groups", GroupTransferMaxGroups))
	}
	snapshots, err := s.repo.SnapshotGroupConfigurations(ctx, tenantID, ids)
	if err != nil {
		return GroupTransferBundle{}, err
	}
	byID := make(map[string]GroupTransferGroup, len(snapshots))
	for _, snapshot := range snapshots {
		configuration := snapshot.Configuration
		if configuration.ClientSurfacePolicy.AllowedSurfaces == nil {
			configuration.ClientSurfacePolicy.AllowedSurfaces = []surface.ID{}
		}
		if configuration.DispatchRules == nil {
			configuration.DispatchRules = []GroupTransferDispatchRule{}
		}
		byID[snapshot.GroupID] = configuration
	}
	groups := make([]GroupTransferGroup, 0, len(ids))
	ruleCount := 0
	for _, id := range ids {
		configuration, ok := byID[id]
		if !ok {
			return GroupTransferBundle{}, newValidationError("group_ids", "group not found: "+id)
		}
		groups = append(groups, configuration)
		ruleCount += len(configuration.DispatchRules)
	}
	if ruleCount > GroupTransferMaxRules {
		return GroupTransferBundle{}, newValidationError("dispatch_rules", fmt.Sprintf("cannot exceed %d rules", GroupTransferMaxRules))
	}
	bundle := GroupTransferBundle{
		SchemaVersion: GroupTransferSchemaVersion,
		BundleID:      s.newID(),
		ExportedAt:    s.now().UTC().Format(time.RFC3339),
		Groups:        groups,
	}
	payload, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return GroupTransferBundle{}, err
	}
	if len(payload)+1 > GroupTransferMaxBytes {
		return GroupTransferBundle{}, newValidationError("bundle", fmt.Sprintf("cannot exceed %d bytes", GroupTransferMaxBytes))
	}
	return bundle, nil
}

func (s *GroupTransferService) Preview(ctx context.Context, tenantID string, req GroupImportRequest) (GroupImportPreview, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return GroupImportPreview{}, newValidationError("tenant_id", "tenant_id is required")
	}
	if err := validateTransferBundle(req.Bundle); err != nil {
		return GroupImportPreview{}, err
	}
	choiceBySource := make(map[string]GroupImportChoice, len(req.Choices))
	priceBookIDs := make([]string, 0, len(req.Choices))
	for _, choice := range req.Choices {
		name := strings.TrimSpace(choice.SourceName)
		if name == "" {
			return GroupImportPreview{}, newValidationError("choices.source_name", "source_name is required")
		}
		if _, exists := choiceBySource[name]; exists {
			return GroupImportPreview{}, newValidationError("choices.source_name", "duplicate source_name: "+name)
		}
		choice.SourceName = name
		choice.TargetName = strings.TrimSpace(choice.TargetName)
		choice.PriceBookID = strings.TrimSpace(choice.PriceBookID)
		choiceBySource[name] = choice
		if choice.PriceBookID != "" {
			priceBookIDs = append(priceBookIDs, choice.PriceBookID)
		}
	}
	groupNames := make([]string, 0, len(req.Bundle.Groups))
	for _, group := range req.Bundle.Groups {
		groupNames = append(groupNames, strings.TrimSpace(group.Name))
	}
	for _, choice := range choiceBySource {
		if choice.Action == GroupImportActionCopy && choice.TargetName != "" {
			groupNames = append(groupNames, choice.TargetName)
		}
	}
	environment, err := s.repo.LoadGroupImportEnvironment(ctx, tenantID, groupNames, uniqueTransferStrings(priceBookIDs))
	if err != nil {
		return GroupImportPreview{}, err
	}
	preview := GroupImportPreview{
		BundleID: req.Bundle.BundleID,
		Items:    make([]GroupImportPreviewItem, 0, len(req.Bundle.Groups)),
		Warnings: []string{},
	}
	plannedNewNames := make(map[string]string, len(req.Bundle.Groups))
	for _, source := range req.Bundle.Groups {
		source.Name = strings.TrimSpace(source.Name)
		choice, selected := choiceBySource[source.Name]
		existing, exists := environment.ExistingByName[source.Name]
		if !selected {
			choice = GroupImportChoice{SourceName: source.Name}
			if exists {
				choice.Action = GroupImportActionSkip
			} else {
				choice.Action = GroupImportActionCreate
			}
		}
		item := planGroupImport(source, choice, existing, exists, environment)
		if item.Action == GroupImportActionCreate || item.Action == GroupImportActionCopy {
			if previousSource, duplicate := plannedNewNames[item.TargetName]; item.TargetName != "" && duplicate {
				item.Errors = append(item.Errors, "目标分组名称与导入项 "+previousSource+" 重复")
			} else if item.TargetName != "" {
				plannedNewNames[item.TargetName] = item.SourceName
			}
		}
		preview.Items = append(preview.Items, item)
		accumulateGroupImportSummary(&preview.Summary, item)
	}
	return preview, nil
}

func (s *GroupTransferService) Import(ctx context.Context, tenantID string, req GroupImportRequest) (GroupImportResult, error) {
	preview, err := s.Preview(ctx, tenantID, req)
	if err != nil {
		return GroupImportResult{}, err
	}
	sourceByName := make(map[string]GroupTransferGroup, len(req.Bundle.Groups))
	for _, source := range req.Bundle.Groups {
		source.Name = strings.TrimSpace(source.Name)
		sourceByName[source.Name] = source
	}
	result := GroupImportResult{
		BundleID: req.Bundle.BundleID,
		Items:    make([]GroupImportResultItem, 0, len(preview.Items)),
	}
	for _, item := range preview.Items {
		entry := GroupImportResultItem{
			SourceName: item.SourceName,
			TargetName: item.TargetName,
			Action:     item.Action,
			Status:     item.AppliedStatus,
		}
		if item.Action == GroupImportActionSkip && len(item.Errors) == 0 {
			entry.Result = "skipped"
			entry.GroupID = item.TargetGroupID
			result.Summary.Skip++
			result.Items = append(result.Items, entry)
			continue
		}
		if len(item.Errors) > 0 {
			entry.Result = "error"
			entry.Error = strings.Join(item.Errors, "；")
			result.Summary.Error++
			result.Items = append(result.Items, entry)
			continue
		}
		applied, applyErr := s.repo.ApplyGroupImport(ctx, tenantID, PlannedGroupImport{
			Source:  sourceByName[item.SourceName],
			Preview: item,
		})
		if applyErr != nil {
			entry.Result = "error"
			entry.Error = applyErr.Error()
			result.Summary.Error++
			result.Items = append(result.Items, entry)
			continue
		}
		entry.Result = "success"
		entry.GroupID = applied.GroupID
		entry.Status = applied.Status
		result.Summary.Success++
		result.Items = append(result.Items, entry)
	}
	return result, nil
}

func planGroupImport(source GroupTransferGroup, choice GroupImportChoice, existing GroupConfigurationSnapshot, exists bool, environment GroupImportEnvironment) GroupImportPreviewItem {
	item := GroupImportPreviewItem{
		SourceName:    source.Name,
		TargetName:    source.Name,
		Action:        choice.Action,
		SourceStatus:  source.Status,
		AppliedStatus: source.Status,
		Warnings:      []string{},
		Errors:        []string{},
		MissingModels: []string{},
	}
	if exists {
		item.TargetGroupID = existing.GroupID
	}
	switch choice.Action {
	case GroupImportActionSkip:
		if exists {
			item.TargetGroupID = existing.GroupID
			item.PriceBookID = existing.PriceBookID
		}
	case GroupImportActionCreate:
		item.PriceBookID = choice.PriceBookID
		item.AppliedStatus = StatusDisabled
		if exists {
			item.Errors = append(item.Errors, "同名分组已存在，请选择更新、跳过或另存为新分组")
		}
		if item.PriceBookID == "" {
			item.Errors = append(item.Errors, "请选择价格表")
		}
	case GroupImportActionCopy:
		item.TargetName = choice.TargetName
		item.PriceBookID = choice.PriceBookID
		item.AppliedStatus = StatusDisabled
		if item.TargetName == "" {
			item.Errors = append(item.Errors, "另存为新分组时必须填写名称")
		}
		if _, conflict := environment.ExistingByName[item.TargetName]; conflict {
			item.Errors = append(item.Errors, "目标分组名称已存在")
		}
		if item.PriceBookID == "" {
			item.Errors = append(item.Errors, "请选择价格表")
		}
	case GroupImportActionUpdate:
		if !exists {
			item.Errors = append(item.Errors, "没有可更新的同名分组")
			break
		}
		item.TargetGroupID = existing.GroupID
		item.TargetName = existing.Configuration.Name
		item.PriceBookID = choice.PriceBookID
		if item.PriceBookID == "" {
			item.PriceBookID = existing.PriceBookID
		}
		if existing.ActiveTargets == 0 {
			item.AppliedStatus = StatusDisabled
		}
		if existing.ActiveTargets == 0 {
			item.Warnings = append(item.Warnings, "现有分组没有活动上游目标，导入后保持停用")
		}
	default:
		item.Errors = append(item.Errors, "不支持的导入动作")
	}
	if item.Action != GroupImportActionSkip {
		item.Errors = append(item.Errors, validateTransferGroup(source)...)
	}
	if item.Action != GroupImportActionSkip && item.PriceBookID != "" {
		priceBook, ok := environment.PriceBooks[item.PriceBookID]
		if !ok {
			item.Errors = append(item.Errors, "价格表不存在")
		} else if priceBook.Status != StatusActive {
			item.Errors = append(item.Errors, "价格表未启用")
		} else {
			missing := make(map[string]struct{})
			incompatible := make(map[string]struct{})
			for _, rule := range source.DispatchRules {
				if rule.Status != StatusActive {
					continue
				}
				exists, compatible := priceBook.SupportsModel(rule.TargetModelID, rule.ClientSurface)
				if !exists {
					missing[rule.TargetModelID] = struct{}{}
					continue
				}
				if !compatible {
					incompatible[rule.TargetModelID] = struct{}{}
				}
			}
			item.MissingModels = sortedTransferMapKeys(missing)
			if len(item.MissingModels) > 0 {
				item.AppliedStatus = StatusDisabled
				item.Warnings = append(item.Warnings, "所选价格表缺少调度规则引用的逻辑模型，导入后保持停用")
			}
			if len(incompatible) > 0 {
				item.AppliedStatus = StatusDisabled
				item.Warnings = append(item.Warnings, "所选价格表中的模型能力与客户端 API 入口不匹配，导入后保持停用: "+strings.Join(sortedTransferMapKeys(incompatible), ", "))
			}
		}
	}
	return item
}

func validateTransferGroup(group GroupTransferGroup) []string {
	errors := make([]string, 0)
	if _, err := normalizeGroupWrite(GroupWrite{
		Name:                    group.Name,
		Description:             group.Description,
		RetailPriceBookID:       "transfer-price-book",
		DefaultUserMultiplier:   group.DefaultUserMultiplier,
		UserDefaultVisible:      group.UserDefaultVisible,
		AllowProtocolConversion: group.AllowProtocolConversion,
		RouteStrategy:           group.RouteStrategy,
		RouteObjective:          group.RouteObjective,
		Status:                  group.Status,
		SortOrder:               group.SortOrder,
	}); err != nil {
		errors = append(errors, err.Error())
	}
	if group.SortOrder > math.MaxInt32 {
		errors = append(errors, "sort_order 超出允许范围")
	}
	if group.DefaultUserMultiplier > 999999.9999 {
		errors = append(errors, "default_user_multiplier 超出允许范围")
	}
	switch group.ClientSurfacePolicy.Mode {
	case GroupClientSurfacePolicyAll:
	case GroupClientSurfacePolicyRestricted:
		if len(group.ClientSurfacePolicy.AllowedSurfaces) == 0 {
			errors = append(errors, "受限 API 入口策略至少需要一个入口")
		}
	default:
		errors = append(errors, "API 入口策略模式无效")
	}
	seenSurfaces := make(map[surface.ID]struct{}, len(group.ClientSurfacePolicy.AllowedSurfaces))
	for _, clientSurface := range group.ClientSurfacePolicy.AllowedSurfaces {
		if !surface.IsKnown(clientSurface) {
			errors = append(errors, "API 入口策略包含不支持的入口: "+string(clientSurface))
			continue
		}
		if _, exists := seenSurfaces[clientSurface]; exists {
			errors = append(errors, "API 入口策略包含重复入口: "+string(clientSurface))
		}
		seenSurfaces[clientSurface] = struct{}{}
	}
	seenRules := make(map[string]struct{}, len(group.DispatchRules))
	for index, rule := range group.DispatchRules {
		if rule.Priority > math.MaxInt32 {
			errors = append(errors, fmt.Sprintf("第 %d 条调度规则的 priority 超出允许范围", index+1))
		}
		key := strings.Join([]string{string(rule.ClientSurface), string(rule.MatchType), strings.TrimSpace(rule.MatchValue)}, "\x00")
		if _, exists := seenRules[key]; exists {
			errors = append(errors, fmt.Sprintf("第 %d 条调度规则与前面的匹配规则重复", index+1))
		} else {
			seenRules[key] = struct{}{}
		}
		_, err := normalizeDispatchRuleWrite(DispatchRuleWrite{
			ClientSurface: rule.ClientSurface,
			MatchType:     rule.MatchType,
			MatchValue:    rule.MatchValue,
			TargetModelID: rule.TargetModelID,
			Priority:      rule.Priority,
			Notes:         rule.Notes,
		})
		if err != nil {
			if rule.MatchType == DispatchMatchRegex {
				errors = append(errors, fmt.Sprintf("第 %d 条调度规则的正则表达式无效", index+1))
			} else {
				errors = append(errors, fmt.Sprintf("第 %d 条调度规则无效: %s", index+1, err.Error()))
			}
		}
	}
	return errors
}

func accumulateGroupImportSummary(summary *GroupImportPreviewSummary, item GroupImportPreviewItem) {
	if len(item.Errors) > 0 {
		summary.Error++
		return
	}
	switch item.Action {
	case GroupImportActionCreate, GroupImportActionCopy:
		summary.Create++
	case GroupImportActionUpdate:
		summary.Update++
	case GroupImportActionSkip:
		summary.Skip++
	}
}

func (bundle *GroupTransferBundle) UnmarshalJSON(data []byte) error {
	type plain GroupTransferBundle
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*bundle = GroupTransferBundle(value)
	bundle.UnknownFields = unknownTransferFields(data, "schema_version", "bundle_id", "exported_at", "groups")
	bundle.RawSize = len(data)
	return nil
}

func (group *GroupTransferGroup) UnmarshalJSON(data []byte) error {
	type plain GroupTransferGroup
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*group = GroupTransferGroup(value)
	group.UnknownFields = unknownTransferFields(data,
		"name", "description", "default_user_multiplier", "user_default_visible",
		"allow_protocol_conversion",
		"route_strategy", "route_objective",
		"sort_order", "status", "client_surface_policy", "dispatch_rules")
	return nil
}

func (policy *GroupTransferClientSurfacePolicy) UnmarshalJSON(data []byte) error {
	type plain GroupTransferClientSurfacePolicy
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*policy = GroupTransferClientSurfacePolicy(value)
	policy.UnknownFields = unknownTransferFields(data, "mode", "allowed_surfaces")
	return nil
}

func (rule *GroupTransferDispatchRule) UnmarshalJSON(data []byte) error {
	type plain GroupTransferDispatchRule
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*rule = GroupTransferDispatchRule(value)
	rule.UnknownFields = unknownTransferFields(data,
		"client_surface", "match_type", "match_value", "target_model_code",
		"priority", "status", "notes")
	return nil
}

func unknownTransferFields(data []byte, known ...string) []string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, field := range known {
		knownSet[field] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range fields {
		if _, exists := knownSet[field]; !exists {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	return unknown
}

func transferUnknownFieldWarnings(bundle GroupTransferBundle) []string {
	warnings := make([]string, 0)
	for _, field := range bundle.UnknownFields {
		warnings = append(warnings, "已忽略导出文件未知字段: "+field)
	}
	for groupIndex, group := range bundle.Groups {
		prefix := fmt.Sprintf("分组 %d", groupIndex+1)
		for _, field := range group.UnknownFields {
			warnings = append(warnings, prefix+" 已忽略未知字段: "+field)
		}
		for _, field := range group.ClientSurfacePolicy.UnknownFields {
			warnings = append(warnings, prefix+" API 入口策略已忽略未知字段: "+field)
		}
		for ruleIndex, rule := range group.DispatchRules {
			for _, field := range rule.UnknownFields {
				warnings = append(warnings, fmt.Sprintf("%s 调度规则 %d 已忽略未知字段: %s", prefix, ruleIndex+1, field))
			}
		}
	}
	return warnings
}

func validateTransferBundle(bundle GroupTransferBundle) error {
	if unknown := transferUnknownFieldWarnings(bundle); len(unknown) > 0 {
		return newValidationError("bundle", "unsupported fields: "+strings.Join(unknown, "; "))
	}
	size := bundle.RawSize
	if size == 0 {
		payload, err := json.Marshal(bundle)
		if err != nil {
			return newValidationError("bundle", "bundle cannot be encoded")
		}
		size = len(payload)
	}
	if size > GroupTransferMaxBytes {
		return newValidationError("bundle", fmt.Sprintf("cannot exceed %d bytes", GroupTransferMaxBytes))
	}
	if bundle.SchemaVersion != GroupTransferSchemaVersion {
		return newValidationError("schema_version", fmt.Sprintf("unsupported schema_version %d", bundle.SchemaVersion))
	}
	if strings.TrimSpace(bundle.BundleID) == "" {
		return newValidationError("bundle_id", "bundle_id is required")
	}
	if _, err := time.Parse(time.RFC3339, bundle.ExportedAt); err != nil {
		return newValidationError("exported_at", "exported_at must be RFC3339")
	}
	if len(bundle.Groups) == 0 {
		return newValidationError("groups", "at least one group is required")
	}
	if len(bundle.Groups) > GroupTransferMaxGroups {
		return newValidationError("groups", fmt.Sprintf("cannot exceed %d groups", GroupTransferMaxGroups))
	}
	ruleCount := 0
	seen := make(map[string]struct{}, len(bundle.Groups))
	for _, group := range bundle.Groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			return newValidationError("groups.name", "name is required")
		}
		if _, exists := seen[name]; exists {
			return newValidationError("groups.name", "duplicate group name: "+name)
		}
		seen[name] = struct{}{}
		ruleCount += len(group.DispatchRules)
	}
	if ruleCount > GroupTransferMaxRules {
		return newValidationError("dispatch_rules", fmt.Sprintf("cannot exceed %d rules", GroupTransferMaxRules))
	}
	return nil
}

func transferCapabilityForSurface(id surface.ID) string {
	switch id {
	case surface.OpenAIChat, surface.OpenAIResponses, surface.AnthropicMessages, surface.GeminiText:
		return "chat"
	case surface.OpenAIEmbeddings, surface.GeminiEmbeddings:
		return "embedding"
	case surface.OpenAIImages, surface.GeminiImages:
		return "image"
	default:
		return ""
	}
}

func uniqueTransferStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sortedTransferMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
