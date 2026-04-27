package urm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"uni-ai-api/backend/internal/config"
)

type Client struct {
	baseURL    string
	appKey     string
	appSecret  string
	httpClient *http.Client
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func NewClient(cfg config.URMConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Client{
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		appKey:    cfg.AppKey,
		appSecret: cfg.AppSecret,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
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

func (c *Client) QueryBalance(ctx context.Context, accountType int, accountID string, detail bool) (*BalanceResponse, error) {
	q := url.Values{}
	q.Set("accountType", strconv.Itoa(accountType))
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

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyBytes []byte
	var err error
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

	for k, v := range c.signHeaders(method, path, string(bodyBytes)) {
		req.Header.Set(k, v)
	}

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
	if envelope.Code != 0 && envelope.Code != 200 {
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
	if envelope.Code != 0 && envelope.Code != 200 {
		return fmt.Errorf("urm error %d: %s", envelope.Code, envelope.Message)
	}

	return nil
}

func (c *Client) signHeaders(method, path, body string) map[string]string {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	signatureText := method + path + timestamp + nonce + body

	mac := hmac.New(sha256.New, []byte(c.appSecret))
	mac.Write([]byte(signatureText))
	signature := hex.EncodeToString(mac.Sum(nil))

	return map[string]string{
		"X-URM-AppKey":    c.appKey,
		"X-URM-Timestamp": timestamp,
		"X-URM-Nonce":     nonce,
		"X-URM-Signature": signature,
		"Content-Type":    "application/json",
	}
}
