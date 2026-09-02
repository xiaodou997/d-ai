package pg_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"xiaodou/dai/internal/announcement"
	announcementpg "xiaodou/dai/internal/announcement/pg"
	"xiaodou/dai/internal/dbtest"
)

func TestTenantScopedAnnouncementVisibilityAndReadReceipt(t *testing.T) {
	pool := openAnnouncementTestPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := "ann_tenant_a_" + suffix
	tenantB := "ann_tenant_b_" + suffix
	userA := "u_ann_user_a_" + suffix
	userB := "u_ann_user_b_" + suffix

	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	mustExec(`INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ($1, $2), ($3, $4)`, tenantA, tenantA, tenantB, tenantB)
	mustExec(`INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type) VALUES ($1, $2, $3, 'x', 4), ($4, $5, $6, 'x', 4)`,
		userA, tenantA, userA, userB, tenantB, userB)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ann_audit_events WHERE actor_user_id = $1`, "SA_"+suffix)
		_, _ = pool.Exec(ctx, `DELETE FROM ann_announcements WHERE created_by = $1`, "SA_"+suffix)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE user_id IN ($1, $2)`, userA, userB)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id IN ($1, $2)`, tenantA, tenantB)
	})

	service := announcement.NewService(announcementpg.NewRepository(pool))
	created, err := service.CreateDraft(ctx, announcement.NewActor("SA_"+suffix, "", 1), announcement.DraftInput{
		Title: "租户 A 升级", ContentMarkdown: "升级内容",
		Audiences: []announcement.AudienceRule{{Kind: announcement.AudienceEndUser, ScopeType: announcement.AudienceScopeTenant, TenantID: tenantA}},
	})
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}
	if _, err := service.Publish(ctx, announcement.NewActor("SA_"+suffix, "", 1), created.ID); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	inboxA, err := service.ListInbox(ctx, announcement.NewActor(userA, tenantA, 4), announcement.InboxQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListInbox(A) error = %v", err)
	}
	if len(inboxA.Items) != 1 || inboxA.UnreadCount != 1 || inboxA.Items[0].Announcement.ID != created.ID {
		t.Fatalf("ListInbox(A) = %#v, want one unread announcement", inboxA)
	}
	inboxB, err := service.ListInbox(ctx, announcement.NewActor(userB, tenantB, 4), announcement.InboxQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListInbox(B) error = %v", err)
	}
	if len(inboxB.Items) != 0 || inboxB.UnreadCount != 0 {
		t.Fatalf("ListInbox(B) = %#v, want empty", inboxB)
	}

	principalA := announcement.NewActor(userA, tenantA, 4)
	if err := service.MarkRead(ctx, principalA, created.ID); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if err := service.MarkRead(ctx, principalA, created.ID); err != nil {
		t.Fatalf("second MarkRead() error = %v", err)
	}
	inboxA, err = service.ListInbox(ctx, principalA, announcement.InboxQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListInbox(A after read) error = %v", err)
	}
	if inboxA.UnreadCount != 0 || inboxA.Items[0].ReadAt == nil {
		t.Fatalf("ListInbox(A after read) = %#v, want read receipt", inboxA)
	}
	stats, err := service.GetStats(ctx, announcement.NewActor("SA_"+suffix, "", 1), created.ID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.AudienceSizeAtPublish != 1 || stats.CurrentAudienceSize != 1 || stats.ReadCount != 1 {
		t.Fatalf("GetStats() = %#v, want audience/read counts 1", stats)
	}
	recipients, err := service.ListRecipients(ctx, announcement.NewActor("SA_"+suffix, "", 1), created.ID, announcement.RecipientQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListRecipients() error = %v", err)
	}
	if recipients.Total != 1 || len(recipients.Items) != 1 || recipients.Items[0].UserID != userA || recipients.Items[0].ReadAt == nil {
		t.Fatalf("ListRecipients() = %#v, want one read recipient", recipients)
	}
	if _, err := service.Archive(ctx, announcement.NewActor("SA_"+suffix, "", 1), created.ID); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	inboxA, err = service.ListInbox(ctx, principalA, announcement.InboxQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListInbox(A after archive) error = %v", err)
	}
	if len(inboxA.Items) != 0 || inboxA.UnreadCount != 0 {
		t.Fatalf("ListInbox(A after archive) = %#v, want empty", inboxA)
	}
	if err := service.DeleteDraft(ctx, announcement.NewActor("SA_"+suffix, "", 1), "missing-announcement"); !errors.Is(err, announcement.ErrNotFound) {
		t.Fatalf("DeleteDraft(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := service.Archive(ctx, announcement.NewActor("SA_"+suffix, "", 1), "missing-announcement"); !errors.Is(err, announcement.ErrNotFound) {
		t.Fatalf("Archive(missing) error = %v, want ErrNotFound", err)
	}
}

func openAnnouncementTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DAI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DAI_TEST_DATABASE_URL to run this DB-backed test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("cannot ping db: %v", err)
	}
	t.Cleanup(pool.Close)

	db := stdlib.OpenDBFromPool(pool)
	defer func(db *sql.DB) { _ = db.Close() }(db)
	if err := dbtest.EnsureCanonicalSchema(ctx, db); err != nil {
		t.Fatalf("initialize canonical test schema: %v", err)
	}
	return pool
}
