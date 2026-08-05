package domain

import "time"

const (
	UpstreamAccountStatusActive   = "active"
	UpstreamAccountStatusInvalid  = "invalid"
	UpstreamAccountStatusDisabled = "disabled"
)

// UpstreamAccount is one concrete upstream (base-URL + credential), API Key 型上游。
// 重构后是顶级实体（原 ai_provider_endpoints 去掉 provider 父层）。
// The encrypted API key is intentionally NOT part of this type: it never leaves
// the repository/secret boundary. Some queries (create/list) do not return every
// field; absent fields are left zero.
type UpstreamAccount struct {
	ID                string
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	BaseURL           string
	ExtraHeaders      []byte
	DefaultProtocol   string
	ConcurrencyLimit  *int
	// 账号级上游成本绑定。PriceBookID 空 = 未绑定；
	// TenantMultiplier nil = 未设置（计费时 COALESCE 到 1）。
	PriceBookID      string
	TenantMultiplier *float64
	Status           string
	InvalidReason    string
	InvalidAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
