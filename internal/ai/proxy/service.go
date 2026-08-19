package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/system"
)

var (
	ErrNotFound        = errors.New("proxy node not found")
	ErrInvalidInput    = errors.New("proxy node input is invalid")
	ErrInvalidEndpoint = errors.New("proxy endpoint is invalid")
)

type Node struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	ProxyType     string     `json:"proxyType"`
	Endpoint      string     `json:"endpoint"`
	Username      string     `json:"username,omitempty"`
	Weight        int        `json:"weight"`
	Status        string     `json:"status"`
	HealthStatus  string     `json:"healthStatus"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type UpsertInput struct {
	Name      string
	ProxyType string
	Endpoint  string
	Username  string
	Password  string
	Weight    int
	Status    string
}

type Service struct {
	pool    *pgxpool.Pool
	modules *system.Service
}

func NewService(pool *pgxpool.Pool, modules *system.Service) *Service {
	return &Service{pool: pool, modules: modules}
}

func (s *Service) List(ctx context.Context) ([]Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, proxy_type, endpoint, username, weight, status, health_status,
		       last_checked_at, COALESCE(last_error, ''), created_at, updated_at
		FROM ai_proxy_nodes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Node, 0)
	for rows.Next() {
		var item Node
		if err := rows.Scan(&item.ID, &item.Name, &item.ProxyType, &item.Endpoint, &item.Username,
			&item.Weight, &item.Status, &item.HealthStatus, &item.LastCheckedAt, &item.LastError,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (Node, error) {
	var item Node
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, proxy_type, endpoint, username, weight, status, health_status,
		       last_checked_at, COALESCE(last_error, ''), created_at, updated_at
		FROM ai_proxy_nodes WHERE id = $1`, id).Scan(
		&item.ID, &item.Name, &item.ProxyType, &item.Endpoint, &item.Username,
		&item.Weight, &item.Status, &item.HealthStatus, &item.LastCheckedAt, &item.LastError,
		&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	return item, err
}

func (s *Service) Upsert(ctx context.Context, id string, input UpsertInput, actor string) (Node, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ProxyType = strings.ToLower(strings.TrimSpace(input.ProxyType))
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Name == "" || input.ProxyType == "" || input.Endpoint == "" {
		return Node{}, fmt.Errorf("%w: name, proxyType and endpoint are required", ErrInvalidInput)
	}
	if input.ProxyType != "http" && input.ProxyType != "socks5" {
		return Node{}, fmt.Errorf("%w: proxyType must be http or socks5", ErrInvalidInput)
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Status != "active" && input.Status != "disabled" {
		return Node{}, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
	}
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5") {
		return Node{}, ErrInvalidEndpoint
	}
	if parsed.User != nil {
		return Node{}, fmt.Errorf("%w: proxy credentials must be entered separately", ErrInvalidInput)
	}
	if (input.ProxyType == "http" && parsed.Scheme == "socks5") || (input.ProxyType == "socks5" && parsed.Scheme != "socks5") {
		return Node{}, fmt.Errorf("%w: proxyType does not match endpoint scheme", ErrInvalidInput)
	}
	if net.ParseIP(parsed.Hostname()).IsLoopback() || parsed.Hostname() == "localhost" {
		return Node{}, fmt.Errorf("%w: loopback proxy endpoints are not allowed", ErrInvalidInput)
	}
	if input.Weight < 1 {
		input.Weight = 1
	}
	if input.Weight > 1000 {
		input.Weight = 1000
	}
	if id == "" {
		id = uuid.NewString()
	} else {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_proxy_nodes WHERE id = $1)`, id).Scan(&exists); err != nil {
			return Node{}, err
		}
		if !exists {
			return Node{}, ErrNotFound
		}
	}
	var encrypted string
	if input.Password != "" {
		encrypted, err = clientsecret.Encrypt(input.Password)
		if err != nil {
			return Node{}, err
		}
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO ai_proxy_nodes (id, name, proxy_type, endpoint, username, proxy_password_enc, weight, status, created_by)
		VALUES ($1, $2, $3, $4, $5, COALESCE(NULLIF($6, ''), ''), $7, $8, NULLIF($9, ''))
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, proxy_type = EXCLUDED.proxy_type,
		 endpoint = EXCLUDED.endpoint, username = EXCLUDED.username,
		 proxy_password_enc = CASE WHEN $10 THEN EXCLUDED.proxy_password_enc ELSE ai_proxy_nodes.proxy_password_enc END,
		 weight = EXCLUDED.weight, status = EXCLUDED.status, updated_at = now()`,
		id, input.Name, input.ProxyType, input.Endpoint, input.Username, encrypted, input.Weight, input.Status, actor, input.Password != "")
	if err != nil {
		return Node{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM ai_proxy_nodes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SelectProxy implements the upstream transport selector. A nil URL means
// direct egress is allowed (module disabled or no healthy node).
func (s *Service) SelectProxy(ctx context.Context) (*url.URL, error) {
	if s.modules != nil {
		active, err := s.modules.IsActive(ctx, system.ModuleProxyEgress)
		if err != nil || !active {
			return nil, err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT endpoint, proxy_type, username, proxy_password_enc, weight
		FROM ai_proxy_nodes WHERE status = 'active' AND health_status <> 'unhealthy'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		endpoint, kind, username, encrypted string
		weight                              int
	}
	items := make([]candidate, 0)
	total := 0
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.endpoint, &item.kind, &item.username, &item.encrypted, &item.weight); err != nil {
			return nil, err
		}
		if item.weight < 1 {
			item.weight = 1
		}
		total += item.weight
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	choice := rand.IntN(total)
	selected := items[0]
	for _, item := range items {
		choice -= item.weight
		if choice < 0 {
			selected = item
			break
		}
	}
	parsed, err := url.Parse(selected.endpoint)
	if err != nil {
		return nil, err
	}
	if selected.username != "" {
		password := ""
		if selected.encrypted != "" {
			password, err = clientsecret.Decrypt(selected.encrypted)
			if err != nil {
				return nil, err
			}
		}
		parsed.User = url.UserPassword(selected.username, password)
	}
	return parsed, nil
}
