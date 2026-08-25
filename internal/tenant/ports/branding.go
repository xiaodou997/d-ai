package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTenantNotFound means that the requested tenant no longer exists.
	ErrTenantNotFound = errors.New("tenant not found")
	// ErrTenantNameTaken means that a tenant name conflicts with another tenant.
	ErrTenantNameTaken = errors.New("tenant name taken")
)

// PortalBranding is the tenant-owned presentation configuration exposed to
// the customer portal. The PNG bytes stay inside the application boundary so
// the raw favicon endpoint can serve the same validated projection.
type PortalBranding struct {
	TenantID         string
	TenantName       string
	CustomerSiteName string
	FaviconPNG       []byte
	FaviconUpdatedAt *time.Time
}

// PortalBrandingReader owns the tenant branding query used by the customer
// portal and public favicon endpoint.
type PortalBrandingReader interface {
	Get(ctx context.Context, tenantID string) (*PortalBranding, error)
}

// PortalBrandingWriter owns tenant branding mutations. The PostgreSQL adapter
// implements the transaction and constraint semantics; HTTP handlers only
// depend on this narrow application port.
type PortalBrandingWriter interface {
	UpdateSettings(ctx context.Context, tenantID, tenantName, customerSiteName string) (*PortalBranding, error)
	UpdateFavicon(ctx context.Context, tenantID string, faviconPNG []byte) (*PortalBranding, error)
	ClearFavicon(ctx context.Context, tenantID string) (*PortalBranding, error)
}

// PortalBrandingStore is the complete adapter contract used by composition
// roots that wire both the read and write ports to one implementation.
type PortalBrandingStore interface {
	PortalBrandingReader
	PortalBrandingWriter
}
