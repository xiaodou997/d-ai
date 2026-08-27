package notification

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/dbtest"
)

func TestListForActorEnforcesRecipientAndTenantScope(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO sys_notification_deliveries
			(id, event_key, channel, recipient_user_id, recipient_user_type, tenant_id, title, body, status, attempts)
		VALUES
			('00000000-0000-0000-0000-000000000001', 'same-tenant', 'in_app', 'user-a', 4, 'tenant-a', 'same', 'same', 'sent', 1),
			('00000000-0000-0000-0000-000000000002', 'other-tenant', 'in_app', 'user-a', 4, 'tenant-b', 'other', 'other', 'sent', 1),
			('00000000-0000-0000-0000-000000000003', 'other-user', 'in_app', 'user-b', 4, 'tenant-a', 'other', 'other', 'sent', 1),
			('00000000-0000-0000-0000-000000000004', 'global', 'in_app', 'user-a', NULL, NULL, 'global', 'global', 'sent', 1),
			('00000000-0000-0000-0000-000000000005', 'wrong-role', 'in_app', 'user-a', 3, 'tenant-a', 'wrong', 'wrong', 'sent', 1)
	`); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool)
	items, err := service.ListForActor(ctx, auth.NewActor("user-a", "tenant-a", int(auth.UserTypeCustomer)), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("customer notifications = %#v, want same-tenant and global only", items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.EventKey] = true
	}
	for _, event := range []string{"same-tenant", "global"} {
		if !seen[event] {
			t.Fatalf("missing visible notification %q in %#v", event, seen)
		}
	}
	for _, event := range []string{"other-tenant", "other-user", "wrong-role"} {
		if seen[event] {
			t.Fatalf("cross-scope notification %q was visible", event)
		}
	}

	if _, err := service.ListForActor(ctx, auth.NewActor("", "tenant-a", int(auth.UserTypeCustomer)), 10); !errors.Is(err, ErrInvalidActor) {
		t.Fatalf("missing user actor error = %v, want ErrInvalidActor", err)
	}
	if _, err := service.ListForActor(ctx, auth.NewActor("unknown", "tenant-a", 99), 10); !errors.Is(err, ErrInvalidActor) {
		t.Fatalf("unknown role actor error = %v, want ErrInvalidActor", err)
	}
}

func TestSendRejectsUnknownChannelBeforePersistence(t *testing.T) {
	service := NewService(nil)
	_, err := service.Send(context.Background(), Input{Channel: "sms", EventKey: "event", Title: "title"})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("Send() error = %v, want ErrInvalidChannel", err)
	}
}
