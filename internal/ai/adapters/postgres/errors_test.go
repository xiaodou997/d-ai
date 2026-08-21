package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestTranslatePersistenceError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil"},
		{name: "not found", err: pgx.ErrNoRows, want: domain.ErrNotFound},
		{name: "unique", err: &pgconn.PgError{Code: "23505"}, want: domain.ErrConflict},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503"}, want: domain.ErrReferencedResourceNotFound},
		{name: "check", err: &pgconn.PgError{Code: "23514"}, want: domain.ErrInvalidFieldValue},
		{name: "format", err: &pgconn.PgError{Code: "22P02"}, want: domain.ErrInvalidInputFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translatePersistenceError(tt.err)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("error = %v, want nil", got)
				}
				return
			}
			if !errors.Is(got, tt.want) {
				t.Fatalf("error = %v, want match %v", got, tt.want)
			}
			if errors.Is(got, tt.err) && tt.err != pgx.ErrNoRows {
				var persistenceErr *domain.PersistenceError
				if !errors.As(got, &persistenceErr) {
					t.Fatalf("error = %T, want PersistenceError", got)
				}
			}
		})
	}
}

func TestNewQueriesTranslatesDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 1})
	if err != nil {
		t.Skipf("open persistence error test pool: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(ctx) })

	q := NewQueries(pool)
	missingID, err := akUUID("10000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("parse missing ID: %v", err)
	}
	if _, err := q.GetPriceBook(ctx, missingID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetPriceBook error = %v, want not found", err)
	}
	params := dbgen.CreatePriceBookParams{
		OwnerType: string(domain.PriceBookOwnerPlatform), Name: "duplicate-book",
	}
	if _, err := q.CreatePriceBook(ctx, params); err != nil {
		t.Fatalf("CreatePriceBook: %v", err)
	}
	if _, err := q.CreatePriceBook(ctx, params); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CreatePriceBook(duplicate) error = %v, want conflict", err)
	}
}
