// Package filestore owns short-lived platform files and their public capability URLs.
package filestore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/imagepayload"
	"xiaodou/dai/internal/weborigin"
)

const (
	mediaScheme          = "media://"
	defaultMaxSize       = 32 << 20
	fileCleanupLeaseTTL  = 2 * time.Minute
	fileCleanupHeartbeat = fileCleanupLeaseTTL / 3
)

type Config struct {
	StorageDir string
	AssetTTL   time.Duration
	URLTTL     time.Duration
	MaxBytes   int64
}

type Asset struct {
	Ref         string
	ContentType string
	SizeBytes   int64
	ExpiresAt   time.Time
}

type Link struct {
	URL       string
	ExpiresAt time.Time
}

type CleanupResult struct {
	DeletedAssets int
	DeletedLinks  int
	DeletedFiles  int
}

var ErrCleanupLeaseLost = errors.New("filestore: cleanup lease was lost")

type storedAsset struct {
	id                string
	storageKey        string
	contentType       string
	sizeBytes         int64
	createdAt         time.Time
	expiresAt         time.Time
	cleanupOwner      string
	cleanupLeaseUntil time.Time
}

type repository interface {
	createAsset(context.Context, storedAsset) error
	assetByID(context.Context, string) (storedAsset, error)
	createLink(context.Context, []byte, string, time.Time) error
	assetByLink(context.Context, []byte, time.Time) (storedAsset, error)
	claimExpired(context.Context, time.Time, time.Time, int, string) ([]storedAsset, int, error)
	renewExpired(context.Context, []string, string, time.Time) (bool, error)
	releaseExpired(context.Context, []string, string) error
	finalizeExpired(context.Context, []string, string) (int, int, error)
}

type Service struct {
	repo              repository
	storageDir        string
	assetTTL          time.Duration
	urlTTL            time.Duration
	maxBytes          int64
	ownerID           string
	leaseTTL          time.Duration
	heartbeatInterval time.Duration
}

func New(pool *pgxpool.Pool, cfg Config) (*Service, error) {
	if pool == nil {
		return nil, errors.New("filestore: postgres is required")
	}
	return newService(&postgresRepository{pool: pool}, cfg)
}

func newService(repo repository, cfg Config) (*Service, error) {
	if repo == nil {
		return nil, errors.New("filestore: repository is required")
	}
	storageDir := strings.TrimSpace(cfg.StorageDir)
	if storageDir == "" {
		storageDir = "files"
	}
	if err := os.MkdirAll(storageDir, 0o700); err != nil {
		return nil, fmt.Errorf("filestore: create storage directory: %w", err)
	}
	assetTTL := cfg.AssetTTL
	if assetTTL <= 0 {
		assetTTL = 24 * time.Hour
	}
	urlTTL := cfg.URLTTL
	if urlTTL <= 0 {
		urlTTL = 24 * time.Hour
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxSize
	}
	return &Service{
		repo:              repo,
		storageDir:        storageDir,
		assetTTL:          assetTTL,
		urlTTL:            urlTTL,
		maxBytes:          maxBytes,
		ownerID:           uuid.NewString(),
		leaseTTL:          fileCleanupLeaseTTL,
		heartbeatInterval: fileCleanupHeartbeat,
	}, nil
}

func (s *Service) Publish(ctx context.Context, content io.Reader, contentType string) (Asset, error) {
	if s == nil {
		return Asset{}, errors.New("filestore: service is not configured")
	}
	if content == nil {
		return Asset{}, errors.New("filestore: content is required")
	}

	id := uuid.NewString()
	temp, err := os.CreateTemp(s.storageDir, ".upload-*")
	if err != nil {
		return Asset{}, fmt.Errorf("filestore: create temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	defer temp.Close()

	written, err := io.Copy(temp, io.LimitReader(content, s.maxBytes+1))
	if err != nil {
		return Asset{}, fmt.Errorf("filestore: write content: %w", err)
	}
	if written == 0 {
		return Asset{}, errors.New("filestore: content is empty")
	}
	if written > s.maxBytes {
		return Asset{}, fmt.Errorf("filestore: content exceeds %d byte limit", s.maxBytes)
	}
	if err := temp.Chmod(0o600); err != nil {
		return Asset{}, fmt.Errorf("filestore: protect content: %w", err)
	}
	if err := temp.Close(); err != nil {
		return Asset{}, fmt.Errorf("filestore: close content: %w", err)
	}

	if contentType = normalizeContentType(contentType, tempPath); contentType == "" {
		return Asset{}, errors.New("filestore: content type is required")
	}
	storageKey := id
	path := s.pathFor(storageKey)
	if err := os.Rename(tempPath, path); err != nil {
		return Asset{}, fmt.Errorf("filestore: commit content: %w", err)
	}

	now := time.Now().UTC()
	stored := storedAsset{
		id:          id,
		storageKey:  storageKey,
		contentType: contentType,
		sizeBytes:   written,
		createdAt:   now,
		expiresAt:   now.Add(s.assetTTL),
	}
	if err := s.repo.createAsset(ctx, stored); err != nil {
		_ = os.Remove(path)
		return Asset{}, fmt.Errorf("filestore: create asset: %w", err)
	}
	return publicAsset(stored), nil
}

func (s *Service) PublishBytes(ctx context.Context, data []byte, contentType string) (Asset, error) {
	return s.Publish(ctx, bytes.NewReader(data), contentType)
}

func (s *Service) IssueURL(ctx context.Context, ref string) (Link, error) {
	if s == nil {
		return Link{}, errors.New("filestore: service is not configured")
	}
	id, err := assetID(ref)
	if err != nil {
		return Link{}, err
	}
	asset, err := s.repo.assetByID(ctx, id)
	if err != nil {
		return Link{}, fmt.Errorf("filestore: find asset: %w", err)
	}
	now := time.Now().UTC()
	if !asset.expiresAt.After(now) {
		return Link{}, errors.New("filestore: asset has expired")
	}
	expiresAt := now.Add(s.urlTTL)
	if asset.expiresAt.Before(expiresAt) {
		expiresAt = asset.expiresAt
	}
	token, err := randomToken()
	if err != nil {
		return Link{}, err
	}
	if err := s.repo.createLink(ctx, tokenHash(token), asset.id, expiresAt); err != nil {
		return Link{}, fmt.Errorf("filestore: create access link: %w", err)
	}
	return Link{
		URL:       weborigin.Resolve(ctx, "/v1/files/content/"+url.PathEscape(token)),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/v1/files/content/{token}", s.handleContent)
}

func (s *Service) handleContent(w http.ResponseWriter, r *http.Request) {
	if s == nil {
		http.NotFound(w, r)
		return
	}
	asset, err := s.repo.assetByLink(r.Context(), tokenHash(chi.URLParam(r, "token")), time.Now().UTC())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(s.pathFor(asset.storageKey))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, asset.id, info.ModTime(), file)
}

func (s *Service) CleanupExpired(ctx context.Context, limit int) (CleanupResult, error) {
	if s == nil {
		return CleanupResult{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	now := time.Now().UTC()
	claims, deletedLinks, err := s.repo.claimExpired(ctx, now, now.Add(s.leaseTTL), limit, s.ownerID)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("filestore: claim expired metadata: %w", err)
	}
	result := CleanupResult{DeletedLinks: deletedLinks}
	ids := make([]string, 0, len(claims))
	for _, asset := range claims {
		ids = append(ids, asset.id)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaseEvents := s.monitorCleanupLease(workCtx, cancel, ids)
	for _, asset := range claims {
		if err := workCtx.Err(); err != nil {
			_ = s.repo.releaseExpired(context.WithoutCancel(ctx), ids, s.ownerID)
			leaseErr := <-leaseEvents
			if leaseErr != nil {
				err = errors.Join(err, leaseErr)
			}
			return result, err
		}
		if err := os.Remove(s.pathFor(asset.storageKey)); err == nil || errors.Is(err, os.ErrNotExist) {
			result.DeletedFiles++
		} else {
			cancel()
			leaseErr := <-leaseEvents
			_ = s.repo.releaseExpired(context.WithoutCancel(ctx), ids, s.ownerID)
			cleanupErr := fmt.Errorf("filestore: delete expired content: %w", err)
			if leaseErr != nil {
				cleanupErr = errors.Join(cleanupErr, leaseErr)
			}
			return result, cleanupErr
		}
	}
	cancel()
	if leaseErr := <-leaseEvents; leaseErr != nil {
		_ = s.repo.releaseExpired(context.WithoutCancel(ctx), ids, s.ownerID)
		return result, leaseErr
	}
	deletedAssets, finalizedLinks, err := s.repo.finalizeExpired(context.WithoutCancel(ctx), ids, s.ownerID)
	if err != nil {
		_ = s.repo.releaseExpired(context.WithoutCancel(ctx), ids, s.ownerID)
		return result, fmt.Errorf("filestore: finalize expired metadata: %w", err)
	}
	if deletedAssets != len(claims) {
		return result, ErrCleanupLeaseLost
	}
	result.DeletedAssets = deletedAssets
	result.DeletedLinks += finalizedLinks
	return result, nil
}

func (s *Service) monitorCleanupLease(ctx context.Context, cancel context.CancelFunc, ids []string) <-chan error {
	events := make(chan error, 1)
	if len(ids) == 0 {
		close(events)
		return events
	}
	interval := s.heartbeatInterval
	if interval <= 0 {
		interval = fileCleanupHeartbeat
	}
	go func() {
		defer close(events)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				beatCtx, beatCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				held, err := s.repo.renewExpired(beatCtx, ids, s.ownerID, time.Now().UTC().Add(s.leaseTTL))
				beatCancel()
				if err != nil {
					events <- fmt.Errorf("filestore: cleanup lease heartbeat failed: %w", err)
					cancel()
					return
				}
				if !held {
					events <- ErrCleanupLeaseLost
					cancel()
					return
				}
			}
		}
	}()
	return events
}

// NormalizeImageResponse fulfills serving.ImageResponseNormalizer. Existing
// upstream HTTP URLs are left untouched; only inline image data is published.
func (s *Service) NormalizeImageResponse(ctx context.Context, body []byte, responseFormat string) ([]byte, error) {
	responseFormat = strings.TrimSpace(responseFormat)
	if responseFormat != "url" && responseFormat != "b64_json" {
		return body, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse image response: %w", err)
	}
	dataRaw, ok := payload["data"]
	if !ok {
		return nil, errors.New("image response contains no data")
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &items); err != nil || len(items) == 0 {
		return nil, errors.New("image response contains no images")
	}
	for _, item := range items {
		b64 := jsonString(item["b64_json"])
		imageURL := jsonString(item["url"])
		if responseFormat == "url" && isHTTPURL(imageURL) {
			delete(item, "b64_json")
			continue
		}
		data, contentType, err := imageBytes(ctx, b64, imageURL)
		if err != nil {
			return nil, err
		}
		if responseFormat == "b64_json" {
			setJSONString(item, "b64_json", base64.StdEncoding.EncodeToString(data))
			delete(item, "url")
			delete(item, "asset_ref")
			continue
		}
		asset, err := s.PublishBytes(ctx, data, contentType)
		if err != nil {
			return nil, err
		}
		link, err := s.IssueURL(ctx, asset.Ref)
		if err != nil {
			return nil, err
		}
		setJSONString(item, "url", link.URL)
		setJSONString(item, "asset_ref", asset.Ref)
		setJSONTime(item, "expires_at", link.ExpiresAt)
		delete(item, "b64_json")
	}
	rewritten, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	payload["data"] = rewritten
	return json.Marshal(payload)
}

func (s *Service) pathFor(storageKey string) string {
	return filepath.Join(s.storageDir, storageKey)
}

func publicAsset(asset storedAsset) Asset {
	return Asset{
		Ref:         mediaScheme + asset.id,
		ContentType: asset.contentType,
		SizeBytes:   asset.sizeBytes,
		ExpiresAt:   asset.expiresAt,
	}
}

func assetID(ref string) (string, error) {
	id := strings.TrimSpace(strings.TrimPrefix(ref, mediaScheme))
	if !strings.HasPrefix(ref, mediaScheme) || id == "" {
		return "", errors.New("filestore: invalid asset reference")
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", errors.New("filestore: invalid asset reference")
	}
	return id, nil
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("filestore: generate access token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func normalizeContentType(value, path string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "" {
		if parsed, _, err := mime.ParseMediaType(value); err == nil {
			return parsed
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	return http.DetectContentType(buf[:n])
}

func jsonString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return strings.TrimSpace(value)
}

func setJSONString(item map[string]json.RawMessage, key, value string) {
	raw, _ := json.Marshal(value)
	item[key] = raw
}

func setJSONTime(item map[string]json.RawMessage, key string, value time.Time) {
	raw, _ := json.Marshal(value.UTC().Format(time.RFC3339))
	item[key] = raw
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func imageBytes(ctx context.Context, b64, rawURL string) ([]byte, string, error) {
	if b64 != "" {
		return imagepayload.DecodeBase64ImageBytes(b64, "")
	}
	if data, contentType, handled, err := imagepayload.DecodeInlineImageValue(rawURL); handled {
		return data, contentType, err
	}
	if !isHTTPURL(rawURL) {
		return nil, "", errors.New("image response contains neither inline data nor an HTTP URL")
	}
	return imageassets.DownloadExternalImage(ctx, rawURL)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func (r *postgresRepository) createAsset(ctx context.Context, asset storedAsset) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO file_assets (id, storage_key, content_type, size_bytes, created_at, expires_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
	`, asset.id, asset.storageKey, asset.contentType, asset.sizeBytes, asset.createdAt, asset.expiresAt)
	return err
}

func (r *postgresRepository) assetByID(ctx context.Context, id string) (storedAsset, error) {
	var asset storedAsset
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, storage_key, content_type, size_bytes, created_at, expires_at
		FROM file_assets
		WHERE id = $1::uuid
	`, id).Scan(&asset.id, &asset.storageKey, &asset.contentType, &asset.sizeBytes, &asset.createdAt, &asset.expiresAt)
	return asset, err
}

func (r *postgresRepository) createLink(ctx context.Context, hash []byte, assetID string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO file_access_links (token_hash, asset_id, expires_at)
		VALUES ($1, $2::uuid, $3)
	`, hash, assetID, expiresAt)
	return err
}

func (r *postgresRepository) assetByLink(ctx context.Context, hash []byte, now time.Time) (storedAsset, error) {
	var asset storedAsset
	err := r.pool.QueryRow(ctx, `
		SELECT a.id::text, a.storage_key, a.content_type, a.size_bytes, a.created_at, a.expires_at
		FROM file_access_links l
		JOIN file_assets a ON a.id = l.asset_id
		WHERE l.token_hash = $1
		  AND l.expires_at > $2
		  AND a.expires_at > $2
	`, hash, now).Scan(&asset.id, &asset.storageKey, &asset.contentType, &asset.sizeBytes, &asset.createdAt, &asset.expiresAt)
	return asset, err
}

func (r *postgresRepository) claimExpired(ctx context.Context, now, leaseUntil time.Time, limit int, owner string) ([]storedAsset, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)
	links, err := tx.Exec(ctx, `DELETE FROM file_access_links WHERE expires_at <= $1`, now)
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM file_assets
			WHERE expires_at <= $1
			  AND (cleanup_owner IS NULL OR cleanup_lease_until <= $1)
			ORDER BY expires_at ASC, id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE file_assets a
		SET cleanup_owner = $3, cleanup_lease_until = $4
		FROM candidates c
		WHERE a.id = c.id
		RETURNING a.id::text, a.storage_key, a.content_type, a.size_bytes, a.created_at, a.expires_at
	`, now, limit, owner, leaseUntil)
	if err != nil {
		return nil, 0, err
	}
	claims := make([]storedAsset, 0, limit)
	for rows.Next() {
		var asset storedAsset
		if err := rows.Scan(&asset.id, &asset.storageKey, &asset.contentType, &asset.sizeBytes, &asset.createdAt, &asset.expiresAt); err != nil {
			return nil, 0, err
		}
		claims = append(claims, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return claims, int(links.RowsAffected()), nil
}

func (r *postgresRepository) renewExpired(ctx context.Context, ids []string, owner string, leaseUntil time.Time) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	result, err := r.pool.Exec(ctx, `
		UPDATE file_assets
		SET cleanup_lease_until = $3
		WHERE id::text = ANY($1::text[])
		  AND cleanup_owner = $2
		  AND cleanup_lease_until > now()
	`, ids, owner, leaseUntil)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == int64(len(ids)), nil
}

func (r *postgresRepository) releaseExpired(ctx context.Context, ids []string, owner string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE file_assets
		SET cleanup_owner = NULL, cleanup_lease_until = NULL
		WHERE id::text = ANY($1::text[]) AND cleanup_owner = $2
	`, ids, owner)
	return err
}

func (r *postgresRepository) finalizeExpired(ctx context.Context, ids []string, owner string) (int, int, error) {
	if len(ids) == 0 {
		return 0, 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT id::text
		FROM file_assets
		WHERE id::text = ANY($1::text[]) AND cleanup_owner = $2
		FOR UPDATE
	`, ids, owner)
	if err != nil {
		return 0, 0, err
	}
	claimedIDs := make([]string, 0, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, err
		}
		claimedIDs = append(claimedIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()
	if len(claimedIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	}
	links, err := tx.Exec(ctx, `DELETE FROM file_access_links WHERE asset_id::text = ANY($1::text[])`, claimedIDs)
	if err != nil {
		return 0, 0, err
	}
	assets, err := tx.Exec(ctx, `
		DELETE FROM file_assets
		WHERE id::text = ANY($1::text[]) AND cleanup_owner = $2
	`, claimedIDs, owner)
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return int(assets.RowsAffected()), int(links.RowsAffected()), nil
}
