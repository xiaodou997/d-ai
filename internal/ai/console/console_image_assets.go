package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/asynctask"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageassets"
)

type consoleImageTaskAssetDTO struct {
	ID                  string `json:"id,omitempty"`
	Index               int    `json:"index,omitempty"`
	PreviewURL          string `json:"preview_url,omitempty"`
	DisplayURL          string `json:"display_url"`
	OriginalURL         string `json:"original_url,omitempty"`
	OriginalContentType string `json:"content_type,omitempty"`
	OriginalSizeBytes   int64  `json:"size_bytes,omitempty"`
	PreviewContentType  string `json:"preview_content_type,omitempty"`
	PreviewSizeBytes    int64  `json:"preview_size_bytes,omitempty"`
	Width               int    `json:"width,omitempty"`
	Height              int    `json:"height,omitempty"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
}

func (s *Console) handleConsoleImageTaskAsset(w http.ResponseWriter, r *http.Request) {
	if s.imageAssets == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "image asset service is not configured")
		return
	}
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	indexValue := strings.TrimSpace(chi.URLParam(r, "index"))
	variant := strings.TrimSpace(chi.URLParam(r, "variant"))
	if taskID == "" || indexValue == "" || (variant != "preview" && variant != "original") {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid image asset path")
		return
	}
	index, err := strconv.Atoi(indexValue)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid image index")
		return
	}
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}

	if s.asyncTasks == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "async task service is not configured")
		return
	}
	task, err := s.asyncTasks.Inspect(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, asynctask.ErrNotFound) {
			writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
			return
		}
		s.logger.Warn("runtime image: inspect task asset payload failed",
			consoleRequestLogFields(r, zap.Error(err), zap.String("task_id", taskID))...,
		)
		writeErr(w, http.StatusInternalServerError, BizErrDatabase, "database error")
		return
	}
	if (task.Type != consoleImageGenerationTaskType && task.Type != consoleImageEditTaskType) ||
		task.Status != domain.TaskCompleted ||
		!consoleImageTaskAssetKeyMatches(task.Output, index, variant, key) {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}

	asset, err := s.imageAssets.TaskAsset(taskID, index, variant)
	if err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
			return
		}
		s.logger.Warn("runtime image: resolve task asset failed",
			consoleRequestLogFields(r,
				zap.Error(err),
				zap.String("task_id", taskID),
				zap.Int("index", index),
				zap.String("variant", variant),
			)...,
		)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "read image asset failed")
		return
	}
	file, err := os.Open(asset.Path)
	if err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
			return
		}
		s.logger.Warn("runtime image: open task asset failed",
			consoleRequestLogFields(r, zap.Error(err), zap.String("path", asset.Path))...,
		)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "read image asset failed")
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "read image asset failed")
		return
	}
	contentType := asset.ContentType
	if variant == "original" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(asset.Path)+`"`)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, filepath.Base(asset.Path), stat.ModTime(), file)
}

func (s *Console) handleConsoleImageEphemeralAsset(w http.ResponseWriter, r *http.Request) {
	if s.imageAssets == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "image asset service is not configured")
		return
	}
	assetID := strings.TrimSpace(chi.URLParam(r, "assetID"))
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	asset, err := s.imageAssets.EphemeralAsset(assetID, key)
	if err != nil {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}
	file, err := os.Open(asset.Path)
	if err != nil {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "read image asset failed")
		return
	}
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, filepath.Base(asset.Path), stat.ModTime(), file)
}

func consoleImageTaskAssetKeyMatches(raw []byte, index int, variant, key string) bool {
	if len(raw) == 0 || key == "" {
		return false
	}
	assets := extractConsoleImageTaskAssets(raw)
	var matched *consoleImageTaskAssetDTO
	for idx := range assets {
		if assets[idx].Index == index {
			matched = &assets[idx]
			break
		}
	}
	if matched == nil && index >= 0 && index < len(assets) && assets[index].Index == 0 {
		matched = &assets[index]
	}
	if matched == nil {
		return false
	}
	var rawURL string
	if variant == "original" {
		rawURL = matched.OriginalURL
	} else {
		rawURL = matched.PreviewURL
		if rawURL == "" {
			rawURL = matched.DisplayURL
		}
	}
	return assetURLKey(rawURL) == key
}

func assetURLKey(rawURL string) string {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("key"))
}

func consoleStoredImageAssetsToDTO(stored []imageassets.StoredAsset) []consoleImageTaskAssetDTO {
	if len(stored) == 0 {
		return nil
	}
	assets := make([]consoleImageTaskAssetDTO, 0, len(stored))
	for _, asset := range stored {
		assets = append(assets, consoleImageTaskAssetDTO{
			ID:                  asset.ID,
			Index:               asset.Index,
			PreviewURL:          asset.PreviewURL,
			DisplayURL:          asset.DisplayURL,
			OriginalURL:         asset.OriginalURL,
			OriginalContentType: asset.OriginalContentType,
			OriginalSizeBytes:   asset.OriginalSizeBytes,
			PreviewContentType:  asset.PreviewContentType,
			PreviewSizeBytes:    asset.PreviewSizeBytes,
			Width:               asset.Width,
			Height:              asset.Height,
			ExpiresAt:           asset.ExpiresAt.UnixMilli(),
		})
	}
	return assets
}

func extractConsoleImageTaskAssets(raw []byte) []consoleImageTaskAssetDTO {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Assets []consoleImageTaskAssetDTO `json:"assets"`
		Data   []struct {
			AssetRef    string `json:"asset_ref"`
			PreviewURL  string `json:"preview_url"`
			DisplayURL  string `json:"display_url"`
			URL         string `json:"url"`
			OriginalURL string `json:"original_url"`
			ExpiresAt   string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	if len(payload.Assets) > 0 {
		return sanitizeConsoleImageTaskAssets(payload.Assets)
	}
	assets := make([]consoleImageTaskAssetDTO, 0, len(payload.Data))
	for index, item := range payload.Data {
		previewURL, displayURL, originalURL := imageassets.NormalizeAssetURLs(
			item.PreviewURL,
			firstNonEmptyImageURL(item.DisplayURL, item.URL),
			item.OriginalURL,
		)
		if previewURL == "" && displayURL == "" && originalURL == "" {
			continue
		}
		asset := consoleImageTaskAssetDTO{
			ID:          item.AssetRef,
			Index:       index,
			PreviewURL:  previewURL,
			DisplayURL:  displayURL,
			OriginalURL: originalURL,
		}
		if expiresAt, err := time.Parse(time.RFC3339, item.ExpiresAt); err == nil {
			asset.ExpiresAt = expiresAt.UnixMilli()
		}
		assets = append(assets, asset)
	}
	return assets
}

func sanitizeConsoleImageTaskAssets(items []consoleImageTaskAssetDTO) []consoleImageTaskAssetDTO {
	if len(items) == 0 {
		return nil
	}
	out := make([]consoleImageTaskAssetDTO, 0, len(items))
	for _, item := range items {
		item.PreviewURL, item.DisplayURL, item.OriginalURL = imageassets.NormalizeAssetURLs(
			item.PreviewURL,
			item.DisplayURL,
			item.OriginalURL,
		)
		if item.PreviewURL == "" && item.DisplayURL == "" && item.OriginalURL == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func firstNonEmptyImageURL(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Console) refreshConsoleImageAssetURLs(ctx context.Context, assets []consoleImageTaskAssetDTO) {
	if s.fileStore == nil {
		return
	}
	for index := range assets {
		asset := &assets[index]
		if !strings.HasPrefix(asset.ID, "media://") {
			continue
		}
		link, err := s.fileStore.IssueURL(ctx, asset.ID)
		if err != nil {
			// An expired platform asset must not leave a stale capability URL in
			// the response. External upstream URLs are never changed here.
			asset.PreviewURL = ""
			asset.DisplayURL = ""
			asset.OriginalURL = ""
			asset.ExpiresAt = 0
			continue
		}
		asset.PreviewURL = link.URL
		asset.DisplayURL = link.URL
		asset.OriginalURL = link.URL
		asset.ExpiresAt = link.ExpiresAt.UnixMilli()
	}
}
