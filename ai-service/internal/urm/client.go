package urm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"xiaodou/uni-ai-api/internal/config"
)

type Client struct {
	baseURL     string
	clientID    string
	displayName string
	description string
	httpClient  *http.Client

	mu           sync.Mutex
	clientSecret string
	serviceToken string
	tokenExpiry  time.Time
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func NewClient(cfg config.URMConfig) (*Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		clientID:    cfg.ClientID,
		displayName: resolveDisplayName(cfg.ClientID, cfg.DisplayName),
		description: cfg.Description,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	// 自动注册获取client_secret
	if err := client.autoRegister(context.Background()); err != nil {
		return nil, fmt.Errorf("auto register failed: %w", err)
	}

	return client, nil
}

func resolveDisplayName(clientID, displayName string) string {
	if displayName == "" {
		return clientID
	}
	return displayName
}

func (c *Client) autoRegister(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"clientId":    c.clientID,
		"displayName": c.displayName,
		"description": c.description,
	})
	if err != nil {
		return fmt.Errorf("marshal register request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/clients/bootstrap-secret", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read register response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("register status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope Response[bootstrapSecretResponse]
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("parse register response: %w", err)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("register error %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data.ClientSecret == "" {
		return fmt.Errorf("register returned empty client_secret")
	}

	c.clientSecret = envelope.Data.ClientSecret
	return nil
}

func (c *Client) Freeze(ctx context.Context, req FreezeRequest) (*FreezeResponse, error) {
	var resp Response[FreezeResponse]
	if err := c.do(ctx, http.MethodPost, "/internal/v1/settle/freeze", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (c *Client) Confirm(ctx context.Context, req ConfirmRequest) (*ConfirmResponse, error) {
	var resp Response[ConfirmResponse]
	if err := c.do(ctx, http.MethodPost, "/internal/v1/settle/confirm", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (c *Client) Cancel(ctx context.Context, transactionID string) error {
	var resp Response[map[string]any]
	return c.do(ctx, http.MethodPost, "/internal/v1/settle/cancel/"+url.PathEscape(transactionID), nil, &resp)
}

// Consume 调用 URM 单阶段幂等扣款接口。Phase 1 起 ai-service 的分账层用此
// 接口替代 Freeze/Confirm 两阶段流程，把聚合后的整数积分一次性扣掉。
func (c *Client) Consume(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	var resp Response[ConsumeResponse]
	if err := c.do(ctx, http.MethodPost, "/internal/v1/settle/consume", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ExchangeCode exchanges an SSO authorization code for a token pair.
// This is a public client endpoint — no service JWT required, body is form-encoded.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (*TokenPairResponse, error) {
	body := "code=" + url.QueryEscape(code) + "&redirect_uri=" + url.QueryEscape(redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/oauth2/exchange", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read exchange response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("exchange status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope Response[TokenPairResponse]
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("parse exchange response: %w", err)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("exchange error %d: %s", envelope.Code, envelope.Message)
	}
	return &envelope.Data, nil
}

func (c *Client) QueryBalance(ctx context.Context, accountType int, accountID string, detail bool) (*BalanceResponse, error) {
	q := url.Values{}
	q.Set("accountType", fmt.Sprintf("%d", accountType))
	q.Set("accountId", accountID)
	if detail {
		q.Set("detail", "true")
	}

	var resp Response[BalanceResponse]
	path := "/internal/v1/assets/balance?" + q.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (c *Client) UserInfo(ctx context.Context, token string) (*UserInfoResponse, error) {
	var resp Response[UserInfoResponse]
	if err := c.doBearer(ctx, http.MethodGet, "/api/oauth2/userinfo", token, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// getServiceToken returns a valid cached service JWT, refreshing if needed.
func (c *Client) getServiceToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.serviceToken != "" && time.Now().Before(c.tokenExpiry.Add(-5*time.Minute)) {
		return c.serviceToken, nil
	}

	if c.clientSecret == "" {
		return "", fmt.Errorf("client secret not initialized")
	}

	token, expiry, err := c.fetchServiceToken(ctx)
	if err != nil {
		return "", err
	}
	c.serviceToken = token
	c.tokenExpiry = expiry
	return token, nil
}

type bootstrapSecretResponse struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	DisplayName  string `json:"displayName"`
}

// bootstrapSecret calls URM to retrieve (or create) this service's client_secret.
// The endpoint is IP-whitelist protected; in production only private network IPs are allowed.
func (c *Client) bootstrapSecret(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{
		"clientId":    c.clientID,
		"displayName": "Uni AI API",
		"description": "AI API gateway service",
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/clients/bootstrap-secret", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create bootstrap request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bootstrap request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read bootstrap response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bootstrap status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope Response[bootstrapSecretResponse]
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return "", fmt.Errorf("parse bootstrap response: %w", err)
	}
	if envelope.Code != 0 {
		return "", fmt.Errorf("bootstrap error %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data.ClientSecret == "" {
		return "", fmt.Errorf("bootstrap returned empty client_secret")
	}
	return envelope.Data.ClientSecret, nil
}

type serviceTokenResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int64  `json:"expiresIn"`
}

func (c *Client) fetchServiceToken(ctx context.Context) (string, time.Time, error) {
	body := strings.NewReader("grant_type=client_credentials&client_id=" + url.QueryEscape(c.clientID) + "&client_secret=" + url.QueryEscape(c.clientSecret))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/oauth2/token", body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("token status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope Response[serviceTokenResponse]
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return "", time.Time{}, fmt.Errorf("parse token response: %w", err)
	}
	if envelope.Code != 0 {
		return "", time.Time{}, fmt.Errorf("token error %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("token response missing accessToken")
	}

	expiresIn := envelope.Data.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	expiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
	return envelope.Data.AccessToken, expiry, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	token, err := c.getServiceToken(ctx)
	if err != nil {
		return fmt.Errorf("get service token: %w", err)
	}

	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("urm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read urm response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("urm status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("parse urm typed response: %w", err)
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("parse urm envelope: %w", err)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("urm error %d: %s", envelope.Code, envelope.Message)
	}

	return nil
}

func (c *Client) doBearer(ctx context.Context, method, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(strings.TrimPrefix(token, "Bearer ")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("urm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read urm response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("urm status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("parse urm typed response: %w", err)
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("parse urm envelope: %w", err)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("urm error %d: %s", envelope.Code, envelope.Message)
	}

	return nil
}
