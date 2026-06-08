package domain

import "time"

// Provider is an upstream vendor (e.g. OpenAI, Anthropic). Config is an opaque
// JSON object passed through to storage.
type Provider struct {
	ID        string
	Code      string
	Name      string
	Config    []byte
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProviderEndpoint is one concrete base-URL + credential under a Provider. The
// encrypted API key is intentionally NOT part of this type: it never leaves the
// repository/secret boundary. Some queries (create/list) do not return every
// field; absent fields are left zero.
type ProviderEndpoint struct {
	ID              string
	ProviderID      string
	Name            string
	BaseURL         string
	ExtraHeaders    []byte
	Weight          int32
	TimeoutMs       int32
	DefaultProtocol string
	// 账户级上游成本绑定（其下 deployment 默认继承）。PriceBookID 空 = 未绑定；
	// CostMultiplier nil = 未设置（计费时 COALESCE 到 1）。
	PriceBookID    string
	CostMultiplier *float64
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
