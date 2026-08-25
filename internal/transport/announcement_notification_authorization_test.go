package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/auth"
)

func TestAnnouncementAndNotificationActorsPreserveTenantScope(t *testing.T) {
	ctx := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{
		UserID: "user-1", TenantID: "tenant-1", UserType: 4,
	})
	announcementActor, err := announcementActor(ctx)
	if err != nil || announcementActor.UserID != "user-1" || announcementActor.TenantID != "tenant-1" || announcementActor.UserType != 4 {
		t.Fatalf("announcement actor = %#v, err=%v", announcementActor, err)
	}
	notificationActor, err := notificationActor(ctx)
	if err != nil || notificationActor != actorFromClaims(userClaimsFromCtx(ctx)) {
		t.Fatalf("notification actor = %#v, err=%v", notificationActor, err)
	}
}

func TestAnnouncementAndNotificationActorsRejectMissingClaims(t *testing.T) {
	if _, err := announcementActor(context.Background()); err == nil {
		t.Fatal("announcement actor accepted missing claims")
	}
	if _, err := notificationActor(context.Background()); err == nil {
		t.Fatal("notification actor accepted missing claims")
	}
}
