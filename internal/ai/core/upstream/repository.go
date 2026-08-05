package upstream

import "context"

// Repository is the vNext persistence port for upstream resources.
type Repository interface {
	CreateUpstream(ctx context.Context, in UpstreamWrite) (Upstream, error)
	ListUpstreams(ctx context.Context) ([]Upstream, error)
	GetUpstream(ctx context.Context, id string) (Upstream, error)
	UpdateUpstream(ctx context.Context, id string, in UpstreamWrite) (Upstream, error)
	UpdateUpstreamStatus(ctx context.Context, id, status string) (Upstream, error)
	DeleteUpstream(ctx context.Context, id string) error

	CreateCredential(ctx context.Context, in CredentialWrite) (Credential, error)
	ListCredentials(ctx context.Context, upstreamID string) ([]Credential, error)
	UpdateCredential(ctx context.Context, id string, in CredentialWrite) (Credential, error)
	DeleteCredential(ctx context.Context, id string) error

	CreateOAuthPool(ctx context.Context, in OAuthPoolWrite) (OAuthPool, error)
	ListOAuthPools(ctx context.Context) ([]OAuthPool, error)
	UpdateOAuthPool(ctx context.Context, id string, in OAuthPoolWrite) (OAuthPool, error)
	DeleteOAuthPool(ctx context.Context, id string) error

	CreateOAuthPoolCredential(ctx context.Context, in OAuthPoolCredentialWrite) (OAuthPoolCredential, error)
	ListOAuthPoolCredentials(ctx context.Context, poolID string) ([]OAuthPoolCredential, error)
	UpdateOAuthPoolCredential(ctx context.Context, id string, in OAuthPoolCredentialWrite) (OAuthPoolCredential, error)
	DeleteOAuthPoolCredential(ctx context.Context, id string) error

	CreateModelBinding(ctx context.Context, in ModelBindingWrite) (ModelBinding, error)
	ListModelBindings(ctx context.Context, filter ModelBindingFilter) ([]ModelBinding, error)
	UpdateModelBinding(ctx context.Context, id string, in ModelBindingWrite) (ModelBinding, error)
	DeleteModelBinding(ctx context.Context, id string) error
}

type UpstreamWrite struct {
	Code           string
	Name           string
	ProviderFamily ProviderFamily
	AccessMode     AccessMode
	BaseURL        string
	Headers        map[string]string
	Status         Status
	Notes          string
}

type CredentialWrite struct {
	UpstreamID       string
	Name             string
	CredentialKind   CredentialKind
	HeaderName       string
	SecretCiphertext string
	ExtraAuth        map[string]any
	Weight           int
	Status           Status
}

type OAuthPoolWrite struct {
	Code              string
	Name              string
	FixedProviderType FixedProviderType
	SelectionStrategy SelectionStrategy
	StickyScope       StickyScope
	Status            Status
	Notes             string
}

type OAuthPoolCredentialWrite struct {
	PoolID                 string
	Name                   string
	Email                  string
	AccessTokenCiphertext  string
	RefreshTokenCiphertext string
	TokenType              string
	Scope                  string
	AuthMetadata           map[string]any
	Weight                 int
	Status                 Status
}

type ModelBindingWrite struct {
	UpstreamKind      AccessMode
	UpstreamID        string
	ModelID           string
	Capability        string
	RequestSurface    string
	ResponseSurface   string
	UpstreamModelName string
	Priority          int
	Status            Status
	Config            map[string]any
}

type ModelBindingFilter struct {
	UpstreamKind   AccessMode
	UpstreamID     string
	ModelID        string
	Capability     string
	RequestSurface string
	Status         Status
}
