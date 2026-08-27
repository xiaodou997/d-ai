package user

import (
	"context"

	"go.uber.org/zap"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/user/pg"
	userports "xiaodou/dai/internal/user/ports"
)

type repository interface {
	GetByUserID(ctx context.Context, userID string) (*pg.User, error)
	GetByUserIDs(ctx context.Context, userIDs []string) ([]*pg.User, error)
	Update(ctx context.Context, userID string, data map[string]any) error
}

// UserService 用户服务
type UserService struct {
	repo      repository
	blacklist *auth.BlacklistService
	logger    *zap.Logger
}

func NewUserService(repo repository, blacklist *auth.BlacklistService, logger *zap.Logger) *UserService {
	return &UserService{repo: repo, blacklist: blacklist, logger: logger}
}

var _ userports.IdentityUserReader = (*UserService)(nil)

func (s *UserService) GetUser(ctx context.Context, userID string) (*pg.User, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *UserService) BatchGetUsers(ctx context.Context, userIDs []string) (map[string]*pg.User, error) {
	users, err := s.repo.GetByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*pg.User, len(users))
	for _, u := range users {
		result[u.UserID] = u
	}
	return result, nil
}

// BatchGetIdentityUsers returns only the stable, non-secret projection needed
// by cross-domain identity enrichment.
func (s *UserService) BatchGetIdentityUsers(ctx context.Context, userIDs []string) (map[string]userports.IdentityUser, error) {
	users, err := s.BatchGetUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]userports.IdentityUser, len(users))
	for id, user := range users {
		result[id] = userports.IdentityUser{
			UserID: user.UserID, TenantID: user.TenantID, Username: user.Username,
			Email: user.Email, Nickname: user.Nickname, Avatar: user.Avatar,
		}
	}
	return result, nil
}

func (s *UserService) UpdateUser(ctx context.Context, userID string, data map[string]any) error {
	return s.repo.Update(ctx, userID, data)
}

func (s *UserService) BanUser(ctx context.Context, userID string) error {
	if err := s.repo.Update(ctx, userID, map[string]any{"status": 3}); err != nil {
		return err
	}
	if s.blacklist != nil {
		if err := s.blacklist.BanUser(ctx, userID); err != nil {
			s.logger.Warn("标记用户封禁状态失败", zap.String("userId", userID), zap.Error(err))
		}
	}
	return nil
}

func (s *UserService) UnbanUser(ctx context.Context, userID string) error {
	if err := s.repo.Update(ctx, userID, map[string]any{"status": 1}); err != nil {
		return err
	}
	if s.blacklist != nil {
		if err := s.blacklist.UnbanUser(ctx, userID); err != nil {
			s.logger.Warn("清除用户封禁状态失败", zap.String("userId", userID), zap.Error(err))
		}
	}
	return nil
}
