package imageassets

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gen2brain/webp"

	"xiaodou/dai/internal/weborigin"
)

const (
	displayContentType = "image/webp"
	defaultMaxImage    = 32 << 20
)

type Config struct {
	StorageDir     string
	Retention      time.Duration
	PublicBasePath string
}

type Service struct {
	httpClient     *http.Client
	storageDir     string
	retention      time.Duration
	publicBasePath string
	maxImageBytes  int64
}

type Owner struct {
	TenantID string
	UserID   string
}

type StoredAsset struct {
	ID                  string
	Index               int
	PreviewURL          string
	DisplayURL          string
	OriginalURL         string
	OriginalContentType string
	OriginalSizeBytes   int64
	PreviewContentType  string
	PreviewSizeBytes    int64
	Width               int
	Height              int
	ExpiresAt           time.Time
}

type TemporaryImage struct {
	Path        string
	ContentType string
	Filename    string
}

type Asset struct {
	Path        string
	ContentType string
}

type StoreResponseResult struct {
	Body   []byte
	Assets []StoredAsset
}

type CleanupResult struct {
	DeletedFiles int
}

type responseImageItem struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	OriginalURL   string `json:"original_url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func New(cfg Config, client *http.Client) *Service {
	storageDir := strings.TrimSpace(cfg.StorageDir)
	if storageDir == "" {
		storageDir = "images"
	}
	retention := cfg.Retention
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	basePath := strings.TrimRight(strings.TrimSpace(cfg.PublicBasePath), "/")
	if basePath == "" {
		basePath = "/runtime/v1/images/tasks"
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Service{
		httpClient:     client,
		storageDir:     storageDir,
		retention:      retention,
		publicBasePath: basePath,
		maxImageBytes:  defaultMaxImage,
	}
}

func (s *Service) StoreOpenAIImagesResponse(ctx context.Context, owner Owner, taskID string, body []byte) (StoreResponseResult, error) {
	if s == nil {
		return StoreResponseResult{Body: body}, nil
	}
	if len(body) == 0 || strings.TrimSpace(taskID) == "" || strings.TrimSpace(owner.TenantID) == "" {
		return StoreResponseResult{Body: body}, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return StoreResponseResult{Body: body}, nil
	}
	dataRaw, ok := raw["data"]
	if !ok {
		return StoreResponseResult{Body: body}, nil
	}
	var items []responseImageItem
	if err := json.Unmarshal(dataRaw, &items); err != nil {
		return StoreResponseResult{Body: body}, nil
	}
	if len(items) == 0 {
		return StoreResponseResult{Body: body}, nil
	}

	expiresAt := time.Now().Add(s.retention)
	assetKey, err := randomToken()
	if err != nil {
		return StoreResponseResult{Body: body}, err
	}
	assets := make([]StoredAsset, 0, len(items))
	for i := range items {
		data, contentType, err := s.imageBytes(ctx, items[i])
		if err != nil {
			return StoreResponseResult{Body: body}, err
		}
		if len(data) == 0 {
			continue
		}
		asset, err := s.storeOne(ctx, taskID, i, data, contentType, assetKey, expiresAt)
		if err != nil {
			return StoreResponseResult{Body: body}, err
		}
		items[i].B64JSON = ""
		items[i].URL = asset.PreviewURL
		items[i].OriginalURL = asset.OriginalURL
		assets = append(assets, asset)
	}
	if len(assets) == 0 {
		return StoreResponseResult{Body: body}, nil
	}

	rewrittenData, err := json.Marshal(items)
	if err != nil {
		return StoreResponseResult{}, err
	}
	raw["data"] = rewrittenData
	rewritten, err := json.Marshal(raw)
	if err != nil {
		return StoreResponseResult{}, err
	}
	return StoreResponseResult{Body: rewritten, Assets: assets}, nil
}

// NormalizeOpenAIResponse makes an OpenAI Images response satisfy the client
// response_format contract even when an upstream ignores the requested format.
// URL results stay untouched; inline image data is materialized as a short
// lived platform asset only when the caller requested a URL.
func (s *Service) NormalizeOpenAIResponse(ctx context.Context, body []byte, responseFormat string) ([]byte, error) {
	if s == nil || len(body) == 0 {
		return body, nil
	}
	responseFormat = strings.TrimSpace(responseFormat)
	if responseFormat != "b64_json" && responseFormat != "url" {
		return body, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse image response: %w", err)
	}
	dataRaw, ok := raw["data"]
	if !ok {
		return nil, errors.New("image response contains no data")
	}
	var items []responseImageItem
	if err := json.Unmarshal(dataRaw, &items); err != nil {
		return nil, fmt.Errorf("parse image response data: %w", err)
	}
	if len(items) == 0 {
		return nil, errors.New("image response contains no images")
	}

	for index := range items {
		item := &items[index]
		if responseFormat == "url" && isHTTPImageURL(item.URL) {
			item.B64JSON = ""
			continue
		}
		data, contentType, err := s.responseImageBytes(ctx, *item)
		if err != nil {
			return nil, fmt.Errorf("normalize image %d: %w", index, err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("normalize image %d: image is empty", index)
		}
		if responseFormat == "b64_json" {
			item.B64JSON = base64.StdEncoding.EncodeToString(data)
			item.URL = ""
			item.OriginalURL = ""
			continue
		}
		url, err := s.storeEphemeralImage(ctx, data, contentType)
		if err != nil {
			return nil, fmt.Errorf("store normalized image %d: %w", index, err)
		}
		item.B64JSON = ""
		item.URL = url
		item.OriginalURL = ""
	}

	rewrittenData, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	raw["data"] = rewrittenData
	return json.Marshal(raw)
}

// NormalizeImageResponse satisfies the serving image response normalization
// seam. The current public image surface is OpenAI Images.
func (s *Service) NormalizeImageResponse(ctx context.Context, body []byte, responseFormat string) ([]byte, error) {
	return s.NormalizeOpenAIResponse(ctx, body, responseFormat)
}

func (s *Service) responseImageBytes(ctx context.Context, item responseImageItem) ([]byte, string, error) {
	if strings.TrimSpace(item.B64JSON) != "" {
		return decodeBase64ImageBytes(item.B64JSON, "")
	}
	if data, contentType, handled, err := DecodeInlineImageValue(item.URL); handled {
		return data, contentType, err
	}
	if !isHTTPImageURL(item.URL) {
		return nil, "", errors.New("image is neither base64 nor an HTTP URL")
	}
	return s.downloadExternal(ctx, item.URL)
}

func isHTTPImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func (s *Service) storeEphemeralImage(ctx context.Context, data []byte, contentType string) (string, error) {
	if len(data) == 0 {
		return "", errors.New("image is empty")
	}
	if int64(len(data)) > s.maxImageBytes {
		return "", errors.New("image exceeds size limit")
	}
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	key, err := randomToken()
	if err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return "", errors.New("payload is not an image")
	}
	filename := safePath(id) + "_" + safePath(key) + extensionForContentType(contentType)
	if err := writeFile(filepath.Join(s.storageDir, "ephemeral", filename), data); err != nil {
		return "", err
	}
	return weborigin.Resolve(ctx, fmt.Sprintf("%s/%s?key=%s", s.publicEphemeralBasePath(), safePath(id), key)), nil
}

func (s *Service) publicEphemeralBasePath() string {
	if base := strings.TrimSuffix(s.publicBasePath, "/tasks"); base != s.publicBasePath {
		return base + "/assets"
	}
	return s.publicBasePath + "/assets"
}

// EphemeralAsset resolves a short-lived image URL created during response
// normalization. Both random URL components form the capability; no caller
// identity is required to dereference an image result URL.
func (s *Service) EphemeralAsset(id, key string) (Asset, error) {
	if s == nil {
		return Asset{}, errors.New("image asset service is not configured")
	}
	id = strings.TrimSpace(id)
	key = strings.TrimSpace(key)
	if id == "" || key == "" || safePath(id) != id || safePath(key) != key {
		return Asset{}, os.ErrNotExist
	}
	matches, err := filepath.Glob(filepath.Join(s.storageDir, "ephemeral", id+"_"+key+".*"))
	if err != nil || len(matches) != 1 {
		return Asset{}, os.ErrNotExist
	}
	info, err := os.Stat(matches[0])
	if err != nil {
		return Asset{}, err
	}
	if info.ModTime().Add(s.retention).Before(time.Now()) {
		_ = os.Remove(matches[0])
		return Asset{}, os.ErrNotExist
	}
	contentType := contentTypeForPath(matches[0])
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return Asset{Path: matches[0], ContentType: contentType}, nil
}

func (s *Service) DownloadTemporaryImage(ctx context.Context, owner Owner, taskID, role, rawURL string) (TemporaryImage, error) {
	if s == nil {
		return TemporaryImage{}, errors.New("image asset service is not configured")
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(owner.TenantID) == "" {
		return TemporaryImage{}, errors.New("tenant_id and task_id are required")
	}
	if err := validateImageDownloadURL(rawURL); err != nil {
		return TemporaryImage{}, err
	}
	data, contentType, err := s.downloadExternal(ctx, rawURL)
	if err != nil {
		return TemporaryImage{}, err
	}
	return s.StoreTemporaryImageBytes(ctx, owner, taskID, role, data, contentType)
}

func (s *Service) StoreTemporaryImageBytes(ctx context.Context, owner Owner, taskID, role string, data []byte, contentType string) (TemporaryImage, error) {
	if s == nil {
		return TemporaryImage{}, errors.New("image asset service is not configured")
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(owner.TenantID) == "" {
		return TemporaryImage{}, errors.New("tenant_id and task_id are required")
	}
	if len(data) == 0 {
		return TemporaryImage{}, errors.New("reference image is empty")
	}
	if int64(len(data)) > s.maxImageBytes {
		return TemporaryImage{}, errors.New("reference image exceeds maximum size")
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return TemporaryImage{}, fmt.Errorf("decode reference image: %w", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		return TemporaryImage{}, errors.New("reference image has invalid dimensions")
	}
	if normalized := normalizeContentType(contentType, format); normalized != "" {
		contentType = normalized
	}
	if contentType == "" {
		contentType = "image/png"
	}

	now := time.Now()
	relBase := filepath.ToSlash(filepath.Join(
		"_tmp",
		safePath(owner.TenantID),
		safePath(owner.UserID),
		now.Format("20060102"),
		safePath(taskID),
	))
	filename := strconv.FormatInt(now.UnixMilli(), 10) + "_" + safePath(role) + extensionForContentType(contentType)
	relPath := filepath.ToSlash(filepath.Join(relBase, filename))
	path := filepath.Join(s.storageDir, filepath.FromSlash(relPath))
	if err := writeFile(path, data); err != nil {
		return TemporaryImage{}, err
	}
	return TemporaryImage{
		Path:        path,
		ContentType: contentType,
		Filename:    filename,
	}, nil
}

func ValidateDownloadURL(rawURL string) error {
	return validateImageDownloadURL(rawURL)
}

// DownloadExternalImage resolves a caller-provided HTTP(S) image URL with the
// same SSRF, redirect, content-type, and size protections used for image
// assets. It is used when a binding requires file or Base64 input upstream.
func DownloadExternalImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	return New(Config{}, nil).downloadExternal(ctx, rawURL)
}

func (s *Service) TaskAsset(taskID string, index int, variant string) (Asset, error) {
	if s == nil {
		return Asset{}, errors.New("image asset service is not configured")
	}
	if strings.TrimSpace(taskID) == "" {
		return Asset{}, errors.New("task_id is required")
	}
	if index < 0 {
		return Asset{}, errors.New("image index is invalid")
	}
	taskDir := filepath.Join(s.storageDir, "tasks", safePath(taskID))
	switch variant {
	case "preview":
		path := filepath.Join(taskDir, strconv.Itoa(index)+"_preview.webp")
		return Asset{Path: path, ContentType: displayContentType}, nil
	case "original":
		matches, err := filepath.Glob(filepath.Join(taskDir, strconv.Itoa(index)+"_original.*"))
		if err != nil {
			return Asset{}, err
		}
		if len(matches) == 0 {
			return Asset{}, os.ErrNotExist
		}
		contentType := contentTypeForPath(matches[0])
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return Asset{Path: matches[0], ContentType: contentType}, nil
	default:
		return Asset{}, errors.New("variant must be preview or original")
	}
}

func (s *Service) DeleteTaskAssets(taskID string) (int, error) {
	if s == nil {
		return 0, nil
	}
	if strings.TrimSpace(taskID) == "" {
		return 0, errors.New("task_id is required")
	}
	return removeTaskDir(filepath.Join(s.storageDir, "tasks", safePath(taskID)))
}

func (s *Service) CleanupExpired() (CleanupResult, error) {
	if s == nil {
		return CleanupResult{}, nil
	}
	result := CleanupResult{}
	cutoff := time.Now().Add(-s.retention)
	deletedTemp, err := s.cleanupExpiredTemporaryImages(cutoff)
	result.DeletedFiles += deletedTemp
	if err != nil {
		return result, err
	}
	deletedEphemeral, err := s.cleanupExpiredEphemeralImages(cutoff)
	result.DeletedFiles += deletedEphemeral
	return result, err
}

func (s *Service) imageBytes(ctx context.Context, item responseImageItem) ([]byte, string, error) {
	if strings.TrimSpace(item.B64JSON) != "" {
		data, contentType, err := decodeBase64ImageBytes(item.B64JSON, "")
		if err != nil {
			return nil, "", fmt.Errorf("decode image b64_json: %w", err)
		}
		return data, contentType, nil
	}
	if strings.TrimSpace(item.URL) == "" {
		return nil, "", nil
	}
	if data, contentType, handled, err := DecodeInlineImageValue(item.URL); handled {
		if err != nil {
			return nil, "", fmt.Errorf("decode image url payload: %w", err)
		}
		return data, contentType, nil
	}
	return s.download(ctx, item.URL)
}

func (s *Service) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	return s.readImageResponse(resp)
}

func (s *Service) downloadExternal(ctx context.Context, rawURL string) ([]byte, string, error) {
	if err := validateImageDownloadURL(rawURL); err != nil {
		return nil, "", err
	}
	client := s.externalHTTPClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("image url has too many redirects")
		}
		return validateImageDownloadURL(req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateImageDownloadURL(resp.Request.URL.String()); err != nil {
			return nil, "", err
		}
	}
	return s.readImageResponse(resp)
}

func (s *Service) externalHTTPClient() *http.Client {
	base := s.httpClient
	if base == nil {
		base = &http.Client{Timeout: 60 * time.Second}
	}
	client := *base
	client.Transport = externalImageTransport(base.Transport)
	client.Jar = nil
	return &client
}

func externalImageTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if transport, ok := base.(*http.Transport); ok {
		cloned := transport.Clone()
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		cloned.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !isPublicImageDownloadAddr(ip.Unmap()) {
					return nil, errors.New("image url host resolved to a disallowed address")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		}
		return cloned
	}
	return base
}

func (s *Service) readImageResponse(resp *http.Response) ([]byte, string, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download image: upstream status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > s.maxImageBytes {
		return nil, "", errors.New("download image exceeds size limit")
	}
	contentType := resp.Header.Get("Content-Type")
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil && strings.HasPrefix(parsed, "image/") {
		contentType = parsed
	} else {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func (s *Service) cleanupExpiredTemporaryImages(cutoff time.Time) (int, error) {
	tmpRoot := filepath.Join(s.storageDir, "_tmp")
	if strings.TrimSpace(tmpRoot) == "" {
		return 0, nil
	}
	deleted := 0
	err := filepath.WalkDir(tmpRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		deleted++
		return nil
	})
	if os.IsNotExist(err) {
		return deleted, nil
	}
	if err != nil {
		return deleted, err
	}
	_ = removeEmptyDirs(tmpRoot, tmpRoot)
	return deleted, nil
}

func (s *Service) cleanupExpiredEphemeralImages(cutoff time.Time) (int, error) {
	root := filepath.Join(s.storageDir, "ephemeral")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return deleted, err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(root, entry.Name())); err != nil && !os.IsNotExist(err) {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func removeEmptyDirs(root, current string) error {
	entries, err := os.ReadDir(current)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := removeEmptyDirs(root, filepath.Join(current, entry.Name())); err != nil {
			return err
		}
	}
	if current != root {
		_ = os.Remove(current)
	}
	return nil
}

func removeTaskDir(path string) (int, error) {
	deleted := 0
	err := filepath.WalkDir(path, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if err := os.Remove(current); err != nil && !os.IsNotExist(err) {
			return err
		}
		deleted++
		return nil
	})
	if os.IsNotExist(err) {
		return deleted, nil
	}
	if err != nil {
		return deleted, err
	}
	_ = os.Remove(path)
	return deleted, nil
}

func validateImageDownloadURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errors.New("image url is required")
	}
	if len(rawURL) > 2048 {
		return errors.New("image url is too long")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid image url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("image url must use http or https")
	}
	if parsed.User != nil {
		return errors.New("image url must not include user info")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return errors.New("image url host is required")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return errors.New("image url host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		addr, ok := netip.AddrFromSlice(ip)
		if ok && !isPublicImageDownloadAddr(addr.Unmap()) {
			return errors.New("image url host is not allowed")
		}
	}
	return nil
}

func isPublicImageDownloadAddr(addr netip.Addr) bool {
	return addr.IsValid() &&
		!addr.IsLoopback() &&
		!addr.IsPrivate() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

func (s *Service) storeOne(ctx context.Context, taskID string, index int, data []byte, contentType, assetKey string, expiresAt time.Time) (StoredAsset, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return StoredAsset{}, fmt.Errorf("decode generated image: %w", err)
	}
	if normalized := normalizeContentType(contentType, format); normalized != "" {
		contentType = normalized
	}
	if contentType == "" {
		contentType = "image/png"
	}

	var display bytes.Buffer
	// Method 零值 0 是最快/压缩最差档，显式取回 libwebp 默认档 4，保持与旧 cgo 实现一致的体积/质量。
	if err := webp.Encode(&display, img, webp.Options{Quality: 82, Method: webp.DefaultMethod}); err != nil {
		return StoredAsset{}, fmt.Errorf("encode display webp: %w", err)
	}

	ext := extensionForContentType(contentType)
	relBase := filepath.ToSlash(filepath.Join("tasks", safePath(taskID)))
	stem := strconv.Itoa(index)
	originalRel := filepath.ToSlash(filepath.Join(relBase, stem+"_original"+ext))
	displayRel := filepath.ToSlash(filepath.Join(relBase, stem+"_preview.webp"))
	originalPath := filepath.Join(s.storageDir, filepath.FromSlash(originalRel))
	displayPath := filepath.Join(s.storageDir, filepath.FromSlash(displayRel))

	writtenFiles := make([]string, 0, 2)
	cleanupOnFailure := true
	defer func() {
		if !cleanupOnFailure {
			return
		}
		for _, path := range writtenFiles {
			_ = os.Remove(path)
		}
	}()

	if err := writeFile(originalPath, data); err != nil {
		return StoredAsset{}, err
	}
	writtenFiles = append(writtenFiles, originalPath)
	if err := writeFile(displayPath, display.Bytes()); err != nil {
		return StoredAsset{}, err
	}
	writtenFiles = append(writtenFiles, displayPath)
	bounds := img.Bounds()
	cleanupOnFailure = false

	return StoredAsset{
		ID:                  fmt.Sprintf("%s:%d", taskID, index),
		Index:               index,
		PreviewURL:          s.publicAssetURL(ctx, taskID, index, "preview", assetKey),
		DisplayURL:          s.publicAssetURL(ctx, taskID, index, "preview", assetKey),
		OriginalURL:         s.publicAssetURL(ctx, taskID, index, "original", assetKey),
		OriginalContentType: contentType,
		OriginalSizeBytes:   int64(len(data)),
		PreviewContentType:  displayContentType,
		PreviewSizeBytes:    int64(display.Len()),
		Width:               bounds.Dx(),
		Height:              bounds.Dy(),
		ExpiresAt:           expiresAt,
	}, nil
}

func (s *Service) publicAssetURL(ctx context.Context, taskID string, index int, variant, key string) string {
	return weborigin.Resolve(ctx, fmt.Sprintf("%s/%s/assets/%d/%s?key=%s", s.publicBasePath, safePath(taskID), index, variant, key))
}

func randomToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func normalizeContentType(contentType, decodedFormat string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(contentType, "image/") {
		if contentType == "image/jpg" {
			return "image/jpeg"
		}
		return contentType
	}
	switch strings.ToLower(decodedFormat) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func extensionForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/png":
		return ".png"
	default:
		return ".bin"
	}
}

func contentTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	default:
		return ""
	}
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func safePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "_"
	}
	return out
}
