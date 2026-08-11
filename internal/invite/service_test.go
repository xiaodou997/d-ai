package invite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xiaodou/dai/internal/invite/pg"

	"go.uber.org/zap"
)

type inviteRepoStub struct {
	getByCodeFn                  func(string) (*pg.InvitationCode, error)
	getTenantPublicBrandFn       func(string) (*pg.TenantPublicBrand, error)
	getByTenantIDFn              func(string, int, int) ([]*pg.InvitationCode, int64, error)
	createFn                     func(*pg.InvitationCode) error
	updateFn                     func(int64, map[string]any) error
	deleteFn                     func(int64) error
	incrUsedCountFn              func(string) error
	checkEndUserUsernameExistsFn func(string) (bool, error)
	createEndUserFn              func(string, string, string, string, *string, *string) error
	registerEndUserFn            func(context.Context, pg.EndUserRegistration) error
}

func (s *inviteRepoStub) GetByCode(code string) (*pg.InvitationCode, error) {
	return s.getByCodeFn(code)
}

func (s *inviteRepoStub) GetTenantPublicBrand(tenantID string) (*pg.TenantPublicBrand, error) {
	if s.getTenantPublicBrandFn == nil {
		return &pg.TenantPublicBrand{}, nil
	}
	return s.getTenantPublicBrandFn(tenantID)
}

func (s *inviteRepoStub) GetByTenantID(tenantID string, page, size int) ([]*pg.InvitationCode, int64, error) {
	if s.getByTenantIDFn == nil {
		return nil, 0, nil
	}
	return s.getByTenantIDFn(tenantID, page, size)
}

func (s *inviteRepoStub) Create(ic *pg.InvitationCode) error {
	if s.createFn == nil {
		return nil
	}
	return s.createFn(ic)
}

func (s *inviteRepoStub) Update(id int64, updates map[string]any) error {
	if s.updateFn == nil {
		return nil
	}
	return s.updateFn(id, updates)
}

func (s *inviteRepoStub) Delete(id int64) error {
	if s.deleteFn == nil {
		return nil
	}
	return s.deleteFn(id)
}

func (s *inviteRepoStub) IncrUsedCount(code string) error {
	if s.incrUsedCountFn == nil {
		return nil
	}
	return s.incrUsedCountFn(code)
}

func (s *inviteRepoStub) CheckEndUserUsernameExists(username string) (bool, error) {
	if s.checkEndUserUsernameExistsFn == nil {
		return false, nil
	}
	return s.checkEndUserUsernameExistsFn(username)
}

func (s *inviteRepoStub) CreateEndUser(tenantID, userID, username, passwordHash string, email, phone *string) error {
	if s.createEndUserFn == nil {
		return nil
	}
	return s.createEndUserFn(tenantID, userID, username, passwordHash, email, phone)
}

func (s *inviteRepoStub) RegisterEndUser(ctx context.Context, input pg.EndUserRegistration) error {
	if s.registerEndUserFn != nil {
		return s.registerEndUserFn(ctx, input)
	}
	return s.CreateEndUser(input.TenantID, input.UserID, input.Username, input.PasswordHash, input.Email, input.Phone)
}

func TestValidateCodeNormalizesInputBeforeLookup(t *testing.T) {
	var received string
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			received = code
			return &pg.InvitationCode{Code: code, Status: 1}, nil
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	ic, err := svc.ValidateCode(context.Background(), " s6jumxvh \n")
	if err != nil {
		t.Fatalf("ValidateCode returned error: %v", err)
	}
	if received != "S6JUMXVH" {
		t.Fatalf("expected normalized code S6JUMXVH, got %q", received)
	}
	if ic.Code != "S6JUMXVH" {
		t.Fatalf("expected invitation code S6JUMXVH, got %q", ic.Code)
	}
}

func TestValidateCodePreservesRepositoryErrors(t *testing.T) {
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			return nil, errors.New("db timeout")
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	_, err := svc.ValidateCode(context.Background(), "S6JUMXVH")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load invitation code") {
		t.Fatalf("expected wrapped repository error, got %v", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Fatalf("did not expect not-found error, got %v", err)
	}
}

func TestValidateCodeReturnsNotFoundForMissingCode(t *testing.T) {
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			return nil, pg.ErrInvitationCodeNotFound
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	_, err := svc.ValidateCode(context.Background(), "S6JUMXVH")
	if !errors.Is(err, pg.ErrInvitationCodeNotFound) {
		t.Fatalf("expected ErrInvitationCodeNotFound, got %v", err)
	}
}

func TestValidateCodeRejectsInvalidFormat(t *testing.T) {
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			t.Fatalf("repository should not be called for invalid code %q", code)
			return nil, nil
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	_, err := svc.ValidateCode(context.Background(), "bad-code")
	if !errors.Is(err, ErrInvalidInvitationCodeFormat) {
		t.Fatalf("expected ErrInvalidInvitationCodeFormat, got %v", err)
	}
}

func TestDescribePublicInvitationMapsStatuses(t *testing.T) {
	tests := []struct {
		name    string
		record  *pg.InvitationCode
		status  PublicInvitationStatus
		canUse  bool
		message string
	}{
		{
			name:    "active",
			record:  &pg.InvitationCode{Code: "S6JUMXVH", TenantID: "T_1", Status: 1, MaxUses: 0},
			status:  PublicInvitationStatusActive,
			canUse:  true,
			message: "",
		},
		{
			name:    "disabled",
			record:  &pg.InvitationCode{Code: "S6JUMXVH", TenantID: "T_1", Status: 2},
			status:  PublicInvitationStatusDisabled,
			canUse:  false,
			message: "邀请码已停用",
		},
		{
			name:    "used up",
			record:  &pg.InvitationCode{Code: "S6JUMXVH", TenantID: "T_1", Status: 1, MaxUses: 1, UsedCount: 1},
			status:  PublicInvitationStatusUsedUp,
			canUse:  false,
			message: "邀请码已达使用上限",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &inviteRepoStub{
				getByCodeFn: func(code string) (*pg.InvitationCode, error) {
					return tt.record, nil
				},
				getTenantPublicBrandFn: func(tenantID string) (*pg.TenantPublicBrand, error) {
					return &pg.TenantPublicBrand{TenantName: "Tenant One"}, nil
				},
			}
			svc := NewInviteService(repo, zap.NewNop())

			view, err := svc.DescribePublicInvitation(context.Background(), "s6jumxvh")
			if err != nil {
				t.Fatalf("DescribePublicInvitation returned error: %v", err)
			}
			if view.Status != tt.status {
				t.Fatalf("expected status %q, got %q", tt.status, view.Status)
			}
			if view.CanRegister != tt.canUse {
				t.Fatalf("expected canRegister=%v, got %v", tt.canUse, view.CanRegister)
			}
			if view.Message != tt.message {
				t.Fatalf("expected message %q, got %q", tt.message, view.Message)
			}
			if view.TenantName != "Tenant One" {
				t.Fatalf("expected tenant name Tenant One, got %q", view.TenantName)
			}
		})
	}
}

func TestDescribePublicInvitationReturnsNotFoundView(t *testing.T) {
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			return nil, pg.ErrInvitationCodeNotFound
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	view, err := svc.DescribePublicInvitation(context.Background(), "S6JUMXVH")
	if err != nil {
		t.Fatalf("DescribePublicInvitation returned error: %v", err)
	}
	if view.Status != PublicInvitationStatusNotFound {
		t.Fatalf("expected status %q, got %q", PublicInvitationStatusNotFound, view.Status)
	}
	if view.CanRegister {
		t.Fatal("expected canRegister=false")
	}
}

func TestRegisterUserReturnsRegisteredUser(t *testing.T) {
	email := "user@example.com"
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			return &pg.InvitationCode{Code: code, TenantID: "T_1", Status: 1}, nil
		},
		registerEndUserFn: func(_ context.Context, input pg.EndUserRegistration) error {
			if input.TenantID != "T_1" {
				t.Fatalf("expected tenantID T_1, got %q", input.TenantID)
			}
			if input.Username != "alice" {
				t.Fatalf("expected username alice, got %q", input.Username)
			}
			if input.Email == nil || *input.Email != "user@example.com" {
				t.Fatalf("expected email user@example.com, got %#v", input.Email)
			}
			if input.PasswordHash == "" {
				t.Fatal("expected password hash to be set")
			}
			if len(input.LegalAcceptances) != 2 || input.LegalAcceptances[0] != (pg.LegalAcceptance{DocumentKey: "terms", Version: "2026-07-19"}) || input.LegalAcceptances[1] != (pg.LegalAcceptance{DocumentKey: "privacy", Version: "2026-07-19"}) {
				t.Fatalf("unexpected legal acceptances: %#v", input.LegalAcceptances)
			}
			return nil
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	user, err := svc.RegisterUser(context.Background(), "s6jumxvh", "alice", "secret123", &email, nil, LegalAcceptance{
		TermsVersion: "2026-07-19", PrivacyVersion: "2026-07-19",
	})
	if err != nil {
		t.Fatalf("RegisterUser returned error: %v", err)
	}
	if user.TenantID != "T_1" || user.Username != "alice" || user.UserType != 4 || user.UserID == "" {
		t.Fatalf("unexpected registered user: %#v", user)
	}
}

func TestRegisterUserRejectsBlankUsername(t *testing.T) {
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			return &pg.InvitationCode{Code: code, TenantID: "T_1", Status: 1}, nil
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	_, err := svc.RegisterUser(context.Background(), "S6JUMXVH", "   ", "secret123", nil, nil, LegalAcceptance{
		TermsVersion: "2026-07-19", PrivacyVersion: "2026-07-19",
	})
	if !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got %v", err)
	}
}

func TestRegisterUserRequiresBothCurrentLegalDocumentVersions(t *testing.T) {
	repo := &inviteRepoStub{
		getByCodeFn: func(code string) (*pg.InvitationCode, error) {
			return &pg.InvitationCode{Code: code, TenantID: "T_1", Status: 1}, nil
		},
	}
	svc := NewInviteService(repo, zap.NewNop())

	_, err := svc.RegisterUser(context.Background(), "S6JUMXVH", "alice", "secret123", nil, nil, LegalAcceptance{})
	if !errors.Is(err, ErrLegalAcceptanceRequired) {
		t.Fatalf("expected ErrLegalAcceptanceRequired, got %v", err)
	}
}
