// Package tokenrefresh provides a background service that keeps OAuth access
// tokens fresh. It polls every 5 minutes, finds credentials expiring within
// 30 minutes, and refreshes them using each provider's token endpoint.
package tokenrefresh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/lifecycle"
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
	TokenURL      string
	ClientID      string
	ClientSecret  string // empty for providers that don't need it
	RequireSecret bool
	Scope         string
	UseJSON       bool // true = JSON body; false = form-encoded body
}

const (
	codexRefreshScope = "openid profile email"
	geminiCLIScope    = "https://www.googleapis.com/auth/cloud-platform " +
		"https://www.googleapis.com/auth/userinfo.email " +
		"https://www.googleapis.com/auth/userinfo.profile"
	antigravityScope = geminiCLIScope + " " +
		"https://www.googleapis.com/auth/cclog " +
		"https://www.googleapis.com/auth/experimentsandconfigs"
)

// defaultProviderConfigs mirrors the refresh contract of each simulated client.
func defaultProviderConfigs() map[domain.FixedProviderType]providerConfig {
	return map[domain.FixedProviderType]providerConfig{
		domain.FixedProviderCodex: {
			TokenURL: "https://auth.openai.com/oauth/token",
			ClientID: "app_EMoamEEZ73f0CkXaXp7hrann",
			Scope:    codexRefreshScope,
			UseJSON:  false,
		},
		domain.FixedProviderClaudeOAuth: {
			TokenURL: "https://platform.claude.com/v1/oauth/token",
			ClientID: "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
			UseJSON:  true,
		},
		domain.FixedProviderGeminiCLI: {
			TokenURL:      "https://oauth2.googleapis.com/token",
			ClientID:      "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
			ClientSecret:  strings.TrimSpace(os.Getenv("GEMINI_OAUTH_CLIENT_SECRET")),
			RequireSecret: true,
			Scope:         geminiCLIScope,
			UseJSON:       false,
		},
		domain.FixedProviderAntigravity: {
			TokenURL:      "https://oauth2.googleapis.com/token",
			ClientID:      "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
			ClientSecret:  strings.TrimSpace(os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET")),
			RequireSecret: true,
			Scope:         antigravityScope,
			UseJSON:       false,
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

type tokenEndpointError struct {
	status    int
	code      string
	detail    string
	permanent bool
}

func (e *tokenEndpointError) Error() string {
	return fmt.Sprintf("token endpoint %d: %s", e.status, e.detail)
}

func isPermanentTokenError(err error) bool {
	var endpointErr *tokenEndpointError
	return errors.As(err, &endpointErr) && endpointErr.permanent
}

// Refresher periodically refreshes expiring OAuth credentials.
type Refresher struct {
	store           *pgadapter.OAuthCredentialStore
	client          *http.Client
	logger          *zap.Logger
	interval        time.Duration
	window          time.Duration // refresh credentials expiring within this window
	providerConfigs map[domain.FixedProviderType]providerConfig
	lifecycleMu     sync.Mutex
	started         bool
	stopped         bool
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	flights         singleflight.Group
	ownerCtx        context.Context
	active          map[uint64]context.CancelFunc
	nextRefresh     uint64
}

var _ lifecycle.Component = (*Refresher)(nil)

// New creates a Refresher that checks every interval and refreshes credentials
// expiring within window.
func New(store *pgadapter.OAuthCredentialStore, logger *zap.Logger) *Refresher {
	if logger == nil {
		logger = zap.NewNop()
	}
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
	if r == nil || r.store == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.lifecycleMu.Lock()
	if r.started || r.stopped {
		r.lifecycleMu.Unlock()
		return
	}
	r.started = true
	workerCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.ownerCtx = workerCtx
	r.wg.Add(1)
	r.lifecycleMu.Unlock()
	go func() { defer r.wg.Done(); r.run(workerCtx) }()
}

// Stop cancels the refresh loop and waits for an in-flight provider request
// and its persistence decision to finish.
func (r *Refresher) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.lifecycleMu.Lock()
	r.stopped = true
	if r.cancel != nil {
		r.cancel()
	}
	for _, cancel := range r.active {
		cancel()
	}
	r.lifecycleMu.Unlock()
	return r.wait(ctx)
}

// Health returns a lock-safe lifecycle snapshot for management probes.
func (r *Refresher) Health() lifecycle.HealthSnapshot {
	if r == nil {
		return lifecycle.HealthSnapshot{}
	}
	r.lifecycleMu.Lock()
	started, stopped := r.started, r.stopped
	r.lifecycleMu.Unlock()
	return lifecycle.HealthSnapshot{Started: started, Stopped: stopped}
}

func (r *Refresher) wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Refresher) run(ctx context.Context) {
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
	if err := r.refresh(ctx, row.ID, row.TokenVersion); err != nil && ctx.Err() == nil {
		r.logger.Warn("tokenrefresh: refresh failed", zap.String("cred_id", row.ID), zap.Error(err))
	}
}

// beginRefresh owns shared work independently of any one waiting HTTP caller.
func (r *Refresher) beginRefresh(ctx context.Context) (context.Context, func(), error) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.stopped {
		return nil, nil, errors.New("token refresher is stopped")
	}
	shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	stopOwner := func() bool { return false }
	if r.ownerCtx != nil {
		stopOwner = context.AfterFunc(r.ownerCtx, cancel)
	}
	if r.active == nil {
		r.active = make(map[uint64]context.CancelFunc)
	}
	r.nextRefresh++
	id := r.nextRefresh
	r.active[id] = cancel
	r.wg.Add(1)
	return shared, func() {
		stopOwner()
		cancel()
		r.lifecycleMu.Lock()
		delete(r.active, id)
		r.lifecycleMu.Unlock()
		r.wg.Done()
	}, nil
}

func (r *Refresher) refresh(ctx context.Context, credID string, expectedVersion int64) error {
	ch := r.flights.DoChan(credID, func() (any, error) {
		shared, release, err := r.beginRefresh(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		var refreshErr error
		err = r.store.WithRefreshLock(shared, credID, func(store *pgadapter.OAuthCredentialStore) error {
			cred, err := store.GetDecryptedByID(shared, credID)
			if err != nil {
				return err
			}
			if cred.Status != domain.OAuthCredentialActive {
				return domain.ErrConflict
			}
			// Another entry point/process already refreshed this snapshot while we waited.
			if cred.TokenVersion != expectedVersion {
				return nil
			}
			cfg, ok := r.providerConfigs[cred.ProviderType]
			if !ok {
				return fmt.Errorf("unsupported provider type %q", cred.ProviderType)
			}
			if cred.RefreshToken == "" {
				return errors.New("credential has no refresh token")
			}
			tok, err := r.callTokenEndpoint(shared, cfg, cred.RefreshToken)
			if err != nil {
				refreshErr = fmt.Errorf("token endpoint: %w", err)
				if isPermanentTokenError(err) {
					err = store.MarkInvalid(shared, credID, fmt.Sprintf("refresh failed: %v", err), cred.TokenVersion)
					if errors.Is(err, domain.ErrConflict) {
						refreshErr = nil
						return nil
					}
					return err
				}
				return nil
			}
			var expiresAt *time.Time
			if tok.ExpiresIn > 0 {
				t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
				expiresAt = &t
			}
			refreshToken := tok.RefreshToken
			if refreshToken == "" {
				refreshToken = cred.RefreshToken
			}
			_, err = store.UpdateTokens(shared, credID, tok.AccessToken, refreshToken, expiresAt, cred.TokenVersion)
			if errors.Is(err, domain.ErrConflict) {
				return nil
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		return nil, refreshErr
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-ch:
		return result.Err
	}
}

// callTokenEndpoint sends a refresh_token grant request to the provider's token URL.
func (r *Refresher) callTokenEndpoint(ctx context.Context, cfg providerConfig, refreshToken string) (*tokenResponse, error) {
	if cfg.RequireSecret && cfg.ClientSecret == "" {
		return nil, errors.New("oauth client secret is not configured")
	}
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
	if cfg.Scope != "" {
		params["scope"] = cfg.Scope
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
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &payload)
		code := strings.TrimSpace(payload.Error)
		return nil, &tokenEndpointError{
			status:    resp.StatusCode,
			code:      code,
			detail:    sanitizeOAuthError(body),
			permanent: code == "invalid_grant" || code == "invalid_token",
		}
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
// Only an explicit invalid_grant/invalid_token response invalidates the
// credential. Network errors and provider 5xx responses remain retryable.
func (r *Refresher) RefreshByID(ctx context.Context, credID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	row, err := r.store.GetByID(ctx, credID)
	if err != nil {
		return fmt.Errorf("get credential: %w", err)
	}
	return r.refresh(ctx, credID, row.TokenVersion)
}
