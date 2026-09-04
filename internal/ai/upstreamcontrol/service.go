package upstreamcontrol

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamaccess"
)

type Encryptor func(plaintext string) (string, error)

const RedactedHeaderValue = "***REDACTED***"

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
	APIKey            string
	Endpoints         []domain.UpstreamAccountEndpointWrite
	ConcurrencyLimit  *int
	PriceBookID       string
	TenantMultiplier  *float64
	Status            string
}

func (s *Service) CreateAccount(ctx context.Context, in CreateAccountInput) (domain.UpstreamAccount, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.APIKey) == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("", "name and api_key are required")
	}
	if len(in.Endpoints) == 0 {
		return domain.UpstreamAccount{}, domain.NewValidationError("endpoints", "at least one endpoint is required")
	}
	endpoints, err := NormalizeEndpointWrites(in.Endpoints)
	if err != nil {
		return domain.UpstreamAccount{}, err
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
	if status == domain.UpstreamAccountStatusActive && !hasActiveEndpointWrite(endpoints) {
		return domain.UpstreamAccount{}, domain.NewValidationError("endpoints", "active account requires at least one active endpoint")
	}
	ciphertext, err := s.encrypt(in.APIKey)
	if err != nil {
		return domain.UpstreamAccount{}, domain.NewValidationError("api_key", err.Error())
	}
	accessMode, err := upstreamaccess.NormalizeMode(in.TenantAccessMode)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	return s.repo.CreateAccount(ctx, AccountCreate{
		Name:              strings.TrimSpace(in.Name),
		TenantDisplayName: upstreamaccess.NormalizeDisplayName(in.Name, in.TenantDisplayName),
		TenantAccessMode:  accessMode,
		Ciphertext:        ciphertext,
		Endpoints:         endpoints,
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
	APIKey            string
	ConcurrencyLimit  *int
	PriceBookID       string
	TenantMultiplier  *float64
	Status            string
}

func (s *Service) UpdateAccount(ctx context.Context, in UpdateAccountInput) (domain.UpstreamAccount, error) {
	if strings.TrimSpace(in.Name) == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("name", "name is required")
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
		Ciphertext:        ciphertext,
		ConcurrencyLimit:  in.ConcurrencyLimit,
		PriceBookID:       in.PriceBookID,
		TenantMultiplier:  in.TenantMultiplier,
		Status:            status,
	})
}

func (s *Service) UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error) {
	id = strings.TrimSpace(id)
	if status == "" {
		return domain.UpstreamAccount{}, domain.NewValidationError("status", "status is required")
	}
	normalized, err := validateManagedStatus(status)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	if normalized == domain.UpstreamAccountStatusActive {
		endpoints, err := s.repo.ListEndpoints(ctx, id)
		if err != nil {
			return domain.UpstreamAccount{}, err
		}
		if !hasActiveEndpoint(endpoints) {
			return domain.UpstreamAccount{}, domain.NewValidationError("status", "account requires at least one active endpoint before it can be enabled")
		}
	}
	return s.repo.UpdateAccountStatus(ctx, id, normalized)
}

func hasActiveEndpointWrite(items []domain.UpstreamAccountEndpointWrite) bool {
	for _, item := range items {
		if item.Status == domain.EndpointStatusActive {
			return true
		}
	}
	return false
}

func hasActiveEndpoint(items []domain.UpstreamAccountEndpoint) bool {
	for _, item := range items {
		if item.Status == domain.EndpointStatusActive {
			return true
		}
	}
	return false
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

func (s *Service) ListEndpoints(ctx context.Context, accountID string) ([]domain.UpstreamAccountEndpoint, error) {
	accountID = strings.TrimSpace(accountID)
	if _, err := s.repo.GetAccountSecret(ctx, accountID); err != nil {
		return nil, err
	}
	return s.repo.ListEndpoints(ctx, accountID)
}

func (s *Service) GetEndpoint(ctx context.Context, accountID, endpointID string) (domain.UpstreamAccountEndpoint, error) {
	return s.repo.GetEndpoint(ctx, strings.TrimSpace(accountID), strings.TrimSpace(endpointID))
}

func (s *Service) CreateEndpoint(ctx context.Context, accountID string, write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpoint, error) {
	accountID = strings.TrimSpace(accountID)
	if _, err := s.repo.GetAccountSecret(ctx, accountID); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	normalized, err := normalizeEndpointWrite(write)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	if err := rejectUnboundRedactedHeaders(normalized.ExtraHeaders); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	if err := s.ensureEndpointFormatAvailable(ctx, accountID, "", normalized.APIFormat); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	return s.repo.CreateEndpoint(ctx, accountID, normalized)
}

func (s *Service) UpdateEndpoint(ctx context.Context, accountID, endpointID string, write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpoint, error) {
	accountID, endpointID = strings.TrimSpace(accountID), strings.TrimSpace(endpointID)
	current, err := s.repo.GetEndpoint(ctx, accountID, endpointID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	normalized, err := normalizeEndpointWrite(write)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	normalized.ExtraHeaders, err = preserveRedactedExtraHeaders(current.ExtraHeaders, normalized.ExtraHeaders)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	if err := s.ensureEndpointFormatAvailable(ctx, accountID, endpointID, normalized.APIFormat); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	if normalized.Status == domain.EndpointStatusDisabled {
		items, err := s.repo.ListEndpoints(ctx, accountID)
		if err != nil {
			return domain.UpstreamAccountEndpoint{}, err
		}
		active := 0
		for _, item := range items {
			if item.Status == domain.EndpointStatusActive && item.ID != endpointID {
				active++
			}
		}
		secret, err := s.repo.GetAccountSecret(ctx, accountID)
		if err != nil {
			return domain.UpstreamAccountEndpoint{}, err
		}
		if secret.Status == domain.UpstreamAccountStatusActive && active == 0 {
			return domain.UpstreamAccountEndpoint{}, domain.NewValidationError("status", "active account must keep at least one active endpoint")
		}
	}
	return s.repo.UpdateEndpoint(ctx, accountID, endpointID, normalized)
}

func (s *Service) UpdateEndpointHealth(ctx context.Context, accountID, endpointID string, health domain.HealthStatus, lastError string) (domain.UpstreamAccountEndpoint, error) {
	switch health {
	case domain.HealthUnknown, domain.HealthHealthy, domain.HealthUnhealthy:
	default:
		return domain.UpstreamAccountEndpoint{}, domain.NewValidationError("health_status", "unsupported health status")
	}
	return s.repo.UpdateEndpointHealth(ctx, strings.TrimSpace(accountID), strings.TrimSpace(endpointID), health, strings.TrimSpace(lastError))
}

func (s *Service) DeleteEndpoint(ctx context.Context, accountID, endpointID string) error {
	accountID, endpointID = strings.TrimSpace(accountID), strings.TrimSpace(endpointID)
	endpoint, err := s.repo.GetEndpoint(ctx, accountID, endpointID)
	if err != nil {
		return err
	}
	items, err := s.repo.ListEndpoints(ctx, accountID)
	if err != nil {
		return err
	}
	if len(items) <= 1 {
		return domain.NewValidationError("endpoint_id", "account must keep at least one endpoint")
	}
	if endpoint.Status == domain.EndpointStatusActive {
		secret, err := s.repo.GetAccountSecret(ctx, accountID)
		if err != nil {
			return err
		}
		if secret.Status == domain.UpstreamAccountStatusActive {
			activeOthers := 0
			for _, item := range items {
				if item.ID != endpointID && item.Status == domain.EndpointStatusActive {
					activeOthers++
				}
			}
			if activeOthers == 0 {
				return domain.NewValidationError("endpoint_id", "active account must keep at least one active endpoint")
			}
		}
	}
	return s.repo.DeleteEndpoint(ctx, accountID, endpointID)
}

func (s *Service) ensureEndpointFormatAvailable(ctx context.Context, accountID, excludeID string, format domain.UpstreamProtocol) error {
	items, err := s.repo.ListEndpoints(ctx, accountID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID != excludeID && item.APIFormat == format {
			return domain.NewValidationError("api_format", "account already has an endpoint for this API format")
		}
	}
	return nil
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

// NormalizeEndpointWrites validates and canonicalizes one account's endpoint set.
// It is also used by import preview so preview and commit cannot drift.
func NormalizeEndpointWrites(items []domain.UpstreamAccountEndpointWrite) ([]domain.UpstreamAccountEndpointWrite, error) {
	out := make([]domain.UpstreamAccountEndpointWrite, 0, len(items))
	seen := make(map[domain.UpstreamProtocol]struct{}, len(items))
	for _, item := range items {
		normalized, err := normalizeEndpointWrite(item)
		if err != nil {
			return nil, err
		}
		if err := rejectUnboundRedactedHeaders(normalized.ExtraHeaders); err != nil {
			return nil, err
		}
		if _, exists := seen[normalized.APIFormat]; exists {
			return nil, domain.NewValidationError("endpoints", "each API format may only be configured once")
		}
		seen[normalized.APIFormat] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeEndpointWrite(write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpointWrite, error) {
	switch write.APIFormat {
	case domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses,
		domain.ProtocolOpenAIEmbeddings, domain.ProtocolOpenAIImages,
		domain.ProtocolAnthropicMessages, domain.ProtocolGeminiGenerate,
		domain.ProtocolGeminiEmbeddings:
	default:
		return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("api_format", "unsupported API format")
	}
	write.BaseURL = strings.TrimRight(strings.TrimSpace(write.BaseURL), "/")
	if write.BaseURL == "" {
		return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("base_url", "base_url is required")
	}
	parsedBaseURL, err := url.Parse(write.BaseURL)
	if err != nil || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") || parsedBaseURL.Host == "" || parsedBaseURL.User != nil || parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" {
		return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("base_url", "base_url must be an http(s) origin or base path without credentials, query, or fragment")
	}
	write.PathOverride = strings.TrimSpace(write.PathOverride)
	if write.PathOverride != "" && !strings.HasPrefix(write.PathOverride, "/") {
		write.PathOverride = "/" + write.PathOverride
	}
	write.AuthScheme = strings.TrimSpace(write.AuthScheme)
	if write.AuthScheme == "" {
		write.AuthScheme = domain.EndpointAuthFormatDefault
	}
	switch write.AuthScheme {
	case domain.EndpointAuthFormatDefault, domain.EndpointAuthBearer,
		domain.EndpointAuthAnthropicAPIKey, domain.EndpointAuthGeminiAPIKey,
		domain.EndpointAuthCustomHeader:
	default:
		return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("auth_scheme", "unsupported endpoint auth scheme")
	}
	write.AuthHeader = strings.TrimSpace(write.AuthHeader)
	if write.AuthScheme == domain.EndpointAuthCustomHeader && write.AuthHeader == "" {
		return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("auth_header", "auth_header is required for custom_header auth")
	}
	write.Status = strings.TrimSpace(write.Status)
	if write.Status == "" {
		write.Status = domain.EndpointStatusActive
	}
	if write.Status != domain.EndpointStatusActive && write.Status != domain.EndpointStatusDisabled {
		return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("status", "endpoint status must be active or disabled")
	}
	if len(write.ExtraHeaders) > 0 {
		var headers map[string]json.RawMessage
		if !json.Valid(write.ExtraHeaders) || json.Unmarshal(write.ExtraHeaders, &headers) != nil || headers == nil {
			return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("extra_headers", "extra_headers must be a JSON object")
		}
		normalizedHeaders := make(map[string]string, len(headers))
		for key, rawValue := range headers {
			key = strings.TrimSpace(key)
			if key == "" {
				return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("extra_headers", "header names must not be empty")
			}
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return domain.UpstreamAccountEndpointWrite{}, domain.NewValidationError("extra_headers", "header values must be strings")
			}
			normalizedHeaders[key] = value
		}
		write.ExtraHeaders, _ = json.Marshal(normalizedHeaders)
	}
	return write, nil
}

func preserveRedactedExtraHeaders(currentRaw, replacementRaw []byte) ([]byte, error) {
	if len(replacementRaw) == 0 {
		return replacementRaw, nil
	}
	var current map[string]json.RawMessage
	var replacement map[string]string
	if len(currentRaw) > 0 {
		if err := json.Unmarshal(currentRaw, &current); err != nil {
			return nil, domain.NewValidationError("extra_headers", "stored extra_headers are invalid")
		}
	}
	if err := json.Unmarshal(replacementRaw, &replacement); err != nil {
		return nil, domain.NewValidationError("extra_headers", "extra_headers must be a JSON object")
	}
	for key, value := range replacement {
		if value != RedactedHeaderValue || !IsSensitiveHeaderKey(key) {
			continue
		}
		originalRaw, ok := current[key]
		if !ok {
			for currentKey, currentValue := range current {
				if strings.EqualFold(currentKey, key) {
					originalRaw, ok = currentValue, true
					break
				}
			}
		}
		if !ok {
			return nil, domain.NewValidationError("extra_headers", "redacted header placeholders may only preserve existing sensitive headers")
		}
		var original string
		if err := json.Unmarshal(originalRaw, &original); err != nil {
			return nil, domain.NewValidationError("extra_headers", "stored sensitive header values must be strings")
		}
		replacement[key] = original
	}
	return json.Marshal(replacement)
}

func rejectUnboundRedactedHeaders(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return domain.NewValidationError("extra_headers", "extra_headers must be a JSON object")
	}
	for key, value := range headers {
		if value == RedactedHeaderValue && IsSensitiveHeaderKey(key) {
			return domain.NewValidationError("extra_headers", "redacted header placeholders are only valid when editing an existing endpoint")
		}
	}
	return nil
}

func IsSensitiveHeaderKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, part := range []string{"authorization", "proxy-authorization", "cookie", "token", "secret", "password", "api-key", "api_key", "apikey", "key"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}
