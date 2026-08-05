package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/libs/go/httpx"
)

type groupExportRequest struct {
	GroupIDs []string `json:"group_ids" doc:"要导出的分组 ID 列表"`
}

type groupExportInput struct{ Body groupExportRequest }
type groupExportOutput struct {
	Body commercial.GroupTransferBundle
}
type groupImportPreviewInput struct{ Body commercial.GroupImportRequest }
type groupImportPreviewOutput struct{ Body commercial.GroupImportPreview }
type groupImportInput struct{ Body commercial.GroupImportRequest }
type groupImportOutput struct{ Body commercial.GroupImportResult }

func registerGroupTransfer(api huma.API, d AIDeps) {
	ready := func() error {
		if d.GroupTransferSvc == nil {
			return httpx.ErrUnavailable.WithDetail("group transfer service is not configured")
		}
		return nil
	}
	huma.Register(api, huma.Operation{
		OperationID: "ai-export-groups",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/me/groups/export",
		Summary:     "导出分组配置",
		Description: "导出分组基础信息、API 入口策略和请求模型调度规则；不包含价格表、上游目标和访问授权。",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, in *groupExportInput) (*groupExportOutput, error) {
		if err := ready(); err != nil {
			return nil, err
		}
		tenantID := tenantIDFromContext(ctx)
		bundle, err := d.GroupTransferSvc.Export(ctx, tenantID, in.Body.GroupIDs)
		if err != nil {
			voidAdminAudit(ctx, d, "groups.export", "group_config_bundle", "", map[string]any{
				"group_ids": in.Body.GroupIDs,
			}, "failed", 500)
			return nil, mapServiceError(err)
		}
		names := make([]string, 0, len(bundle.Groups))
		for _, group := range bundle.Groups {
			names = append(names, group.Name)
		}
		voidAdminAudit(ctx, d, "groups.export", "group_config_bundle", bundle.BundleID, map[string]any{
			"group_count": len(bundle.Groups),
			"group_names": names,
		}, "success", 200)
		return &groupExportOutput{Body: bundle}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:  "ai-preview-import-groups",
		Method:       http.MethodPost,
		Path:         "/api/v1/tenants/me/groups/import/preview",
		MaxBodyBytes: 8 << 20,
		Summary:      "预检导入分组配置",
		Description:  "校验导入文件、冲突动作、价格表和调度模型，并返回实际应用计划。",
		Tags:         []string{"groups"},
	}, func(ctx context.Context, in *groupImportPreviewInput) (*groupImportPreviewOutput, error) {
		if err := ready(); err != nil {
			return nil, err
		}
		preview, err := d.GroupTransferSvc.Preview(ctx, tenantIDFromContext(ctx), in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &groupImportPreviewOutput{Body: preview}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:  "ai-import-groups",
		Method:       http.MethodPost,
		Path:         "/api/v1/tenants/me/groups/import",
		MaxBodyBytes: 8 << 20,
		Summary:      "导入分组配置",
		Description:  "按预检计划逐组事务导入；新建分组强制停用，更新分组保留上游目标和访问授权。",
		Tags:         []string{"groups"},
	}, func(ctx context.Context, in *groupImportInput) (*groupImportOutput, error) {
		if err := ready(); err != nil {
			return nil, err
		}
		result, err := d.GroupTransferSvc.Import(ctx, tenantIDFromContext(ctx), in.Body)
		if err != nil {
			voidAdminAudit(ctx, d, "groups.import", "group_config_bundle", in.Body.Bundle.BundleID, groupImportAuditSummary(in.Body, commercial.GroupImportResult{}), "failed", 500)
			return nil, mapServiceError(err)
		}
		auditResult := "success"
		if result.Summary.Success == 0 && result.Summary.Error > 0 {
			auditResult = "failed"
		}
		voidAdminAudit(ctx, d, "groups.import", "group_config_bundle", in.Body.Bundle.BundleID, groupImportAuditSummary(in.Body, result), auditResult, 200)
		return &groupImportOutput{Body: result}, nil
	})
}

func groupImportAuditSummary(req commercial.GroupImportRequest, result commercial.GroupImportResult) map[string]any {
	groups := make([]map[string]string, 0, len(req.Choices))
	for _, choice := range req.Choices {
		groups = append(groups, map[string]string{
			"source_name":   choice.SourceName,
			"action":        string(choice.Action),
			"target_name":   choice.TargetName,
			"price_book_id": choice.PriceBookID,
		})
	}
	return map[string]any{
		"bundle_id":     req.Bundle.BundleID,
		"groups":        groups,
		"success_count": result.Summary.Success,
		"skip_count":    result.Summary.Skip,
		"error_count":   result.Summary.Error,
	}
}
