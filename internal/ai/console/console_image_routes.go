package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/workspace"
)

type consoleImageModelDTO struct {
	GroupID                 string  `json:"group_id"`
	GroupName               string  `json:"group_name"`
	EffectiveUserMultiplier float64 `json:"effective_user_multiplier"`
	BillingGroupLabel       string  `json:"billing_group_label"`
	ModelCode               string  `json:"model_code"`
	CapabilityType          string  `json:"capability_type"`
	Status                  string  `json:"status"`
	MaxOutputCount          int     `json:"max_output_count"`
	EditMaxOutputCount      int     `json:"edit_max_output_count"`
}

type consoleImageJobDTO struct {
	ID                   string                     `json:"id"`
	Operation            string                     `json:"operation"`
	GroupID              string                     `json:"group_id,omitempty"`
	ModelCode            string                     `json:"model_code"`
	Prompt               string                     `json:"prompt"`
	RetryPrompt          string                     `json:"retry_prompt,omitempty"`
	Status               string                     `json:"status"`
	StoragePolicy        string                     `json:"storage_policy"`
	RawImageRetained     bool                       `json:"raw_image_retained"`
	Size                 string                     `json:"size,omitempty"`
	Quality              string                     `json:"quality,omitempty"`
	Style                string                     `json:"style,omitempty"`
	ResponseFormat       string                     `json:"response_format,omitempty"`
	RequestedOutputCount int                        `json:"requested_output_count"`
	CallerChargeUSD      float64                    `json:"caller_charge_usd"`
	ImageCount           int                        `json:"image_count"`
	InlineCount          int                        `json:"inline_count"`
	URLCount             int                        `json:"url_count"`
	RevisedPrompts       []string                   `json:"revised_prompts,omitempty"`
	Assets               []consoleImageTaskAssetDTO `json:"assets,omitempty"`
	ErrorMessage         string                     `json:"error_message,omitempty"`
	CreatedAt            int64                      `json:"created_at"`
	CompletedAt          *int64                     `json:"completed_at,omitempty"`
}

type consoleImageModelRow struct {
	GroupID                 string
	GroupName               string
	EffectiveUserMultiplier float64
	BillingGroupLabel       string
	ModelCode               string
	CapabilityType          string
	MaxOutputCount          int
	EditMaxOutputCount      int
}

func (s *Console) handleConsoleImageModels(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	models, err := s.consoleGrantedImageModels(r, subject)
	if err != nil {
		s.logger.Error("runtime image models: list grants failed",
			consoleSubjectLogFields(r, subject, zap.Error(err))...,
		)
		writeDBErr(w, err)
		return
	}
	out := make([]consoleImageModelDTO, 0, len(models))
	for _, model := range models {
		out = append(out, consoleImageModelDTO{
			GroupID:                 model.GroupID,
			GroupName:               model.GroupName,
			EffectiveUserMultiplier: model.EffectiveUserMultiplier,
			BillingGroupLabel:       model.BillingGroupLabel,
			ModelCode:               model.ModelCode,
			CapabilityType:          model.CapabilityType,
			Status:                  "available",
			MaxOutputCount:          model.MaxOutputCount,
			EditMaxOutputCount:      model.EditMaxOutputCount,
		})
	}
	writeOK(w, out)
}

func (s *Console) handleConsoleImageListJobs(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.workspaceSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "workspace service is not configured")
		return
	}
	jobs, err := s.workspaceSvc.ListImageJobs(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	}, 50)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	items := make([]consoleImageJobDTO, 0, len(jobs))
	for _, job := range jobs {
		item := workspaceImageJobToConsoleDTO(job)
		s.refreshConsoleImageAssetURLs(r.Context(), item.Assets)
		items = append(items, item)
	}
	writeOK(w, items)
}

func writeConsoleImagePrepareErr(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeErr(w, http.StatusForbidden, BizErrForbidden, "model is not authorized")
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
	default:
		var validation *domain.ValidationError
		if errors.As(err, &validation) {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, validation.Message)
			return
		}
		writeDBErr(w, err)
	}
}

func (s *Console) consoleGrantedImageModels(r *http.Request, subject *coreidentity.Subject) ([]consoleImageModelRow, error) {
	groups, err := s.grantChecker.AccessibleGroupIDsForSubject(r.Context(), subject)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []consoleImageModelRow{}, nil
	}
	rows, err := s.postgres.Query(r.Context(), `
		SELECT
		       g.id::text,
		       g.name,
		       COALESCE(ug.user_multiplier_override, g.default_user_multiplier)::float8 AS effective_user_multiplier,
		       um.model_code,
		       um.capability_type,
		       MIN(COALESCE((um.config_json->'image_generation'->>'max_output_count')::int, 1)),
		       MIN(COALESCE((um.config_json->'image_generation'->>'edit_max_output_count')::int, 1))
		FROM ai_group_targets gt
		JOIN ai_groups g
		  ON g.id = gt.group_id AND g.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		 AND um.capability_type = 'image'
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		LEFT JOIN ai_user_groups ug
		  ON ug.group_id = g.id AND ug.tenant_id = $2 AND ug.user_id = $3
		WHERE gt.group_id = ANY($1::uuid[])
		  AND g.tenant_id = $2
		  AND gt.status = 'active'
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		GROUP BY g.id,
		         g.name,
		         COALESCE(ug.user_multiplier_override, g.default_user_multiplier),
		         um.model_code,
		         um.capability_type
		ORDER BY array_position($1::uuid[], g.id), um.model_code ASC`, groups, subject.TenantID, subject.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := make([]consoleImageModelRow, 0)
	for rows.Next() {
		var model consoleImageModelRow
		if err := rows.Scan(&model.GroupID, &model.GroupName, &model.EffectiveUserMultiplier, &model.ModelCode, &model.CapabilityType, &model.MaxOutputCount, &model.EditMaxOutputCount); err != nil {
			return nil, err
		}
		model.BillingGroupLabel = consoleImageBillingGroupLabel(model.GroupName, model.EffectiveUserMultiplier)
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return models, nil
	}
	filtered := make([]consoleImageModelRow, 0, len(models))
	for _, model := range models {
		ok, err := s.routeInspector.ModelSupportsClientProtocolInGroups(r.Context(), model.ModelCode, domain.CapabilityImage, []string{model.GroupID}, domain.ProtocolOpenAIImages, false, true)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered, nil
}

func consoleImageBillingGroupLabel(groupName string, multiplier float64) string {
	return fmt.Sprintf("%s · %.4gx", groupName, multiplier)
}

func workspaceImageJobToConsoleDTO(job workspace.ImageJob) consoleImageJobDTO {
	dto := consoleImageJobDTO{
		ID:                   job.ID,
		Operation:            job.Operation,
		ModelCode:            job.ModelCode,
		Prompt:               job.Prompt,
		Status:               job.Status,
		StoragePolicy:        job.StoragePolicy,
		RawImageRetained:     job.RawImageRetained,
		Size:                 job.Size,
		Quality:              job.Quality,
		Style:                job.Style,
		ResponseFormat:       job.ResponseFormat,
		RequestedOutputCount: job.RequestedOutputCount,
		CallerChargeUSD:      domain.MicroToUSD(job.CallerChargeMicro),
		ImageCount:           job.ImageCount,
		InlineCount:          job.InlineCount,
		URLCount:             job.URLCount,
		RevisedPrompts:       job.RevisedPrompts,
		Assets:               workspaceImageAssetsToConsoleDTO(job.Assets),
		ErrorMessage:         job.ErrorMessage,
		CreatedAt:            job.CreatedAt.UnixMilli(),
	}
	if job.CompletedAt != nil {
		completedAt := job.CompletedAt.UnixMilli()
		dto.CompletedAt = &completedAt
	}
	return dto
}

func workspaceImageAssetsToConsoleDTO(items []workspace.ImageAsset) []consoleImageTaskAssetDTO {
	if len(items) == 0 {
		return nil
	}
	out := make([]consoleImageTaskAssetDTO, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.PreviewURL) == "" && strings.TrimSpace(item.DisplayURL) == "" && strings.TrimSpace(item.OriginalURL) == "" {
			continue
		}
		out = append(out, consoleImageTaskAssetDTO{
			ID:                  item.ID,
			Index:               item.Index,
			PreviewURL:          item.PreviewURL,
			DisplayURL:          item.DisplayURL,
			OriginalURL:         item.OriginalURL,
			OriginalContentType: item.OriginalContentType,
			OriginalSizeBytes:   item.OriginalSizeBytes,
			PreviewContentType:  item.PreviewContentType,
			PreviewSizeBytes:    item.PreviewSizeBytes,
			Width:               item.Width,
			Height:              item.Height,
			ExpiresAt:           item.ExpiresAt,
		})
	}
	return out
}

func millisFromEpochSecondsPtr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v * 1000
	return &out
}

func decodeStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func decodeJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyConsoleRuntimeHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func parsePositiveInt(raw string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
