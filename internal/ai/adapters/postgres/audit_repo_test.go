package postgres

import (
	"context"
	"testing"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestAuditRepoRecordsDomainEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open audit repo test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	status := int32(201)
	repo := NewAuditRepo(dbgen.New(pool))
	err = repo.Record(ctx, domain.AdminAuditEvent{
		Actor:          "admin-1",
		Action:         "groups.import",
		ObjectType:     "group_config_bundle",
		ObjectID:       "bundle-1",
		RequestSummary: []byte(`{"group_count":2}`),
		Result:         "success",
		HttpStatus:     &status,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	items, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	got := items[0]
	if got.Actor != "admin-1" || got.Action != "groups.import" || got.ObjectType != "group_config_bundle" || got.ObjectID != "bundle-1" {
		t.Fatalf("identity fields = %+v", got)
	}
	if string(got.RequestSummary) != `{"group_count": 2}` || got.Result != "success" {
		t.Fatalf("payload fields = %+v", got)
	}
	if got.HttpStatus == nil || *got.HttpStatus != 201 || got.ID == "" || got.CreatedAt.IsZero() {
		t.Fatalf("generated fields = %+v", got)
	}
}

func TestAuditRepoRecordsNullableFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open audit repo test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	repo := NewAuditRepo(dbgen.New(pool))
	if err := repo.Record(ctx, domain.AdminAuditEvent{Action: "accounts.export", RequestSummary: []byte(`{}`), Result: "failed"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	items, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Actor != "" || items[0].ObjectType != "" || items[0].ObjectID != "" || items[0].HttpStatus != nil {
		t.Fatalf("nullable fields not preserved: %+v", items)
	}
}
