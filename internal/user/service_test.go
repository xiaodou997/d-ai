package user

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/user/pg"
)

type userRepositoryStub struct {
	getByUserIDCtx  context.Context
	getByUserIDsCtx context.Context
	updateCtx       context.Context
	users           []*pg.User
	getErr          error
	updateErr       error
}

func (s *userRepositoryStub) GetByUserID(ctx context.Context, _ string) (*pg.User, error) {
	s.getByUserIDCtx = ctx
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &pg.User{UserID: "user-1"}, nil
}

func (s *userRepositoryStub) GetByUserIDs(ctx context.Context, _ []string) ([]*pg.User, error) {
	s.getByUserIDsCtx = ctx
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.users, nil
}

func (s *userRepositoryStub) Update(ctx context.Context, _ string, _ map[string]any) error {
	s.updateCtx = ctx
	return s.updateErr
}

func TestUserServicePropagatesRequestContextToRepository(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &userRepositoryStub{getErr: ctx.Err(), updateErr: ctx.Err(), users: []*pg.User{{UserID: "user-1"}}}
	service := NewUserService(repo, nil, nil)

	if _, err := service.GetUser(ctx, "user-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUser() error = %v, want context.Canceled", err)
	}
	if repo.getByUserIDCtx != ctx {
		t.Fatal("GetUser() did not pass the request context to the repository")
	}

	if _, err := service.BatchGetUsers(ctx, []string{"user-1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("BatchGetUsers() error = %v, want context.Canceled", err)
	}
	if repo.getByUserIDsCtx != ctx {
		t.Fatal("BatchGetUsers() did not pass the request context to the repository")
	}

	if err := service.UpdateUser(ctx, "user-1", map[string]any{"nickname": "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateUser() error = %v, want context.Canceled", err)
	}
	if repo.updateCtx != ctx {
		t.Fatal("UpdateUser() did not pass the request context to the repository")
	}
}
