package serviceidentity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRenewInterval = 3 * time.Minute
	DefaultTokenTTL      = 5 * time.Minute
	maxInstanceIDLength  = 128
)

type SessionConfig struct {
	ServiceBaseURL string
	ServiceID      string
	InstanceID     string
	Name           string
	Version        string
	Environment    string
	Timeout        time.Duration
	RenewInterval  time.Duration
}

type SessionResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	SourceCIDR  string `json:"source_cidr"`
	ServiceID   string `json:"service_id"`
	InstanceID  string `json:"instance_id"`
}

type Manager struct {
	cfg    SessionConfig
	client *http.Client

	mu        sync.RWMutex
	token     string
	expiresAt time.Time
	source    string
	ready     bool
	lastErr   error
	started   bool
}

func NewManager(cfg SessionConfig) (*Manager, error) {
	cfg.ServiceBaseURL = strings.TrimRight(strings.TrimSpace(cfg.ServiceBaseURL), "/")
	cfg.ServiceID = strings.TrimSpace(cfg.ServiceID)
	cfg.InstanceID = strings.TrimSpace(cfg.InstanceID)
	if cfg.ServiceBaseURL == "" {
		return nil, errors.New("URM service base URL is required")
	}
	if cfg.ServiceID == "" {
		return nil, errors.New("service ID is required")
	}
	if cfg.InstanceID == "" {
		cfg.InstanceID = defaultInstanceID()
	}
	if len(cfg.InstanceID) > maxInstanceIDLength {
		return nil, fmt.Errorf("instance ID must not exceed %d characters", maxInstanceIDLength)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = DefaultRenewInterval
	}
	return &Manager{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()
	go m.run(ctx)
}

func (m *Manager) run(ctx context.Context) {
	m.renew(ctx)
	ticker := time.NewTicker(m.cfg.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.renew(ctx)
		}
	}
}

func (m *Manager) renew(ctx context.Context) {
	resp, err := m.register(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.ready = false
		m.lastErr = err
		return
	}
	m.token = resp.AccessToken
	m.expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	m.source = resp.SourceCIDR
	m.ready = true
	m.lastErr = nil
}

func (m *Manager) Token(ctx context.Context) (string, error) {
	m.mu.RLock()
	token, expiry := m.token, m.expiresAt
	m.mu.RUnlock()
	if token != "" && time.Now().Before(expiry) {
		return token, nil
	}
	m.renew(ctx)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.token == "" || !time.Now().Before(m.expiresAt) {
		if m.lastErr != nil {
			return "", m.lastErr
		}
		return "", errors.New("service identity is not available")
	}
	return m.token, nil
}

func (m *Manager) Ready() (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ready && m.token != "" && time.Now().Before(m.expiresAt), m.lastErr
}

func (m *Manager) InstanceID() string { return m.cfg.InstanceID }

func (m *Manager) register(ctx context.Context) (*SessionResponse, error) {
	body, err := json.Marshal(map[string]string{
		"serviceId":   m.cfg.ServiceID,
		"instanceId":  m.cfg.InstanceID,
		"name":        m.cfg.Name,
		"version":     m.cfg.Version,
		"environment": m.cfg.Environment,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.ServiceBaseURL+"/service/v1/sessions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("register service session: %w", err)
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("register service session: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out SessionResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode service session: %w", err)
	}
	if out.AccessToken == "" || out.ExpiresIn <= 0 || out.SourceCIDR == "" {
		return nil, errors.New("invalid service session response")
	}
	return &out, nil
}

func newInstanceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("instance-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func defaultInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil {
		return newInstanceID()
	}
	return instanceIDFromHostname(hostname)
}

func instanceIDFromHostname(hostname string) string {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return newInstanceID()
	}
	if len(hostname) <= maxInstanceIDLength {
		return hostname
	}
	sum := sha256.Sum256([]byte(hostname))
	return "host-" + hex.EncodeToString(sum[:16])
}
