package tenant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tenantports "xiaodou/dai/internal/tenant/ports"
)

type selfReaderStub struct {
	user tenantports.TenantUser
}

func (s *selfReaderStub) GetByUserID(context.Context, string) (*tenantports.TenantUser, error) {
	return &s.user, nil
}

func (s *selfReaderStub) ListInvitationCodes(context.Context, string, int, int) ([]tenantports.InviteCodeItem, int64, error) {
	return []tenantports.InviteCodeItem{}, 0, nil
}

func (s *selfReaderStub) GetTenantOverviewStats(context.Context, string, *time.Time, *time.Time) (*tenantports.TenantOverviewStats, error) {
	return nil, nil
}

func (s *selfReaderStub) GetClientConsumption(context.Context, string, *time.Time, *time.Time) ([]tenantports.ClientConsumptionItem, error) {
	return nil, nil
}

func (s *selfReaderStub) GetUserConsumptionRanking(context.Context, string, *time.Time, *time.Time, int) ([]tenantports.UserConsumptionItem, error) {
	return nil, nil
}

type invitationWriterStub struct {
	attempts int
	commands []string
	err      error
}

func (s *invitationWriterStub) CreateInvitationCode(_ context.Context, code, tenantID, createdBy, description string, maxUses int, expireTime *int64) error {
	s.attempts++
	s.commands = append(s.commands, strings.Join([]string{code, tenantID, createdBy, description}, ":"))
	if s.err != nil {
		if errors.Is(s.err, tenantports.ErrInvitationCodeTaken) {
			if s.attempts == 1 {
				return s.err
			}
		} else {
			return s.err
		}
	}
	return nil
}

func (s *invitationWriterStub) UpdateInvitationCode(context.Context, int64, string, int, string) error {
	return nil
}

func (s *invitationWriterStub) DeleteInvitationCode(context.Context, int64, string) error {
	return nil
}

func TestSelfServiceRetriesInvitationCodeCollision(t *testing.T) {
	writer := &invitationWriterStub{err: tenantports.ErrInvitationCodeTaken}
	service := NewSelfService(&selfReaderStub{}, writer)

	item, err := service.CreateInvitation(context.Background(), tenantports.InvitationCreateCommand{
		TenantID: "tenant-1", CreatedBy: "user-1", Description: "onboarding", MaxUses: 3,
	})
	if err != nil {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
	if writer.attempts != 2 {
		t.Fatalf("writer attempts = %d, want 2", writer.attempts)
	}
	if len(item.Code) != 8 || item.TenantID != "tenant-1" || item.Description != "onboarding" || item.MaxUses != 3 {
		t.Fatalf("unexpected invitation result: %#v", item)
	}
	if item.Code == writer.commands[0][:8] {
		t.Fatal("expected a new code after collision")
	}
}

func TestSelfServiceDoesNotRetryNonCollision(t *testing.T) {
	writer := &invitationWriterStub{err: errors.New("database unavailable")}
	service := NewSelfService(&selfReaderStub{}, writer)

	_, err := service.CreateInvitation(context.Background(), tenantports.InvitationCreateCommand{TenantID: "tenant-1"})
	if !errors.Is(err, writer.err) {
		t.Fatalf("CreateInvitation() error = %v, want %v", err, writer.err)
	}
	if writer.attempts != 1 {
		t.Fatalf("writer attempts = %d, want 1", writer.attempts)
	}
}

func TestSelfServiceReportsMissingCapabilities(t *testing.T) {
	service := NewSelfService(nil, nil)
	if _, err := service.GetByUserID(context.Background(), "user-1"); !errors.Is(err, tenantports.ErrSelfServiceUnavailable) {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	if _, err := service.CreateInvitation(context.Background(), tenantports.InvitationCreateCommand{}); !errors.Is(err, tenantports.ErrSelfServiceUnavailable) {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
}
