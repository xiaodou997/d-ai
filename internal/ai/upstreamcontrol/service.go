package upstreamcontrol

import (
	"context"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamaccess"
)

type Encryptor func(plaintext string) (string, error)

// Service implements upstream account management business logic.
type Service struct {
	repo    Repository
	encrypt Encryptor
}

func New(repo Repository, encrypt Encryptor) *Service {
	return &Service{repo: repo, encrypt: encrypt}
}

type CreateAccountInput struct {
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	BaseURL           string
	APIKey            string
	ExtraHeaders      []byte
	DefaultProtocol   string
	ConcurrencyLimit  *int
	PriceBookID       string
	TenantMultiplier  *float64
	Status            string
}

func (s *Service) CreateAccount(ctx context.Context, in CreateAccountInput) (domain.UpstreamAccount, error) {
	if in.Name == "" || in.BaseURL == "" || in.APIKey == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("", "name, base_url and api_key are required")
	}
	if in.ConcurrencyLimit != nil && *in.ConcurrencyLimit <= 0 {
		return domain.UpstreamAccount{}, domain.NewValidationError("concurrency_limit", "concurrency_limit must be greater than zero")
	}
	if in.PriceBookID != "" {
		exists, err := s.repo.PriceBookExists(ctx, in.PriceBookID)
		if err != nil {
			return domain.UpstreamAccount{}, err
		}
		if !exists {
			return domain.UpstreamAccount{}, domain.NewValidationError("price_book_id", "price book does not exist")
		}
	}
	status, err := managedStatusOrDefault(in.Status)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	ciphertext, err := s.encrypt(in.APIKey)
	if err != nil {
		return domain.UpstreamAccount{}, domain.NewValidationError("api_key", err.Error())
	}
	protocol := in.DefaultProtocol
	if protocol == "" {
		protocol = string(domain.EndpointProtocolOpenAICompatible)
	}
	if err := validateEndpointProtocol(protocol); err != nil {
		return domain.UpstreamAccount{}, err
	}
	accessMode, err := upstreamaccess.NormalizeMode(in.TenantAccessMode)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	return s.repo.CreateAccount(ctx, AccountCreate{
		Name:              strings.TrimSpace(in.Name),
		TenantDisplayName: upstreamaccess.NormalizeDisplayName(in.Name, in.TenantDisplayName),
		TenantAccessMode:  accessMode,
		BaseURL:           in.BaseURL,
		Ciphertext:        ciphertext,
		ExtraHeaders:      in.ExtraHeaders,
		DefaultProtocol:   protocol,
		ConcurrencyLimit:  in.ConcurrencyLimit,
		PriceBookID:       in.PriceBookID,
		TenantMultiplier:  in.TenantMultiplier,
		Status:            status,
	})
}

func (s *Service) ListAccounts(ctx context.Context) ([]domain.UpstreamAccount, error) {
	return s.repo.ListAccounts(ctx)
}

func (s *Service) GetAccountSecret(ctx context.Context, id string) (AccountSecret, error) {
	return s.repo.GetAccountSecret(ctx, id)
}

type UpdateAccountInput struct {
	ID                string
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	BaseURL           string
	APIKey            string
	ExtraHeaders      []byte
	DefaultProtocol   string
	ConcurrencyLimit  *int
	PriceBookID       string
	TenantMultiplier  *float64
	Status            string
}

func (s *Service) UpdateAccount(ctx context.Context, in UpdateAccountInput) (domain.UpstreamAccount, error) {
	if in.Name == "" || in.BaseURL == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("", "name and base_url are required")
	}
	if in.ConcurrencyLimit != nil && *in.ConcurrencyLimit <= 0 {
		return domain.UpstreamAccount{}, domain.NewValidationError("concurrency_limit", "concurrency_limit must be greater than zero")
	}
	if in.PriceBookID != "" {
		exists, err := s.repo.PriceBookExists(ctx, in.PriceBookID)
		if err != nil {
			return domain.UpstreamAccount{}, err
		}
		if !exists {
			return domain.UpstreamAccount{}, domain.NewValidationError("price_book_id", "price book does not exist")
		}
	}
	current, err := s.repo.GetAccountSecret(ctx, in.ID)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	status := current.Status
	if strings.TrimSpace(in.Status) != "" {
		status, err = validateManagedStatus(in.Status)
		if err != nil {
			return domain.UpstreamAccount{}, err
		}
	}
	ciphertext := current.Ciphertext
	if in.APIKey != "" {
		ciphertext, err = s.encrypt(in.APIKey)
		if err != nil {
			return domain.UpstreamAccount{}, domain.NewValidationError("api_key", err.Error())
		}
	}
	protocol := in.DefaultProtocol
	if protocol == "" {
		protocol = current.DefaultProtocol
	}
	if err := validateEndpointProtocol(protocol); err != nil {
		return domain.UpstreamAccount{}, err
	}
	accessMode := current.TenantAccessMode
	if strings.TrimSpace(in.TenantAccessMode) != "" {
		accessMode, err = upstreamaccess.NormalizeMode(in.TenantAccessMode)
	}
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	displayName := current.TenantDisplayName
	if strings.TrimSpace(in.TenantDisplayName) != "" {
		displayName = upstreamaccess.NormalizeDisplayName(in.Name, in.TenantDisplayName)
	}
	if displayName == "" {
		displayName = upstreamaccess.NormalizeDisplayName(in.Name, "")
	}
	return s.repo.UpdateAccount(ctx, AccountUpdate{
		ID:                in.ID,
		Name:              strings.TrimSpace(in.Name),
		TenantDisplayName: displayName,
		TenantAccessMode:  accessMode,
		BaseURL:           in.BaseURL,
		Ciphertext:        ciphertext,
		ExtraHeaders:      in.ExtraHeaders,
		DefaultProtocol:   protocol,
		ConcurrencyLimit:  in.ConcurrencyLimit,
		PriceBookID:       in.PriceBookID,
		TenantMultiplier:  in.TenantMultiplier,
		Status:            status,
	})
}

func (s *Service) UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error) {
	if status == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("status", "status is required")
	}
	normalized, err := validateManagedStatus(status)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	return s.repo.UpdateAccountStatus(ctx, id, normalized)
}

// MarkAccountInvalid is the runtime-only transition used when an upstream
// rejects the direct account credential. Administrators cannot set this state.
func (s *Service) MarkAccountInvalid(ctx context.Context, id, reason string) (domain.UpstreamAccount, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("account_id", "account id is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "upstream rejected account credential"
	}
	return s.repo.MarkAccountInvalid(ctx, id, reason)
}

func (s *Service) DeleteAccount(ctx context.Context, id string) error {
	return s.repo.DeleteAccount(ctx, id)
}

func managedStatusOrDefault(status string) (string, error) {
	if status == "" {
		return domain.UpstreamAccountStatusDisabled, nil
	}
	return validateManagedStatus(status)
}

func validateManagedStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	switch status {
	case domain.UpstreamAccountStatusActive, domain.UpstreamAccountStatusDisabled:
		return status, nil
	default:
		return "", domain.NewValidationError("status", "status must be active or disabled")
	}
}

func validateEndpointProtocol(protocol string) error {
	switch protocol {
	case string(domain.EndpointProtocolOpenAICompatible), string(domain.EndpointProtocolAnthropic), string(domain.EndpointProtocolGemini):
		return nil
	default:
		return domain.NewValidationError("default_protocol", "default_protocol must be openai_compatible, anthropic, or gemini")
	}
}
