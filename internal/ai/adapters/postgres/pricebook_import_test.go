package postgres

import (
	"context"
	"math"
	"sync"
	"testing"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/dbtest"
)

func TestImportEntriesSerializesReplicasAndOnlyBumpsRevisionOnChange(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("price book test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	repo := NewPriceBookRepo(dbgen.New(pool), pool)
	book, err := repo.CreatePriceBook(ctx, domain.PriceBookOwnerPlatform, "", "replica-import", "")
	if err != nil {
		t.Fatalf("create price book: %v", err)
	}
	entry := domain.PriceBookEntry{
		PriceBookID:    book.ID,
		ModelCode:      "gpt-replica",
		CapabilityType: "chat",
		TokenPriceTiers: []domain.TokenPriceTier{{
			InputPerToken:  0.000001,
			OutputPerToken: 0.000002,
		}},
	}

	start := make(chan struct{})
	results := make(chan int, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			changed, err := repo.ImportEntries(ctx, book.ID, []domain.PriceBookEntry{entry})
			results <- changed
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	changedTotal := 0
	for changed := range results {
		changedTotal += changed
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent LiteLLM import: %v", err)
		}
	}
	if changedTotal != 1 {
		t.Fatalf("concurrent changed rows = %d, want exactly 1", changedTotal)
	}
	book, err = repo.GetPriceBook(ctx, book.ID)
	if err != nil {
		t.Fatalf("reload price book: %v", err)
	}
	if book.Revision != 2 {
		t.Fatalf("price book revision = %d, want 2 after one effective import", book.Revision)
	}

	changed, err := repo.ImportEntries(ctx, book.ID, []domain.PriceBookEntry{entry})
	if err != nil {
		t.Fatalf("repeat identical import: %v", err)
	}
	if changed != 0 {
		t.Fatalf("repeat identical import changed rows = %d, want 0", changed)
	}
	book, err = repo.GetPriceBook(ctx, book.ID)
	if err != nil {
		t.Fatalf("reload price book after no-op: %v", err)
	}
	if book.Revision != 2 {
		t.Fatalf("price book revision after no-op = %d, want 2", book.Revision)
	}
}

func TestImportEntriesProtectsManualRowsAndRollsBackBatchErrors(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("price book test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	repo := NewPriceBookRepo(dbgen.New(pool), pool)
	book, err := repo.CreatePriceBook(ctx, domain.PriceBookOwnerPlatform, "", "manual-import", "")
	if err != nil {
		t.Fatalf("create price book: %v", err)
	}
	manual := domain.PriceBookEntry{
		PriceBookID:    book.ID,
		ModelCode:      "manual-model",
		CapabilityType: "chat",
		TokenPriceTiers: []domain.TokenPriceTier{{
			InputPerToken:  0.000010,
			OutputPerToken: 0.000020,
		}},
	}
	if _, err := repo.UpsertEntry(ctx, manual); err != nil {
		t.Fatalf("write manual price: %v", err)
	}
	imported := manual
	imported.TokenPriceTiers = []domain.TokenPriceTier{{InputPerToken: 0.000001, OutputPerToken: 0.000002}}
	changed, err := repo.ImportEntries(ctx, book.ID, []domain.PriceBookEntry{imported})
	if err != nil {
		t.Fatalf("import over manual price: %v", err)
	}
	if changed != 0 {
		t.Fatalf("manual row changed = %d, want 0", changed)
	}
	got, err := repo.GetEntry(ctx, book.ID, manual.ModelCode, manual.CapabilityType)
	if err != nil {
		t.Fatalf("read manual price: %v", err)
	}
	if got.TokenPriceTiers[0].InputPerToken != manual.TokenPriceTiers[0].InputPerToken {
		t.Fatalf("manual input price = %v, want %v", got.TokenPriceTiers[0].InputPerToken, manual.TokenPriceTiers[0].InputPerToken)
	}

	rollbackEntry := domain.PriceBookEntry{
		PriceBookID:    book.ID,
		ModelCode:      "rollback-model",
		CapabilityType: "chat",
		TokenPriceTiers: []domain.TokenPriceTier{{
			InputPerToken:  0.000003,
			OutputPerToken: 0.000004,
		}},
	}
	invalid := rollbackEntry
	invalid.ModelCode = "invalid-model"
	invalid.TokenPriceTiers = []domain.TokenPriceTier{{InputPerToken: math.Inf(1)}}
	if _, err := repo.ImportEntries(ctx, book.ID, []domain.PriceBookEntry{rollbackEntry, invalid}); err == nil {
		t.Fatal("invalid batch import unexpectedly succeeded")
	}
	if _, err := repo.GetEntry(ctx, book.ID, rollbackEntry.ModelCode, rollbackEntry.CapabilityType); err != domain.ErrNotFound {
		t.Fatalf("rolled-back entry lookup error = %v, want ErrNotFound", err)
	}
	if changed, err := repo.ImportEntries(ctx, book.ID, []domain.PriceBookEntry{rollbackEntry}); err != nil || changed != 1 {
		t.Fatalf("retry after rolled-back batch = changed:%d err:%v, want changed:1", changed, err)
	}
}
