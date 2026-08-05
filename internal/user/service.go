package user

import (
	"context"

	"go.uber.org/zap"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/user/pg"
)

// UserService 用户服务
type UserService struct {
	repo      *pg.UserRepository
	blacklist *auth.BlacklistService
	logger    *zap.Logger
}

func NewUserService(repo *pg.UserRepository, blacklist *auth.BlacklistService, logger *zap.Logger) *UserService {
	return &UserService{repo: repo, blacklist: blacklist, logger: logger}
}

func (s *UserService) GetUser(_ context.Context, userID string) (*pg.User, error) {
	return s.repo.GetByUserID(userID)
}

func (s *UserService) BatchGetUsers(_ context.Context, userIDs []string) (map[string]*pg.User, error) {
	users, err := s.repo.GetByUserIDs(userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*pg.User, len(users))
	for _, u := range users {
		result[u.UserID] = u
	}
	return result, nil
}

func (s *UserService) UpdateUser(_ context.Context, userID string, data map[string]any) error {
	return s.repo.Update(userID, data)
}

func (s *UserService) BanUser(ctx context.Context, userID string) error {
	if err := s.repo.Update(userID, map[string]any{"status": 3}); err != nil {
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
	if err := s.repo.Update(userID, map[string]any{"status": 1}); err != nil {
		return err
	}
	if s.blacklist != nil {
		if err := s.blacklist.UnbanUser(ctx, userID); err != nil {
			s.logger.Warn("清除用户封禁状态失败", zap.String("userId", userID), zap.Error(err))
		}
	}
	return nil
}
