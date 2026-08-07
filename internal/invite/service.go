package invite

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"xiaodou/dai/internal/auth"
	shared "xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/invite/pg"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type inviteRepository interface {
	GetByCode(code string) (*pg.InvitationCode, error)
	GetTenantPublicBrand(tenantID string) (*pg.TenantPublicBrand, error)
	GetByTenantID(tenantID string, page, size int) ([]*pg.InvitationCode, int64, error)
	Create(ic *pg.InvitationCode) error
	Update(id int64, updates map[string]any) error
	Delete(id int64) error
	CheckEndUserUsernameExists(username string) (bool, error)
	RegisterEndUser(ctx context.Context, input pg.EndUserRegistration) error
}

// InviteService 邀请码服务
type InviteService struct {
	repo   inviteRepository
	logger *zap.Logger
}

type PublicInvitationStatus string

const (
	PublicInvitationStatusActive   PublicInvitationStatus = "active"
	PublicInvitationStatusExpired  PublicInvitationStatus = "expired"
	PublicInvitationStatusDisabled PublicInvitationStatus = "disabled"
	PublicInvitationStatusUsedUp   PublicInvitationStatus = "used_up"
	PublicInvitationStatusNotFound PublicInvitationStatus = "not_found"
)

type PublicInvitationView struct {
	Code             string
	TenantID         string
	TenantName       string
	CustomerSiteName string
	FaviconVersion   int64
	Description      string
	ExpireTime       *int64
	Status           PublicInvitationStatus
	CanRegister      bool
	Message          string
}

type RegisteredEndUser struct {
	UserID   string
	Username string
	TenantID string
	UserType int
}

type LegalAcceptance struct {
	TermsVersion   string
	PrivacyVersion string
}

var invitationCodePattern = regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{8}$`)
var ErrInvalidInvitationCodeFormat = errors.New("invalid invitation code format")
var ErrInvitationCodeUnavailable = errors.New("invitation code is invalid, expired, or exhausted")
var ErrUsernameExists = errors.New("username already exists")
var ErrInvalidUsername = errors.New("invalid username")
var ErrLegalAcceptanceRequired = errors.New("current legal documents must be accepted")

func NewInviteService(repo inviteRepository, logger *zap.Logger) *InviteService {
	return &InviteService{repo: repo, logger: logger}
}

func (s *InviteService) ValidateCode(ctx context.Context, code string) (*pg.InvitationCode, error) {
	normalizedCode, err := normalizeInvitationCodeChecked(code)
	if err != nil {
		return nil, err
	}
	ic, err := s.repo.GetByCode(normalizedCode)
	if err != nil {
		if errors.Is(err, pg.ErrInvitationCodeNotFound) {
			return nil, pg.ErrInvitationCodeNotFound
		}
		return nil, fmt.Errorf("load invitation code: %w", err)
	}
	if !ic.IsValid() {
		return nil, ErrInvitationCodeUnavailable
	}
	return ic, nil
}

func (s *InviteService) DescribePublicInvitation(ctx context.Context, code string) (*PublicInvitationView, error) {
	normalizedCode, err := normalizeInvitationCodeChecked(code)
	if err != nil {
		return nil, err
	}

	ic, err := s.repo.GetByCode(normalizedCode)
	if err != nil {
		if errors.Is(err, pg.ErrInvitationCodeNotFound) {
			return &PublicInvitationView{
				Code:        normalizedCode,
				Status:      PublicInvitationStatusNotFound,
				CanRegister: false,
				Message:     "邀请码不存在",
			}, nil
		}
		return nil, fmt.Errorf("load invitation code: %w", err)
	}

	brand, err := s.repo.GetTenantPublicBrand(ic.TenantID)
	if err != nil {
		return nil, fmt.Errorf("load tenant public brand: %w", err)
	}

	view := &PublicInvitationView{
		Code:             normalizedCode,
		TenantID:         ic.TenantID,
		TenantName:       brand.TenantName,
		CustomerSiteName: brand.CustomerSiteName,
		Description:      ic.Description,
		ExpireTime:       ic.ExpireTime,
		Status:           PublicInvitationStatusActive,
		CanRegister:      true,
	}
	if view.CustomerSiteName == "" {
		view.CustomerSiteName = view.TenantName
	}
	if brand.FaviconUpdatedAt != nil {
		view.FaviconVersion = brand.FaviconUpdatedAt.UnixMilli()
	}

	switch {
	case ic.Status != 1:
		view.Status = PublicInvitationStatusDisabled
		view.CanRegister = false
		view.Message = "邀请码已停用"
	case ic.ExpireTime != nil && *ic.ExpireTime < nowMillis():
		view.Status = PublicInvitationStatusExpired
		view.CanRegister = false
		view.Message = "邀请码已过期"
	case ic.MaxUses > 0 && ic.UsedCount >= ic.MaxUses:
		view.Status = PublicInvitationStatusUsedUp
		view.CanRegister = false
		view.Message = "邀请码已达使用上限"
	}

	return view, nil
}

func (s *InviteService) ListCodes(ctx context.Context, tenantID string, page, size int) ([]*pg.InvitationCode, int64, error) {
	return s.repo.GetByTenantID(tenantID, page, size)
}

func (s *InviteService) CreateCode(ctx context.Context, tenantID, createdBy, description string, maxUses int, expireTime *int64) (*pg.InvitationCode, error) {
	code := shared.GenerateRandomString(8)
	ic := &pg.InvitationCode{
		Code:        code,
		TenantID:    tenantID,
		CreatedBy:   createdBy,
		Description: description,
		MaxUses:     maxUses,
		ExpireTime:  expireTime,
	}
	if err := s.repo.Create(ic); err != nil {
		return nil, fmt.Errorf("failed to create invitation code: %w", err)
	}
	s.logger.Info("Invitation code created", zap.String("code", code), zap.String("tenantId", tenantID))
	return ic, nil
}

func (s *InviteService) UpdateCode(ctx context.Context, id int64, updates map[string]any) error {
	return s.repo.Update(id, updates)
}

func (s *InviteService) DeleteCode(ctx context.Context, id int64) error {
	return s.repo.Delete(id)
}

func (s *InviteService) RegisterUser(ctx context.Context, code, username, password string, email, phone *string, legal LegalAcceptance) (*RegisteredEndUser, error) {
	normalizedCode, err := normalizeInvitationCodeChecked(code)
	if err != nil {
		return nil, err
	}
	normalizedUsername, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}
	email = normalizeOptionalString(email)
	phone = normalizeOptionalString(phone)
	legal.TermsVersion = strings.TrimSpace(legal.TermsVersion)
	legal.PrivacyVersion = strings.TrimSpace(legal.PrivacyVersion)
	if legal.TermsVersion == "" || legal.PrivacyVersion == "" {
		return nil, ErrLegalAcceptanceRequired
	}
	ic, err := s.ValidateCode(ctx, normalizedCode)
	if err != nil {
		return nil, err
	}

	exists, err := s.repo.CheckEndUserUsernameExists(normalizedUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to check username uniqueness: %w", err)
	}
	if exists {
		return nil, ErrUsernameExists
	}

	userID := "U_" + shared.GenerateRandomString(16)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.repo.RegisterEndUser(ctx, pg.EndUserRegistration{
		InvitationCode: normalizedCode,
		TenantID:       ic.TenantID,
		UserID:         userID,
		Username:       normalizedUsername,
		PasswordHash:   string(hash),
		Email:          email,
		Phone:          phone,
		LegalAcceptances: []pg.LegalAcceptance{
			{DocumentKey: "terms", Version: legal.TermsVersion},
			{DocumentKey: "privacy", Version: legal.PrivacyVersion},
		},
	}); err != nil {
		if errors.Is(err, pg.ErrInvitationCodeUnavailable) {
			return nil, ErrInvitationCodeUnavailable
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("User registered via invitation code",
		zap.String("userId", userID),
		zap.String("tenantId", ic.TenantID),
		zap.String("inviteCode", normalizedCode),
	)
	return &RegisteredEndUser{
		UserID:   userID,
		Username: normalizedUsername,
		TenantID: ic.TenantID,
		UserType: 4,
	}, nil
}

func normalizeInvitationCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func normalizeInvitationCodeChecked(code string) (string, error) {
	normalized := normalizeInvitationCode(code)
	if !invitationCodePattern.MatchString(normalized) {
		return "", ErrInvalidInvitationCodeFormat
	}
	return normalized, nil
}

func normalizeUsername(username string) (string, error) {
	normalized := auth.NormalizeUsername(username)
	if normalized == "" {
		return "", ErrInvalidUsername
	}
	return auth.NormalizeEndUsername(normalized), nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
