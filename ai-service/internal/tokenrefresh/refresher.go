// Package tokenrefresh provides a background service that keeps OAuth access
// tokens fresh. It polls every 5 minutes, finds credentials expiring within
// 30 minutes, and refreshes them using each provider's token endpoint.
package tokenrefresh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	pgadapter "xiaodou/uni-ai-api/internal/adapters/postgres"
	"xiaodou/uni-ai-api/internal/domain"
)

// sanitizeOAuthError truncates an OAuth error response body and masks
// token-like substrings so that values which might leak access/refresh tokens
// or auth codes never reach DB columns / structured logs.
//
// Heuristics:
//   - Cap total length to 512 bytes.
//   - Replace any run of [A-Za-z0-9_\-.]{24,} with "<redacted:N>".
//
// 24+ chars is long enough to catch realistic bearer/JWT/OAuth values without
// shredding short error codes like "invalid_grant".
func sanitizeOAuthError(body []byte) string {
	const maxLen = 512
	s := string(body)
	s = oauthTokenLike.ReplaceAllStringFunc(s, func(m string) string {
		return fmt.Sprintf("<redacted:%d>", len(m))
	})
	if len(s) > maxLen {
		s = s[:maxLen] + "…(truncated)"
	}
	return s
}

var oauthTokenLike = regexp.MustCompile(`[A-Za-z0-9_\-.]{24,}`)

// providerConfig holds the token refresh settings for a fixed OAuth provider.
type providerConfig struct {
	TokenURL     string
	ClientID     string
	ClientSecret string // empty for providers that don't need it
	UseJSON      bool   // true = JSON body; false = form-encoded body
}

// defaultProviderConfigs returns the base config for each provider.
// ClientSecret fields for Google-based providers must be supplied via
// GEMINI_OAUTH_CLIENT_SECRET and ANTIGRAVITY_OAUTH_CLIENT_SECRET env vars.
func defaultProviderConfigs() map[domain.FixedProviderType]providerConfig {
	return map[domain.FixedProviderType]providerConfig{
		domain.FixedProviderCodex: {
			TokenURL: "https://auth.openai.com/oauth/token",
			ClientID: "app_EMoamEEZ73f0CkXaXp7hrann",
			UseJSON:  false,
		},
		domain.FixedProviderClaudeOAuth: {
			TokenURL: "https://console.anthropic.com/v1/oauth/token",
			ClientID: "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
			UseJSON:  true,
		},
		domain.FixedProviderGeminiCLI: {
			TokenURL:     "https://oauth2.googleapis.com/token",
			ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
			ClientSecret: os.Getenv("GEMINI_OAUTH_CLIENT_SECRET"),
			UseJSON:      false,
		},
		domain.FixedProviderAntigravity: {
			TokenURL:     "https://oauth2.googleapis.com/token",
			ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
			ClientSecret: os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET"),
			UseJSON:      false,
		},
	}
}

// tokenResponse is the standard OAuth token endpoint response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	TokenType    string `json:"token_type"`
}

// Refresher periodically refreshes expiring OAuth credentials.
type Refresher struct {
	store           *pgadapter.OAuthCredentialStore
	client          *http.Client
	logger          *zap.Logger
	interval        time.Duration
	window          time.Duration // refresh credentials expiring within this window
	providerConfigs map[domain.FixedProviderType]providerConfig
}

// New creates a Refresher that checks every interval and refreshes credentials
// expiring within window.
func New(store *pgadapter.OAuthCredentialStore, logger *zap.Logger) *Refresher {
	return &Refresher{
		store:           store,
		client:          &http.Client{Timeout: 30 * time.Second},
		logger:          logger,
		interval:        5 * time.Minute,
		window:          30 * time.Minute,
		providerConfigs: defaultProviderConfigs(),
	}
}

// Start runs the refresh loop until ctx is cancelled.
func (r *Refresher) Start(ctx context.Context) {
	// Run once immediately so restarts pick up near-expired tokens right away.
	r.runOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

func (r *Refresher) runOnce(ctx context.Context) {
	rows, err := r.store.ListExpiring(ctx, r.window)
	if err != nil {
		r.logger.Error("tokenrefresh: list expiring credentials failed", zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}

	r.logger.Info("tokenrefresh: refreshing credentials", zap.Int("count", len(rows)))
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		r.refreshOne(ctx, row)
	}
}

func (r *Refresher) refreshOne(ctx context.Context, row pgadapter.OAuthCredentialRow) {
	cfg, ok := r.providerConfigs[domain.FixedProviderType(row.ProviderType)]
	if !ok {
		r.logger.Warn("tokenrefresh: unknown provider type, skipping",
			zap.String("cred_id", row.ID), zap.String("provider_type", row.ProviderType))
		return
	}

	cred, err := r.store.GetDecryptedByID(ctx, row.ID)
	if err != nil {
		r.logger.Error("tokenrefresh: get credential failed", zap.String("cred_id", row.ID), zap.Error(err))
		return
	}

	refreshToken := cred.RefreshToken
	if refreshToken == "" {
		r.logger.Warn("tokenrefresh: no refresh token, skipping", zap.String("cred_id", row.ID))
		return
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	tok, err := r.callTokenEndpoint(refreshCtx, cfg, refreshToken)
	cancel()
	if err != nil {
		r.logger.Error("tokenrefresh: token refresh failed",
			zap.String("cred_id", row.ID), zap.String("provider", row.ProviderType), zap.Error(err))
		if markErr := r.store.MarkInvalid(ctx, row.ID, fmt.Sprintf("refresh failed: %v", err)); markErr != nil {
			r.logger.Error("tokenrefresh: mark invalid failed", zap.String("cred_id", row.ID), zap.Error(markErr))
		}
		return
	}

	// Compute new expiry
	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = refreshToken // keep the old one if not rotated
	}

	if err := r.store.UpdateTokens(ctx, row.ID, tok.AccessToken, newRefresh, expiresAt); err != nil {
		r.logger.Error("tokenrefresh: update tokens failed", zap.String("cred_id", row.ID), zap.Error(err))
		return
	}

	r.logger.Info("tokenrefresh: refreshed successfully",
		zap.String("cred_id", row.ID), zap.String("provider", row.ProviderType),
		zap.Int("expires_in_s", tok.ExpiresIn))
}

// callTokenEndpoint sends a refresh_token grant request to the provider's token URL.
func (r *Refresher) callTokenEndpoint(ctx context.Context, cfg providerConfig, refreshToken string) (*tokenResponse, error) {
	var (
		reqBody     io.Reader
		contentType string
	)

	params := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     cfg.ClientID,
	}
	if cfg.ClientSecret != "" {
		params["client_secret"] = cfg.ClientSecret
	}

	if cfg.UseJSON {
		contentType = "application/json"
		b, _ := json.Marshal(params)
		reqBody = strings.NewReader(string(b))
	} else {
		contentType = "application/x-www-form-urlencoded"
		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
		reqBody = strings.NewReader(form.Encode())
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, sanitizeOAuthError(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned empty access_token")
	}
	return &tok, nil
}

// RefreshByID immediately refreshes the credential with the given ID.
// Returns an error if the provider type is unknown, the credential has no
// refresh token, or the token endpoint request fails.
// On success the credential status is set to 'active' and tokens are updated.
// On failure the credential status is set to 'invalid'.
func (r *Refresher) RefreshByID(ctx context.Context, credID string) error {
	row, err := r.store.GetByID(ctx, credID)
	if err != nil {
		return fmt.Errorf("get credential: %w", err)
	}
	cfg, ok := r.providerConfigs[domain.FixedProviderType(row.ProviderType)]
	if !ok {
		return fmt.Errorf("unsupported provider type %q", row.ProviderType)
	}
	cred, err := r.store.GetDecryptedByID(ctx, credID)
	if err != nil {
		return fmt.Errorf("decrypt credential: %w", err)
	}
	if cred.RefreshToken == "" {
		return fmt.Errorf("credential has no refresh token")
	}
	tok, err := r.callTokenEndpoint(ctx, cfg, cred.RefreshToken)
	if err != nil {
		_ = r.store.MarkInvalid(ctx, credID, fmt.Sprintf("manual refresh failed: %v", err))
		return fmt.Errorf("token endpoint: %w", err)
	}
	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = cred.RefreshToken
	}
	if err := r.store.UpdateTokens(ctx, credID, tok.AccessToken, newRefresh, expiresAt); err != nil {
		return fmt.Errorf("update tokens: %w", err)
	}
	return nil
}
