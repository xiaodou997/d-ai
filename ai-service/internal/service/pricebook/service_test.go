package pricebook

import (
	"context"
	"encoding/json"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

// fakeRepo is a minimal in-memory Repository for pure-logic tests.
type fakeRepo struct {
	books    map[string]domain.PriceBook
	entries  map[string]domain.PriceBookEntry // key: bookID|modelCode
	settings map[string]json.RawMessage
	imported []domain.PriceBookEntry
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		books:    map[string]domain.PriceBook{"book1": {ID: "book1", Name: "标准价"}},
		entries:  map[string]domain.PriceBookEntry{},
		settings: map[string]json.RawMessage{},
	}
}

func (f *fakeRepo) CreatePriceBook(_ context.Context, name, desc string) (domain.PriceBook, error) {
	return domain.PriceBook{ID: "newbook", Name: name, Description: desc}, nil
}
func (f *fakeRepo) GetPriceBook(_ context.Context, id string) (domain.PriceBook, error) {
	b, ok := f.books[id]
	if !ok {
		return domain.PriceBook{}, domain.ErrNotFound
	}
	return b, nil
}
func (f *fakeRepo) ListPriceBooks(context.Context) ([]domain.PriceBook, error) { return nil, nil }
func (f *fakeRepo) UpdatePriceBook(_ context.Context, id, name, desc, status string) (domain.PriceBook, error) {
	return domain.PriceBook{ID: id, Name: name, Description: desc, Status: status}, nil
}
func (f *fakeRepo) DeletePriceBook(context.Context, string) error { return nil }

func (f *fakeRepo) UpsertEntry(_ context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	f.entries[e.PriceBookID+"|"+e.ModelCode] = e
	return e, nil
}
func (f *fakeRepo) ImportEntry(_ context.Context, e domain.PriceBookEntry) error {
	f.imported = append(f.imported, e)
	return nil
}
func (f *fakeRepo) GetEntry(_ context.Context, b, m string) (domain.PriceBookEntry, error) {
	e, ok := f.entries[b+"|"+m]
	if !ok {
		return domain.PriceBookEntry{}, domain.ErrNotFound
	}
	return e, nil
}
func (f *fakeRepo) ListEntries(context.Context, string) ([]domain.PriceBookEntry, error) {
	return nil, nil
}
func (f *fakeRepo) DeleteEntry(context.Context, string, string) error { return nil }

func (f *fakeRepo) GetSetting(_ context.Context, key string) (json.RawMessage, error) {
	v, ok := f.settings[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return v, nil
}
func (f *fakeRepo) UpsertSetting(_ context.Context, key string, v json.RawMessage) error {
	f.settings[key] = v
	return nil
}

func (f *fakeRepo) UpsertTenantSellBinding(_ context.Context, b domain.TenantSellBinding) (domain.TenantSellBinding, error) {
	return b, nil
}
func (f *fakeRepo) GetTenantSellBinding(context.Context, string) (domain.TenantSellBinding, error) {
	return domain.TenantSellBinding{}, domain.ErrNotFound
}
func (f *fakeRepo) ListTenantSellBindings(context.Context) ([]domain.TenantSellBinding, error) {
	return nil, nil
}
func (f *fakeRepo) DeleteTenantSellBinding(context.Context, string) error { return nil }
func (f *fakeRepo) UpsertUserSellBinding(_ context.Context, b domain.UserSellBinding) (domain.UserSellBinding, error) {
	return b, nil
}
func (f *fakeRepo) GetUserSellBinding(context.Context, string) (domain.UserSellBinding, error) {
	return domain.UserSellBinding{}, domain.ErrNotFound
}
func (f *fakeRepo) DeleteUserSellBinding(context.Context, string) error { return nil }

// fakeFetcher returns a fixed LiteLLM map.
type fakeFetcher struct{ m map[string]LiteLLMModel }

func (f fakeFetcher) Fetch(context.Context) (map[string]LiteLLMModel, error) { return f.m, nil }

func TestImportFromLiteLLM(t *testing.T) {
	repo := newFakeRepo()
	fetch := fakeFetcher{m: map[string]LiteLLMModel{
		"sample_spec":      {InputCostPerToken: 1, Mode: "chat"}, // skipped by name
		"gpt-x":            {InputCostPerToken: 0.000003, OutputCostPerToken: 0.000006, Mode: "chat"},
		"embed-y":          {InputCostPerToken: 0.0000001, Mode: "embedding"},
		"weird-mode":       {InputCostPerToken: 0.1, Mode: "moderation"}, // unsupported → skipped
		"free-or-unpriced": {Mode: "chat"},                               // no price → skipped
	}}
	svc := New(repo, fetch)

	res, err := svc.ImportFromLiteLLM(context.Background(), "book1")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported != 2 {
		t.Fatalf("imported = %d, want 2", res.Imported)
	}
	if res.Fetched != 5 {
		t.Fatalf("fetched = %d, want 5", res.Fetched)
	}
	if res.Skipped != 3 {
		t.Fatalf("skipped = %d, want 3", res.Skipped)
	}
	// verify capability mapping + per-token values pass through unchanged.
	var gotGPT, gotEmbed bool
	for _, e := range repo.imported {
		switch e.ModelCode {
		case "gpt-x":
			gotGPT = true
			if e.CapabilityType != "chat" || e.InputPerToken != 0.000003 {
				t.Fatalf("gpt-x mapped wrong: %+v", e)
			}
		case "embed-y":
			gotEmbed = true
			if e.CapabilityType != "embedding" {
				t.Fatalf("embed-y capability = %q", e.CapabilityType)
			}
		}
	}
	if !gotGPT || !gotEmbed {
		t.Fatalf("missing imported entries: gpt=%v embed=%v", gotGPT, gotEmbed)
	}
}

func TestImportFromLiteLLM_MissingBook(t *testing.T) {
	svc := New(newFakeRepo(), fakeFetcher{})
	if _, err := svc.ImportFromLiteLLM(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for missing price book")
	}
}

func TestCreditsPerUSD(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo, nil)

	// default when unset
	if v, _ := svc.GetCreditsPerUSD(context.Background()); v != domain.DefaultCreditsPerUSD {
		t.Fatalf("default = %v, want %v", v, domain.DefaultCreditsPerUSD)
	}
	// round-trip
	if err := svc.SetCreditsPerUSD(context.Background(), 8.5); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, _ := svc.GetCreditsPerUSD(context.Background()); v != 8.5 {
		t.Fatalf("got = %v, want 8.5", v)
	}
	// reject non-positive
	if err := svc.SetCreditsPerUSD(context.Background(), 0); err == nil {
		t.Fatal("expected validation error for 0")
	}
	// corrupt stored value falls back to default
	repo.settings[domain.SettingCreditsPerUSD] = json.RawMessage(`"oops"`)
	if v, _ := svc.GetCreditsPerUSD(context.Background()); v != domain.DefaultCreditsPerUSD {
		t.Fatalf("corrupt fallback = %v, want default", v)
	}
}

func TestValidateEntry(t *testing.T) {
	svc := New(newFakeRepo(), nil)
	ctx := context.Background()

	// negative price rejected
	if _, err := svc.UpsertEntry(ctx, domain.PriceBookEntry{
		PriceBookID: "book1", ModelCode: "m", CapabilityType: "chat", InputPerToken: -1,
	}); err == nil {
		t.Fatal("expected error for negative price")
	}
	// bad capability rejected
	if _, err := svc.UpsertEntry(ctx, domain.PriceBookEntry{
		PriceBookID: "book1", ModelCode: "m", CapabilityType: "nope",
	}); err == nil {
		t.Fatal("expected error for bad capability")
	}
	// valid passes
	if _, err := svc.UpsertEntry(ctx, domain.PriceBookEntry{
		PriceBookID: "book1", ModelCode: "m", CapabilityType: "chat", InputPerToken: 0.000003,
	}); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
}
