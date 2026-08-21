package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/libs/go/httpx"
)

const upstreamAccountTransferSchemaVersion = 4

type upstreamAccountExportInput struct {
	Body struct {
		AccountIDs           []string `json:"account_ids" doc:"要导出的上游账号 ID 列表"`
		IncludeModelBindings bool     `json:"include_model_bindings" doc:"是否包含显式模型绑定"`
	}
}

type upstreamAccountTransferBindingDTO struct {
	ModelCode                   string `json:"model_code"`
	CapabilityType              string `json:"capability_type"`
	APIFormat                   string `json:"api_format"`
	UpstreamModelName           string `json:"upstream_model_name"`
	Status                      string `json:"status"`
	ImageStreamMode             string `json:"image_stream_mode,omitempty"`
	ImageEditTransport          string `json:"image_edit_transport,omitempty"`
	ImageUpstreamResponseFormat string `json:"image_upstream_response_format,omitempty"`
	ImageMaxOutputCount         int    `json:"image_max_output_count,omitempty"`
	ImageEditMaxOutputCount     int    `json:"image_edit_max_output_count,omitempty"`
}

type upstreamAccountTransferAccountDTO struct {
	Name                  string                              `json:"name"`
	TenantDisplayName     string                              `json:"tenant_display_name"`
	TenantAccessMode      string                              `json:"tenant_access_mode"`
	BaseURL               string                              `json:"base_url"`
	APIKey                string                              `json:"api_key"`
	DefaultProviderFamily string                              `json:"default_provider_family"`
	ConcurrencyLimit      *int32                              `json:"concurrency_limit,omitempty" minimum:"1"`
	Status                string                              `json:"status"`
	ExtraHeaders          json.RawMessage                     `json:"extra_headers,omitempty"`
	ModelBindings         []upstreamAccountTransferBindingDTO `json:"model_bindings,omitempty"`
}

type upstreamAccountExportOutput struct {
	Body struct {
		SchemaVersion            int                                 `json:"schema_version"`
		ExportedAt               string                              `json:"exported_at"`
		ContainsPlaintextAPIKeys bool                                `json:"contains_plaintext_api_keys"`
		Accounts                 []upstreamAccountTransferAccountDTO `json:"accounts"`
	}
}

type upstreamAccountImportPreviewInput struct {
	Body upstreamAccountImportRequest
}

type upstreamAccountImportInput struct {
	Body upstreamAccountImportRequest
}

type upstreamAccountImportRequest struct {
	Accounts                 []upstreamAccountTransferAccountDTO `json:"accounts"`
	DefaultPriceBookID       string                              `json:"default_price_book_id,omitempty"`
	DefaultTenantMultiplier  *float64                            `json:"default_tenant_multiplier,omitempty"`
	DuplicateAccountStrategy string                              `json:"duplicate_account_strategy,omitempty" enum:"skip"`
	DuplicateBindingStrategy string                              `json:"duplicate_binding_strategy,omitempty" enum:"skip"`
}

type upstreamAccountImportPreviewItemDTO struct {
	Name                   string   `json:"name"`
	BaseURL                string   `json:"base_url"`
	Action                 string   `json:"action"`
	Reason                 string   `json:"reason,omitempty"`
	ModelBindingCount      int      `json:"model_binding_count"`
	DuplicateModelBindings int      `json:"duplicate_model_bindings,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
}

type upstreamAccountImportSummaryDTO struct {
	CreateAccounts      int `json:"create_accounts"`
	SkipAccounts        int `json:"skip_accounts"`
	CreateModelBindings int `json:"create_model_bindings"`
	SkipModelBindings   int `json:"skip_model_bindings"`
	ErrorAccounts       int `json:"error_accounts"`
}

type upstreamAccountImportPreviewOutput struct {
	Body struct {
		Items   []upstreamAccountImportPreviewItemDTO `json:"items"`
		Summary upstreamAccountImportSummaryDTO       `json:"summary"`
	}
}

type upstreamAccountImportSkippedDTO struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type upstreamAccountImportCreatedBindingDTO struct {
	AccountName string `json:"account_name"`
	ModelCode   string `json:"model_code"`
	BindingID   string `json:"binding_id"`
}

type upstreamAccountImportOutput struct {
	Body struct {
		CreatedAccountIDs      []string                                 `json:"created_account_ids"`
		SkippedAccounts        []upstreamAccountImportSkippedDTO        `json:"skipped_accounts"`
		CreatedModelBindingIDs []upstreamAccountImportCreatedBindingDTO `json:"created_model_bindings"`
		SkippedModelBindings   []upstreamAccountImportSkippedDTO        `json:"skipped_model_bindings"`
		Summary                upstreamAccountImportSummaryDTO          `json:"summary"`
	}
}

func registerUpstreamAccountTransfer(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-export-upstream-accounts",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts/export",
		Summary:     "导出上游账号",
		Description: "导出选中的上游账号。响应包含明文上游 API key，调用方必须妥善保管导出文件。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *upstreamAccountExportInput) (*upstreamAccountExportOutput, error) {
		out, err := exportUpstreamAccounts(ctx, d, in.Body.AccountIDs, in.Body.IncludeModelBindings)
		if err != nil {
			voidAdminAudit(ctx, d, "upstream_accounts.export", "upstream_account", "", map[string]any{
				"account_ids":            in.Body.AccountIDs,
				"include_model_bindings": in.Body.IncludeModelBindings,
			}, "failed", 500)
			return nil, err
		}
		voidAdminAudit(ctx, d, "upstream_accounts.export", "upstream_account", "", map[string]any{
			"account_ids":            in.Body.AccountIDs,
			"account_count":          len(out.Body.Accounts),
			"include_model_bindings": in.Body.IncludeModelBindings,
		}, "success", 200)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-preview-import-upstream-accounts",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts/import/preview",
		Summary:     "预检导入上游账号",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *upstreamAccountImportPreviewInput) (*upstreamAccountImportPreviewOutput, error) {
		preview, err := previewImportUpstreamAccounts(ctx, d, in.Body)
		if err != nil {
			return nil, err
		}
		out := &upstreamAccountImportPreviewOutput{}
		out.Body.Items = preview.Items
		out.Body.Summary = preview.Summary
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-import-upstream-accounts",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts/import",
		Summary:     "导入上游账号",
		Description: "导入上游账号并重新加密明文 API key。价格表与成本倍率使用当前系统导入参数，不从导出文件继承。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *upstreamAccountImportInput) (*upstreamAccountImportOutput, error) {
		out, err := importUpstreamAccounts(ctx, d, in.Body)
		if err != nil {
			voidAdminAudit(ctx, d, "upstream_accounts.import", "upstream_account", "", map[string]any{
				"account_count": len(in.Body.Accounts),
			}, "failed", 500)
			return nil, err
		}
		voidAdminAudit(ctx, d, "upstream_accounts.import", "upstream_account", "", map[string]any{
			"created_account_ids":       out.Body.CreatedAccountIDs,
			"created_account_count":     len(out.Body.CreatedAccountIDs),
			"created_binding_count":     len(out.Body.CreatedModelBindingIDs),
			"skipped_account_count":     len(out.Body.SkippedAccounts),
			"skipped_binding_count":     len(out.Body.SkippedModelBindings),
			"default_price_book_id":     in.Body.DefaultPriceBookID,
			"default_tenant_multiplier": defaultTenantMultiplier(in.Body.DefaultTenantMultiplier),
		}, "success", 200)
		return out, nil
	})
}

func exportUpstreamAccounts(ctx context.Context, d AIDeps, accountIDs []string, includeBindings bool) (*upstreamAccountExportOutput, error) {
	if d.Accounts == nil || d.AccountReader == nil || d.ProviderSecrets == nil {
		return nil, httpx.ErrUnavailable.WithDetail("account service or provider secret codec is not configured")
	}
	if includeBindings && d.ModelBindings == nil {
		return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	ids := uniqueNonEmptyStrings(accountIDs)
	if len(ids) == 0 {
		return nil, httpx.ErrBadRequest.WithDetail("account_ids is required")
	}
	accounts, err := d.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	byID := make(map[string]domain.UpstreamAccount, len(accounts))
	for _, account := range accounts {
		byID[account.ID] = account
	}
	out := &upstreamAccountExportOutput{}
	out.Body.SchemaVersion = upstreamAccountTransferSchemaVersion
	out.Body.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	out.Body.ContainsPlaintextAPIKeys = true
	out.Body.Accounts = make([]upstreamAccountTransferAccountDTO, 0, len(ids))
	for _, id := range ids {
		account, ok := byID[id]
		if !ok {
			return nil, httpx.ErrNotFound.WithDetail("upstream account not found: " + id)
		}
		secretRow, err := d.AccountReader.GetAccountSecret(ctx, id)
		if err != nil {
			return nil, mapServiceError(err)
		}
		apiKey, err := d.ProviderSecrets.Decrypt(secretRow.Ciphertext)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("decrypt provider key failed for account " + id)
		}
		item := upstreamAccountTransferAccountDTO{
			Name:                  account.Name,
			TenantDisplayName:     account.TenantDisplayName,
			TenantAccessMode:      account.TenantAccessMode,
			BaseURL:               account.BaseURL,
			APIKey:                apiKey,
			DefaultProviderFamily: account.DefaultProtocol,
			ConcurrencyLimit:      intPtrToInt32Ptr(account.ConcurrencyLimit),
			Status:                portableUpstreamAccountStatus(account.Status),
			ExtraHeaders:          copyRawJSONOrObject(account.ExtraHeaders),
		}
		if includeBindings {
			bindings, err := listUpstreamModelBindings(ctx, d, "direct_upstream", id)
			if err != nil {
				return nil, err
			}
			item.ModelBindings = make([]upstreamAccountTransferBindingDTO, 0, len(bindings))
			for _, binding := range bindings {
				item.ModelBindings = append(item.ModelBindings, bindingRecordToTransferDTO(binding))
			}
		}
		out.Body.Accounts = append(out.Body.Accounts, item)
	}
	return out, nil
}

type importPreviewResult struct {
	Items   []upstreamAccountImportPreviewItemDTO
	Summary upstreamAccountImportSummaryDTO
}

func previewImportUpstreamAccounts(ctx context.Context, d AIDeps, req upstreamAccountImportRequest) (importPreviewResult, error) {
	if d.Accounts == nil {
		return importPreviewResult{}, httpx.ErrUnavailable.WithDetail("account service is not configured")
	}
	if len(req.Accounts) == 0 {
		return importPreviewResult{}, httpx.ErrBadRequest.WithDetail("accounts is required")
	}
	if err := validateImportStrategies(req); err != nil {
		return importPreviewResult{}, err
	}
	if err := validateImportPriceBook(ctx, d, req.DefaultPriceBookID); err != nil {
		return importPreviewResult{}, err
	}
	existingNames, err := existingUpstreamAccountNames(ctx, d)
	if err != nil {
		return importPreviewResult{}, err
	}
	seenNames := map[string]struct{}{}
	result := importPreviewResult{Items: make([]upstreamAccountImportPreviewItemDTO, 0, len(req.Accounts))}
	for _, account := range req.Accounts {
		item := previewImportAccount(account, existingNames, seenNames)
		result.Items = append(result.Items, item)
		accumulateImportPreview(&result.Summary, item)
	}
	return result, nil
}

func importUpstreamAccounts(ctx context.Context, d AIDeps, req upstreamAccountImportRequest) (*upstreamAccountImportOutput, error) {
	if d.Accounts == nil || d.AccountManager == nil {
		return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
	}
	if hasTransferModelBindings(req.Accounts) && d.ModelBindings == nil {
		return nil, httpx.ErrUnavailable.WithDetail("database is not configured")
	}
	if _, err := previewImportUpstreamAccounts(ctx, d, req); err != nil {
		return nil, err
	}
	out := &upstreamAccountImportOutput{}
	existingNames, err := existingUpstreamAccountNames(ctx, d)
	if err != nil {
		return nil, err
	}
	seenNames := map[string]struct{}{}
	for _, account := range req.Accounts {
		item := previewImportAccount(account, existingNames, seenNames)
		if item.Action != "create" {
			out.Body.SkippedAccounts = append(out.Body.SkippedAccounts, upstreamAccountImportSkippedDTO{Name: account.Name, Reason: item.Reason})
			if item.Action == "error" {
				out.Body.Summary.ErrorAccounts++
			} else {
				out.Body.Summary.SkipAccounts++
			}
			out.Body.Summary.SkipModelBindings += len(account.ModelBindings)
			continue
		}
		multiplier := defaultTenantMultiplier(req.DefaultTenantMultiplier)
		created, err := d.AccountManager.CreateAccount(ctx, upstreamcontrol.CreateAccountInput{
			Name:              strings.TrimSpace(account.Name),
			TenantDisplayName: strings.TrimSpace(account.TenantDisplayName),
			TenantAccessMode:  strings.TrimSpace(account.TenantAccessMode),
			BaseURL:           strings.TrimSpace(account.BaseURL),
			APIKey:            strings.TrimSpace(account.APIKey),
			ExtraHeaders:      normalizedRawJSONBytes(account.ExtraHeaders),
			DefaultProtocol:   strings.TrimSpace(account.DefaultProviderFamily),
			ConcurrencyLimit:  int32PtrToIntPtr(account.ConcurrencyLimit),
			PriceBookID:       strings.TrimSpace(req.DefaultPriceBookID),
			TenantMultiplier:  &multiplier,
			Status:            domain.UpstreamAccountStatusDisabled,
		})
		if err != nil {
			out.Body.SkippedAccounts = append(out.Body.SkippedAccounts, upstreamAccountImportSkippedDTO{Name: account.Name, Reason: err.Error()})
			out.Body.Summary.ErrorAccounts++
			out.Body.Summary.SkipModelBindings += len(account.ModelBindings)
			continue
		}
		out.Body.CreatedAccountIDs = append(out.Body.CreatedAccountIDs, created.ID)
		out.Body.Summary.CreateAccounts++
		existingNames[strings.TrimSpace(account.Name)] = struct{}{}
		createdBindingKeys := map[string]struct{}{}
		for _, binding := range account.ModelBindings {
			key := transferBindingDuplicateKey(binding)
			if _, exists := createdBindingKeys[key]; exists {
				out.Body.SkippedModelBindings = append(out.Body.SkippedModelBindings, upstreamAccountImportSkippedDTO{Name: binding.ModelCode, Reason: "duplicate binding in import file"})
				out.Body.Summary.SkipModelBindings++
				continue
			}
			createdBindingKeys[key] = struct{}{}
			createdBinding, err := createUpstreamModelBinding(ctx, d, "direct_upstream", created.ID, fixedProviderEndpointProtocolFromAccount(created.DefaultProtocol), nil, transferBindingToWriteRequest(binding))
			if err != nil {
				out.Body.SkippedModelBindings = append(out.Body.SkippedModelBindings, upstreamAccountImportSkippedDTO{Name: binding.ModelCode, Reason: err.Error()})
				out.Body.Summary.SkipModelBindings++
				continue
			}
			out.Body.CreatedModelBindingIDs = append(out.Body.CreatedModelBindingIDs, upstreamAccountImportCreatedBindingDTO{
				AccountName: account.Name,
				ModelCode:   createdBinding.ModelCode,
				BindingID:   createdBinding.ID,
			})
			out.Body.Summary.CreateModelBindings++
		}
	}
	return out, nil
}

func bindingRecordToTransferDTO(item domain.UpstreamModelBinding) upstreamAccountTransferBindingDTO {
	imagePolicy := parseImageGenerationBindingPolicy(item.ConfigJSON)
	return upstreamAccountTransferBindingDTO{
		ModelCode:                   item.ModelCode,
		CapabilityType:              item.CapabilityType,
		APIFormat:                   item.APIFormat,
		UpstreamModelName:           item.UpstreamModelName,
		Status:                      item.Status,
		ImageStreamMode:             imagePolicy.StreamMode,
		ImageEditTransport:          imagePolicy.EditTransport,
		ImageUpstreamResponseFormat: imagePolicy.UpstreamResponseFormat,
		ImageMaxOutputCount:         imagePolicy.MaxOutputCount,
		ImageEditMaxOutputCount:     imagePolicy.EditMaxOutputCount,
	}
}

func transferBindingToWriteRequest(item upstreamAccountTransferBindingDTO) upstreamModelBindingWriteRequest {
	return upstreamModelBindingWriteRequest{
		ModelCode:                   item.ModelCode,
		CapabilityType:              item.CapabilityType,
		APIFormat:                   item.APIFormat,
		UpstreamModelName:           item.UpstreamModelName,
		Status:                      item.Status,
		ImageStreamMode:             item.ImageStreamMode,
		ImageEditTransport:          item.ImageEditTransport,
		ImageUpstreamResponseFormat: optionalString(item.ImageUpstreamResponseFormat),
		ImageMaxOutputCount:         optionalImageOutputCount(item.ImageMaxOutputCount),
		ImageEditMaxOutputCount:     optionalImageOutputCount(item.ImageEditMaxOutputCount),
	}
}

func optionalImageOutputCount(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func previewImportAccount(account upstreamAccountTransferAccountDTO, existingNames map[string]struct{}, seenNames map[string]struct{}) upstreamAccountImportPreviewItemDTO {
	name := strings.TrimSpace(account.Name)
	item := upstreamAccountImportPreviewItemDTO{
		Name:              name,
		BaseURL:           strings.TrimSpace(account.BaseURL),
		Action:            "create",
		ModelBindingCount: len(account.ModelBindings),
	}
	if name == "" || strings.TrimSpace(account.BaseURL) == "" || strings.TrimSpace(account.APIKey) == "" {
		item.Action = "error"
		item.Reason = "name, base_url and api_key are required"
		return item
	}
	if _, ok := existingNames[name]; ok {
		item.Action = "skip"
		item.Reason = "account name already exists"
		return item
	}
	if _, ok := seenNames[name]; ok {
		item.Action = "skip"
		item.Reason = "duplicate account name in import file"
		return item
	}
	seenNames[name] = struct{}{}
	duplicates := countDuplicateTransferBindings(account.ModelBindings)
	if duplicates > 0 {
		item.DuplicateModelBindings = duplicates
		item.Warnings = append(item.Warnings, fmt.Sprintf("%d duplicate model binding(s) will be skipped", duplicates))
	}
	return item
}

func accumulateImportPreview(summary *upstreamAccountImportSummaryDTO, item upstreamAccountImportPreviewItemDTO) {
	switch item.Action {
	case "create":
		summary.CreateAccounts++
		summary.CreateModelBindings += item.ModelBindingCount - item.DuplicateModelBindings
		summary.SkipModelBindings += item.DuplicateModelBindings
	case "skip":
		summary.SkipAccounts++
		summary.SkipModelBindings += item.ModelBindingCount
	default:
		summary.ErrorAccounts++
		summary.SkipModelBindings += item.ModelBindingCount
	}
}

func existingUpstreamAccountNames(ctx context.Context, d AIDeps) (map[string]struct{}, error) {
	accounts, err := d.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		out[account.Name] = struct{}{}
	}
	return out, nil
}

func validateImportPriceBook(ctx context.Context, d AIDeps, priceBookID string) error {
	priceBookID = strings.TrimSpace(priceBookID)
	if priceBookID == "" {
		return nil
	}
	if d.PriceBooks == nil {
		return httpx.ErrUnavailable.WithDetail("price book reader is not configured")
	}
	if _, err := d.PriceBooks.GetPriceBook(ctx, priceBookID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return httpx.ErrBadRequest.WithDetail("default_price_book_id does not exist")
		}
		return httpx.ErrBadRequest.WithDetail("invalid default_price_book_id")
	}
	return nil
}

func validateImportStrategies(req upstreamAccountImportRequest) error {
	accountStrategy := strings.TrimSpace(req.DuplicateAccountStrategy)
	if accountStrategy != "" && accountStrategy != "skip" {
		return httpx.ErrBadRequest.WithDetail("duplicate_account_strategy only supports skip")
	}
	bindingStrategy := strings.TrimSpace(req.DuplicateBindingStrategy)
	if bindingStrategy != "" && bindingStrategy != "skip" {
		return httpx.ErrBadRequest.WithDetail("duplicate_binding_strategy only supports skip")
	}
	return nil
}

func defaultTenantMultiplier(value *float64) float64 {
	if value == nil || *value <= 0 {
		return 1
	}
	return *value
}

func portableUpstreamAccountStatus(status string) string {
	if strings.TrimSpace(status) == domain.UpstreamAccountStatusActive {
		return domain.UpstreamAccountStatusActive
	}
	return domain.UpstreamAccountStatusDisabled
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// transferBindingDuplicateKey 用 (model_code, capability_type) 做去重键——这与
// ai_upstream_models 的唯一约束 (upstream_kind, upstream_id, model_code, capability_type)
// 语义一致：同一账号下一个 model_code+capability_type 只能有一条绑定，api_format 只是
// 这条绑定的属性而非独立识别键，不应参与去重判断。
func transferBindingDuplicateKey(item upstreamAccountTransferBindingDTO) string {
	return strings.Join([]string{
		strings.TrimSpace(item.ModelCode),
		strings.TrimSpace(item.CapabilityType),
		strings.TrimSpace(item.UpstreamModelName),
	}, "\x00")
}

func countDuplicateTransferBindings(items []upstreamAccountTransferBindingDTO) int {
	seen := map[string]struct{}{}
	duplicates := 0
	for _, item := range items {
		key := transferBindingDuplicateKey(item)
		if _, ok := seen[key]; ok {
			duplicates++
			continue
		}
		seen[key] = struct{}{}
	}
	return duplicates
}

func hasTransferModelBindings(accounts []upstreamAccountTransferAccountDTO) bool {
	for _, account := range accounts {
		if len(account.ModelBindings) > 0 {
			return true
		}
	}
	return false
}

func copyRawJSONOrObject(raw []byte) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage("{}")
	}
	return append(json.RawMessage(nil), raw...)
}

func normalizedRawJSONBytes(raw json.RawMessage) []byte {
	if len(raw) == 0 || !json.Valid(raw) || string(raw) == "null" {
		return []byte("{}")
	}
	return append([]byte(nil), raw...)
}

func voidAdminAudit(ctx context.Context, d AIDeps, action, objectType, objectID string, summary map[string]any, result string, httpStatus int32) {
	if d.AdminAudit == nil {
		return
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		raw = []byte("{}")
	}
	var status *int32
	if httpStatus > 0 {
		status = &httpStatus
	}
	_ = d.AdminAudit.Record(ctx, domain.AdminAuditEvent{
		Actor:          claimsUserID(ctx),
		Action:         action,
		ObjectType:     objectType,
		ObjectID:       objectID,
		RequestSummary: raw,
		Result:         result,
		HttpStatus:     status,
	})
}
