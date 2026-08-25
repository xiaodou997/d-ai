package tenant

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	tenantports "xiaodou/dai/internal/tenant/ports"
)

const invitationCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// SelfService coordinates tenant self-service queries and invitation commands.
// Persistence remains split into reader/writer ports so the HTTP layer never
// needs to know which PostgreSQL repository implements either side.
type SelfService struct {
	reader      tenantports.TenantSelfReader
	invitations tenantports.TenantInvitationWriter
}

var _ tenantports.TenantSelfService = (*SelfService)(nil)

func NewSelfService(reader tenantports.TenantSelfReader, invitations tenantports.TenantInvitationWriter) *SelfService {
	return &SelfService{reader: reader, invitations: invitations}
}

func (s *SelfService) GetByUserID(ctx context.Context, userID string) (*tenantports.TenantUser, error) {
	if s == nil || s.reader == nil {
		return nil, tenantports.ErrSelfServiceUnavailable
	}
	return s.reader.GetByUserID(ctx, userID)
}

func (s *SelfService) ListInvitationCodes(ctx context.Context, tenantID string, page, size int) ([]tenantports.InviteCodeItem, int64, error) {
	if s == nil || s.reader == nil {
		return nil, 0, tenantports.ErrSelfServiceUnavailable
	}
	return s.reader.ListInvitationCodes(ctx, tenantID, page, size)
}

func (s *SelfService) CreateInvitation(ctx context.Context, input tenantports.InvitationCreateCommand) (tenantports.InviteCodeItem, error) {
	if s == nil || s.invitations == nil {
		return tenantports.InviteCodeItem{}, tenantports.ErrSelfServiceUnavailable
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateInvitationCode()
		if err != nil {
			return tenantports.InviteCodeItem{}, err
		}
		err = s.invitations.CreateInvitationCode(ctx, code, input.TenantID, input.CreatedBy, input.Description, input.MaxUses, input.ExpireTime)
		if err == nil {
			return tenantports.InviteCodeItem{
				Code:        code,
				TenantID:    input.TenantID,
				Description: input.Description,
				MaxUses:     input.MaxUses,
				ExpireTime:  input.ExpireTime,
			}, nil
		}
		lastErr = err
		if !errors.Is(err, tenantports.ErrInvitationCodeTaken) {
			break
		}
	}
	return tenantports.InviteCodeItem{}, lastErr
}

func (s *SelfService) UpdateInvitation(ctx context.Context, input tenantports.InvitationUpdateCommand) error {
	if s == nil || s.invitations == nil {
		return tenantports.ErrSelfServiceUnavailable
	}
	return s.invitations.UpdateInvitationCode(ctx, input.ID, input.TenantID, input.Status, input.Description)
}

func (s *SelfService) DeleteInvitation(ctx context.Context, id int64, tenantID string) error {
	if s == nil || s.invitations == nil {
		return tenantports.ErrSelfServiceUnavailable
	}
	return s.invitations.DeleteInvitationCode(ctx, id, tenantID)
}

func (s *SelfService) GetTenantOverviewStats(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) (*tenantports.TenantOverviewStats, error) {
	if s == nil || s.reader == nil {
		return nil, tenantports.ErrSelfServiceUnavailable
	}
	return s.reader.GetTenantOverviewStats(ctx, tenantID, timeFrom, timeTo)
}

func (s *SelfService) GetClientConsumption(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) ([]tenantports.ClientConsumptionItem, error) {
	if s == nil || s.reader == nil {
		return nil, tenantports.ErrSelfServiceUnavailable
	}
	return s.reader.GetClientConsumption(ctx, tenantID, timeFrom, timeTo)
}

func (s *SelfService) GetUserConsumptionRanking(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time, limit int) ([]tenantports.UserConsumptionItem, error) {
	if s == nil || s.reader == nil {
		return nil, tenantports.ErrSelfServiceUnavailable
	}
	return s.reader.GetUserConsumptionRanking(ctx, tenantID, timeFrom, timeTo, limit)
}

func generateInvitationCode() (string, error) {
	code := make([]byte, 8)
	n := big.NewInt(int64(len(invitationCharset)))
	for i := range code {
		x, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		code[i] = invitationCharset[x.Int64()]
	}
	return string(code), nil
}
