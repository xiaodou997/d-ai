package transport

import (
	"context"
	"errors"
	"reflect"
	"testing"

	aitransport "xiaodou/dai/internal/ai/transport"
	userports "xiaodou/dai/internal/user/ports"
)

type identityUserReaderStub struct {
	users map[string]userports.IdentityUser
	err   error
	ids   []string
}

func (s *identityUserReaderStub) BatchGetIdentityUsers(_ context.Context, userIDs []string) (map[string]userports.IdentityUser, error) {
	s.ids = append([]string(nil), userIDs...)
	return s.users, s.err
}

func TestAIIdentityAdapterUsesUserIdentityProjection(t *testing.T) {
	email := "user@example.com"
	nickname := "User"
	avatar := "avatar.png"
	reader := &identityUserReaderStub{users: map[string]userports.IdentityUser{
		"user-1": {
			UserID: "user-1", TenantID: "tenant-1", Username: "user-one",
			Email: &email, Nickname: &nickname, Avatar: &avatar,
		},
	}}
	adapter := &aiIdentityAdapter{users: reader}

	got, err := adapter.BatchGetUsers(context.Background(), []string{"user-1"})
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if !reflect.DeepEqual(reader.ids, []string{"user-1"}) {
		t.Fatalf("reader ids = %#v", reader.ids)
	}
	want := map[string]*aitransport.IdentityUser{
		"user-1": {
			UserID: "user-1", TenantID: "tenant-1", Username: "user-one",
			Email: &email, Nickname: &nickname, Avatar: &avatar,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity projection = %#v, want %#v", got, want)
	}
}

func TestAIIdentityAdapterPropagatesUserReaderError(t *testing.T) {
	wantErr := errors.New("identity reader unavailable")
	adapter := &aiIdentityAdapter{users: &identityUserReaderStub{err: wantErr}}

	_, err := adapter.BatchGetUsers(context.Background(), []string{"user-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
