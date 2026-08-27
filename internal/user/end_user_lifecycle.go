package user

import (
	"context"
	"errors"

	authports "xiaodou/dai/internal/auth/ports"
	userports "xiaodou/dai/internal/user/ports"
)

// AdminEndUserLifecycleService owns the application sequence for end-user
// mutations. Persistence commits first; security effects are then projected
// through the auth port, keeping Redis/session orchestration out of HTTP.
type AdminEndUserLifecycleService struct {
	writer   userports.AdminEndUserWriter
	security authports.AccountSecurityWriter
}

var _ userports.AdminEndUserLifecycle = (*AdminEndUserLifecycleService)(nil)

func NewAdminEndUserLifecycleService(writer userports.AdminEndUserWriter, security authports.AccountSecurityWriter) *AdminEndUserLifecycleService {
	return &AdminEndUserLifecycleService{writer: writer, security: security}
}

func (s *AdminEndUserLifecycleService) UpdateEndUser(ctx context.Context, input userports.AdminEndUserUpdate) (bool, error) {
	if err := s.requireWriter(); err != nil {
		return false, err
	}
	return s.writer.UpdateEndUser(ctx, input)
}

func (s *AdminEndUserLifecycleService) UpdateEndUserStatus(ctx context.Context, input userports.AdminEndUserStatusUpdate) (bool, error) {
	if err := s.requireWriter(); err != nil {
		return false, err
	}
	updated, err := s.writer.UpdateEndUserStatus(ctx, input)
	if err != nil || !updated {
		return updated, err
	}
	if s.security != nil {
		if err := s.security.SyncUserStatus(ctx, input.UserID, input.Status); err != nil {
			return updated, &userports.AdminEndUserSecurityError{Cause: err}
		}
	}
	return updated, nil
}

func (s *AdminEndUserLifecycleService) ResetEndUserPassword(ctx context.Context, input userports.AdminEndUserPasswordReset) (userports.ActivationCredentialResult, error) {
	if err := s.requireWriter(); err != nil {
		return userports.ActivationCredentialResult{}, err
	}
	result, err := s.writer.ResetEndUserPassword(ctx, input)
	if err != nil || result.Token == "" {
		return result, err
	}
	if s.security != nil {
		if err := s.security.InvalidateUserSessions(ctx, input.UserID); err != nil {
			return result, &userports.AdminEndUserSecurityError{Cause: err}
		}
	}
	return result, nil
}

func (s *AdminEndUserLifecycleService) DeleteEndUser(ctx context.Context, input userports.AdminEndUserDeleteCommand) (userports.AdminEndUserDeleteResult, error) {
	if err := s.requireWriter(); err != nil {
		return userports.AdminEndUserDeleteResult{}, err
	}
	if s.security == nil || !s.security.IsEnabled() {
		return userports.AdminEndUserDeleteResult{}, &userports.AdminEndUserSecurityError{Cause: errors.New("account security ban service is unavailable")}
	}
	input.BeforeCommit = func(ctx context.Context, userID string) error {
		return s.security.BanUser(ctx, userID)
	}
	result, err := s.writer.DeleteEndUser(ctx, input)
	if err != nil {
		var guardErr *userports.AdminEndUserDeleteGuardError
		if errors.As(err, &guardErr) {
			return result, &userports.AdminEndUserSecurityError{Cause: guardErr.Cause}
		}
	}
	return result, err
}

func (s *AdminEndUserLifecycleService) requireWriter() error {
	if s == nil || s.writer == nil {
		return errors.New("admin end-user lifecycle writer is not configured")
	}
	return nil
}
