package user

import (
	"context"
	"errors"

	authports "xiaodou/dai/internal/auth/ports"
	userports "xiaodou/dai/internal/user/ports"
)

// AdminAccountLifecycleService owns the application-level sequence for
// administrator account mutations: the account writer commits the database
// state first, then the security writer projects the same state to token/ban
// storage. Keeping this sequence here prevents HTTP handlers from becoming
// cross-domain workflow coordinators.
type AdminAccountLifecycleService struct {
	writer   userports.AdminAccountWriter
	security authports.AccountSecurityWriter
}

var _ userports.AdminAccountLifecycle = (*AdminAccountLifecycleService)(nil)

func NewAdminAccountLifecycleService(writer userports.AdminAccountWriter, security authports.AccountSecurityWriter) *AdminAccountLifecycleService {
	return &AdminAccountLifecycleService{writer: writer, security: security}
}

func (s *AdminAccountLifecycleService) UpdateSystemAdmin(ctx context.Context, input userports.AdminAccountUpdate) (userports.AdminAccountMutationResult, error) {
	if err := s.requireWriter(); err != nil {
		return userports.AdminAccountMutationResult{}, err
	}
	result, err := s.writer.UpdateSystemAdmin(ctx, input)
	if err != nil || !result.Updated || result.Forbidden {
		return result, err
	}
	if err := s.syncUserStatus(ctx, input.UserID, input.Status); err != nil {
		return result, err
	}
	return result, nil
}

func (s *AdminAccountLifecycleService) UpdateTenantUser(ctx context.Context, input userports.AdminAccountUpdate) (bool, error) {
	if err := s.requireWriter(); err != nil {
		return false, err
	}
	updated, err := s.writer.UpdateTenantUser(ctx, input)
	if err != nil || !updated {
		return updated, err
	}
	if err := s.syncUserStatus(ctx, input.UserID, input.Status); err != nil {
		return updated, err
	}
	return updated, nil
}

func (s *AdminAccountLifecycleService) UpdateTenantUserStatus(ctx context.Context, userID, status string) (bool, error) {
	if err := s.requireWriter(); err != nil {
		return false, err
	}
	updated, err := s.writer.UpdateTenantUserStatus(ctx, userID, status)
	if err != nil || !updated {
		return updated, err
	}
	if err := s.syncUserStatus(ctx, userID, status); err != nil {
		return updated, err
	}
	return updated, nil
}

func (s *AdminAccountLifecycleService) ResetSystemAdminPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	if err := s.requireWriter(); err != nil {
		return userports.ActivationCredentialResult{}, err
	}
	result, err := s.writer.ResetSystemAdminPassword(ctx, userID)
	if err != nil || result.Token == "" {
		return result, err
	}
	if err := s.invalidateUserSessions(ctx, userID); err != nil {
		return result, err
	}
	return result, nil
}

func (s *AdminAccountLifecycleService) ResetTenantUserPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	if err := s.requireWriter(); err != nil {
		return userports.ActivationCredentialResult{}, err
	}
	result, err := s.writer.ResetTenantUserPassword(ctx, userID)
	if err != nil || result.Token == "" {
		return result, err
	}
	if err := s.invalidateUserSessions(ctx, userID); err != nil {
		return result, err
	}
	return result, nil
}

func (s *AdminAccountLifecycleService) requireWriter() error {
	if s == nil || s.writer == nil {
		return errors.New("admin account lifecycle writer is not configured")
	}
	return nil
}

func (s *AdminAccountLifecycleService) syncUserStatus(ctx context.Context, userID, status string) error {
	if s.security == nil {
		return nil
	}
	if err := s.security.SyncUserStatus(ctx, userID, status); err != nil {
		return &userports.AdminAccountSecurityError{Cause: err}
	}
	return nil
}

func (s *AdminAccountLifecycleService) invalidateUserSessions(ctx context.Context, userID string) error {
	if s.security == nil {
		return nil
	}
	if err := s.security.InvalidateUserSessions(ctx, userID); err != nil {
		return &userports.AdminAccountSecurityError{Cause: err}
	}
	return nil
}
