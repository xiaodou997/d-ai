package ports

import (
	"context"
	"errors"
)

var (
	// ErrTenantEndUserNotFound means the requested active end user has no
	// visible tenant ownership.
	ErrTenantEndUserNotFound = errors.New("tenant end user not found")
	// ErrTenantReferenced means tenant deletion is blocked by dependent data.
	ErrTenantReferenced = errors.New("tenant is referenced")
)

// TenantListQuery is the validated management tenant list query.
type TenantListQuery struct {
	Keyword string
	Status  string
	Page    int64
	Size    int64
	Offset  int64
}

// TenantListItem is the management tenant list projection.
type TenantListItem struct {
	TenantID      string  `json:"tenantId"`
	TenantName    string  `json:"tenantName"`
	ContactPerson *string `json:"contactPerson"`
	ContactEmail  *string `json:"contactEmail"`
	Status        *string `json:"status"`
	CreatedTime   *int64  `json:"createdTime"`
	BalanceUSD    float64 `json:"balanceUsd"`
	UserCount     int64   `json:"userCount"`
}

type TenantListPage struct {
	Records []TenantListItem
	Total   int64
	Page    int64
	Size    int64
}

// TenantDetails is the scoped tenant projection used by management reads.
type TenantDetails struct {
	TenantID      string
	TenantName    string
	ContactPerson *string
	ContactEmail  *string
	Status        string
	CreatedTime   int64
}

// TenantSummary is the minimal tenant identity projection used for AI
// identity enrichment.
type TenantSummary struct {
	TenantID   string
	TenantName string
	Status     *string
}

// AdminTenantReader exposes tenant queries needed by management and identity
// HTTP adapters without leaking a PostgreSQL repository.
type AdminTenantReader interface {
	List(ctx context.Context, query TenantListQuery) (TenantListPage, error)
	GetTenantDetails(ctx context.Context, tenantID string) (*TenantDetails, error)
	GetEndUserTenantID(ctx context.Context, userID string) (string, error)
	GetByTenantIDs(ctx context.Context, tenantIDs []string) ([]*TenantSummary, error)
}
