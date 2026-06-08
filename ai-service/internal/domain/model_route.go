package domain

import "time"

// ModelRoute is the management-domain view of a row in ai_model_routes
// (create/get/update projection — only the fields the console exposes; the
// table's scorer/timeout/numeric columns are not surfaced here).
type ModelRoute struct {
	ID                   string
	ModelID              string
	UpstreamDeploymentID string
	Priority             int32
	Weight               int32
	SupportsStream       bool
	Status               string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ModelRouteListItem is the list projection: a ModelRoute joined with upstream
// deployment / endpoint / provider / pool descriptors for display.
type ModelRouteListItem struct {
	ModelRoute
	UpstreamDeploymentName string
	UpstreamModel          string
	CapabilityType         string
	UpstreamProtocol       string
	HealthStatus           string
	CredentialSource       string
	EndpointID             string
	EndpointName           string
	BaseURL                string
	ProviderID             string
	ProviderCode           string
	ProviderName           string
	PoolID                 string
	PoolName               string
	FixedProviderType      string
}
