package invite

import (
	"context"
	"testing"

	"xiaodou/dai/internal/invite/pg"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestInvitationBearerCredentialsAreNotLogged(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	repo := &inviteRepoStub{
		createFn: func(_ context.Context, ic *pg.InvitationCode) error {
			ic.ID = 42
			return nil
		},
		getByCodeFn: func(_ context.Context, code string) (*pg.InvitationCode, error) {
			return &pg.InvitationCode{ID: 77, Code: code, TenantID: "tenant-log", Status: 1}, nil
		},
		checkEndUserUsernameExistsFn: func(context.Context, string) (bool, error) {
			return false, nil
		},
		registerEndUserFn: func(context.Context, pg.EndUserRegistration) error {
			return nil
		},
	}
	service := NewInviteService(repo, zap.New(core))

	created, err := service.CreateCode(context.Background(), "tenant-log", "tenant-user", "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 42 || created.Code == "" {
		t.Fatalf("created invitation = %#v", created)
	}

	_, err = service.RegisterUser(
		context.Background(),
		"ABCDEFGH",
		"invited-user",
		"SecurePass!1234",
		nil,
		nil,
		LegalAcceptance{TermsVersion: "terms-v1", PrivacyVersion: "privacy-v1"},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range observed.All() {
		fields := entry.ContextMap()
		if _, ok := fields["code"]; ok {
			t.Fatalf("log %q exposed invitation code field: %#v", entry.Message, fields)
		}
		if _, ok := fields["inviteCode"]; ok {
			t.Fatalf("log %q exposed invitation code field: %#v", entry.Message, fields)
		}
		if entry.Message == "Invitation code created" && fields["invitationId"] != int64(42) {
			t.Fatalf("creation log invitation id = %#v", fields["invitationId"])
		}
		if entry.Message == "User registered via invitation code" && fields["invitationId"] != int64(77) {
			t.Fatalf("registration log invitation id = %#v", fields["invitationId"])
		}
	}
}
