package billingcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

type repositoryStub struct {
	priceBookExists bool
	importedEntries []domain.PriceBookEntry
}

func (r *repositoryStub) CreatePriceBook(context.Context, domain.PriceBookOwnerType, string, string, string) (domain.PriceBook, error) {
	return domain.PriceBook{}, nil
}

func (r *repositoryStub) GetPriceBook(_ context.Context, id string) (domain.PriceBook, error) {
	if !r.priceBookExists {
		return domain.PriceBook{}, domain.ErrNotFound
	}
	return domain.PriceBook{ID: id, Name: "test", OwnerType: domain.PriceBookOwnerPlatform}, nil
}

func (r *repositoryStub) ListPriceBooks(context.Context) ([]domain.PriceBook, error) {
	return nil, nil
}

func (r *repositoryStub) ListVisiblePriceBooks(context.Context, string) ([]domain.PriceBook, error) {
	return nil, nil
}

func (r *repositoryStub) UpdatePriceBook(context.Context, string, string, string, string) (domain.PriceBook, error) {
	return domain.PriceBook{}, nil
}

func (r *repositoryStub) DeletePriceBook(context.Context, string) error {
	return nil
}

func (r *repositoryStub) CountPriceBookReferences(context.Context, string) (int, error) {
	return 0, nil
}

func (r *repositoryStub) UpsertEntry(_ context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	return e, nil
}

func (r *repositoryStub) ImportEntry(_ context.Context, e domain.PriceBookEntry) error {
	r.importedEntries = append(r.importedEntries, e)
	return nil
}

func (r *repositoryStub) ImportEntries(_ context.Context, _ string, entries []domain.PriceBookEntry) (int, error) {
	r.importedEntries = append(r.importedEntries, entries...)
	return len(entries), nil
}

func (r *repositoryStub) GetEntry(context.Context, string, string, string) (domain.PriceBookEntry, error) {
	return domain.PriceBookEntry{}, nil
}

func (r *repositoryStub) ListEntries(context.Context, string) ([]domain.PriceBookEntry, error) {
	return nil, nil
}

func (r *repositoryStub) DeleteEntry(context.Context, string, string, string) error {
	return nil
}

func (r *repositoryStub) GetSetting(context.Context, string) (json.RawMessage, error) {
	return nil, domain.ErrNotFound
}

func (r *repositoryStub) UpsertSetting(context.Context, string, json.RawMessage) error {
	return nil
}

type fetcherStub struct {
	data map[string]LiteLLMModel
	err  error
}

type fetchStep struct {
	data    map[string]LiteLLMModel
	err     error
	release <-chan struct{}
}

type sequenceFetcher struct {
	mu       sync.Mutex
	steps    []fetchStep
	started  chan int
	finished chan int
	calls    int
}

func newSequenceFetcher(steps ...fetchStep) *sequenceFetcher {
	return &sequenceFetcher{
		steps:    steps,
		started:  make(chan int, len(steps)+1),
		finished: make(chan int, len(steps)+1),
	}
}

func (f *sequenceFetcher) Fetch(ctx context.Context) (map[string]LiteLLMModel, error) {
	f.mu.Lock()
	index := f.calls
	f.calls++
	step := f.steps[min(index, len(f.steps)-1)]
	f.mu.Unlock()
	f.started <- index
	defer func() { f.finished <- index }()
	if step.release != nil {
		select {
		case <-step.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return step.data, step.err
}

func (f *sequenceFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f fetcherStub) Fetch(context.Context) (map[string]LiteLLMModel, error) {
	return f.data, f.err
}

func TestSearchLiteLLMReturnsBuiltInsWhileInitialRefreshRuns(t *testing.T) {
	release := make(chan struct{})
	fetcher := newSequenceFetcher(fetchStep{
		data:    map[string]LiteLLMModel{"claude-test": {Mode: "chat", InputCostPerToken: 0.000001}},
		release: release,
	})
	svc := New(&repositoryStub{}, fetcher)
	result := make(chan []LiteLLMModelInfo, 1)

	go func() {
		items, _ := svc.SearchLiteLLM(context.Background(), "gpt-5.6", 10)
		result <- items
	}()

	select {
	case items := <-result:
		if len(items) != 3 {
			t.Fatalf("initial fallback items = %d, want 3", len(items))
		}
	case <-time.After(200 * time.Millisecond):
		close(release)
		t.Fatal("search blocked on the initial LiteLLM download")
	}

	select {
	case index := <-fetcher.started:
		if index != 0 {
			t.Fatalf("fetch index = %d, want 0", index)
		}
	case <-time.After(time.Second):
		t.Fatal("background LiteLLM refresh did not start")
	}
	close(release)
	waitForServiceLiteLLMModel(t, svc, "claude-test")
}

func TestLiteLLMSourceServesStaleSnapshotWhileSingleRefreshRuns(t *testing.T) {
	secondRelease := make(chan struct{})
	fetcher := newSequenceFetcher(
		fetchStep{data: map[string]LiteLLMModel{"remote-v1": {Mode: "chat", InputCostPerToken: 0.000001}}},
		fetchStep{data: map[string]LiteLLMModel{"remote-v2": {Mode: "chat", InputCostPerToken: 0.000002}}, release: secondRelease},
	)
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	source := newLiteLLMPriceSource(fetcher, time.Minute, time.Second)
	source.now = func() time.Time { return now }
	source.Start(context.Background())

	if index := <-fetcher.started; index != 0 {
		t.Fatalf("first fetch index = %d, want 0", index)
	}
	waitForLiteLLMModel(t, source, "remote-v1")
	<-fetcher.finished
	for range 10 {
		_ = source.Snapshot()
	}
	if calls := fetcher.callCount(); calls != 1 {
		t.Fatalf("fetch calls while cache is fresh = %d, want 1", calls)
	}
	now = now.Add(2 * time.Minute)

	stale := source.Snapshot()
	if _, ok := stale["remote-v1"]; !ok {
		t.Fatal("stale snapshot was not returned during refresh")
	}
	if index := <-fetcher.started; index != 1 {
		t.Fatalf("second fetch index = %d, want 1", index)
	}
	for range 10 {
		_ = source.Snapshot()
	}
	if calls := fetcher.callCount(); calls != 2 {
		t.Fatalf("fetch calls while refresh is in flight = %d, want 2", calls)
	}

	close(secondRelease)
	waitForLiteLLMModel(t, source, "remote-v2")
}

func TestLiteLLMSourceKeepsLastSuccessAfterRefreshFailure(t *testing.T) {
	fetcher := newSequenceFetcher(
		fetchStep{data: map[string]LiteLLMModel{"last-good": {Mode: "chat", InputCostPerToken: 0.000001}}},
		fetchStep{err: errors.New("remote unavailable")},
	)
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	source := newLiteLLMPriceSource(fetcher, time.Minute, time.Second)
	source.now = func() time.Time { return now }
	source.Start(context.Background())
	<-fetcher.started
	waitForLiteLLMModel(t, source, "last-good")
	<-fetcher.finished
	now = now.Add(2 * time.Minute)

	_ = source.Snapshot()
	<-fetcher.started
	if index := <-fetcher.finished; index != 1 {
		t.Fatalf("finished fetch index = %d, want 1", index)
	}
	if _, ok := source.Snapshot()["last-good"]; !ok {
		t.Fatal("failed refresh replaced the last successful snapshot")
	}
	if calls := fetcher.callCount(); calls != 2 {
		t.Fatalf("retry started before backoff elapsed: calls=%d", calls)
	}
}

func waitForLiteLLMModel(t *testing.T, source *liteLLMPriceSource, modelCode string) {
	t.Helper()
	eventually(t, func() bool {
		_, ok := source.Snapshot()[modelCode]
		return ok
	})
}

func waitForServiceLiteLLMModel(t *testing.T, svc *Service, modelCode string) {
	t.Helper()
	eventually(t, func() bool {
		items, err := svc.SearchLiteLLM(context.Background(), modelCode, 100)
		if err != nil {
			return false
		}
		return slices.ContainsFunc(items, func(item LiteLLMModelInfo) bool {
			return item.ModelCode == modelCode
		})
	})
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestValidateEntryRequiresImageDefaultPrice(t *testing.T) {
	err := validateEntry(domain.PriceBookEntry{
		PriceBookID:    "pb_1",
		ModelCode:      "gpt-image-2",
		CapabilityType: "image",
		ImagePrices: []domain.ResolutionUSDPrice{
			{Resolution: "1k", Price: 0.1},
		},
	})
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if verr.Field != "image_default_price" {
		t.Fatalf("validation field = %q, want image_default_price", verr.Field)
	}
}

func TestValidateEntryRequiresVideoDefaultPrice(t *testing.T) {
	err := validateEntry(domain.PriceBookEntry{
		PriceBookID:    "pb_1",
		ModelCode:      "veo-3",
		CapabilityType: "video",
		VideoPrices: []domain.ResolutionUSDPrice{
			{Resolution: "720p", Price: 0.1},
		},
	})
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if verr.Field != "video_default_price" {
		t.Fatalf("validation field = %q, want video_default_price", verr.Field)
	}
}

func TestValidateEntryRejectsOversizedResolutionPrice(t *testing.T) {
	err := validateEntry(domain.PriceBookEntry{
		PriceBookID:       "pb_1",
		ModelCode:         "gpt-image-2",
		CapabilityType:    "image",
		ImageDefaultPrice: 0.1,
		ImagePrices: []domain.ResolutionUSDPrice{
			{Resolution: "1k", Price: maxPricePerToken + 1},
		},
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "image_prices" {
		t.Fatalf("validation error = %#v", err)
	}
}

func TestValidateEntryRejectsNonCanonicalImagePriceTier(t *testing.T) {
	for _, resolution := range []string{"1024x1024", " 1k", "1K"} {
		err := validateEntry(domain.PriceBookEntry{
			PriceBookID:       "pb_1",
			ModelCode:         "gpt-image-2",
			CapabilityType:    "image",
			ImageDefaultPrice: 0.1,
			ImagePrices:       []domain.ResolutionUSDPrice{{Resolution: resolution, Price: 0.1}},
		})
		var validationErr *domain.ValidationError
		if !errors.As(err, &validationErr) || validationErr.Field != "image_prices" {
			t.Fatalf("resolution %q validation error = %#v", resolution, err)
		}
	}
}

func TestImportFromLiteLLMSkipsImageModels(t *testing.T) {
	repo := &repositoryStub{priceBookExists: true}
	svc := New(repo, fetcherStub{
		data: map[string]LiteLLMModel{
			"sample_spec": {
				Mode:              "chat",
				InputCostPerToken: 0.000001,
			},
			"gpt-image-2": {
				Mode:              "image_generation",
				InputCostPerToken: 0.1,
			},
			"gpt-5": {
				Mode:              "chat",
				InputCostPerToken: 0.000002,
			},
		},
	})
	svc.Start(context.Background())
	waitForServiceLiteLLMModel(t, svc, "gpt-5")

	res, err := svc.ImportFromLiteLLM(context.Background(), "pb_1")
	if err != nil {
		t.Fatalf("ImportFromLiteLLM error = %v", err)
	}
	if res.Fetched != 7 || res.Imported != 5 || res.Skipped != 2 {
		t.Fatalf("result = %+v, want fetched=7 imported=5 skipped=2", res)
	}
	gotCodes := make([]string, 0, len(repo.importedEntries))
	for _, entry := range repo.importedEntries {
		gotCodes = append(gotCodes, entry.ModelCode)
	}
	slices.Sort(gotCodes)
	wantCodes := []string{"codex-auto-review", "gpt-5", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"}
	if !slices.Equal(gotCodes, wantCodes) {
		t.Fatalf("imported model codes = %v, want %v", gotCodes, wantCodes)
	}
}

func TestHTTPFetcherAllowsTwoMinutesForLiteLLMDownload(t *testing.T) {
	fetcher := NewHTTPFetcher("")
	if fetcher.client.Timeout != 120*time.Second {
		t.Fatalf("HTTP timeout = %s, want 2m0s", fetcher.client.Timeout)
	}
}

func TestSearchLiteLLMUsesBuiltInFallbackModels(t *testing.T) {
	svc := New(&repositoryStub{}, fetcherStub{err: errors.New("boom")})

	items, err := svc.SearchLiteLLM(context.Background(), "gpt-5.6", 10)
	if err != nil {
		t.Fatalf("SearchLiteLLM error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items len = %d, want 3", len(items))
	}

	want := map[string]LiteLLMModelInfo{
		"gpt-5.6-luna":  builtInModelInfo("gpt-5.6-luna", 1, 6),
		"gpt-5.6-sol":   builtInModelInfo("gpt-5.6-sol", 5, 30),
		"gpt-5.6-terra": builtInModelInfo("gpt-5.6-terra", 2.5, 15),
	}
	for _, item := range items {
		expected, ok := want[item.ModelCode]
		if !ok {
			t.Fatalf("unexpected model returned: %+v", item)
		}
		if !reflect.DeepEqual(item, expected) {
			t.Fatalf("item for %s = %+v, want %+v", item.ModelCode, item, expected)
		}
	}
}

func builtInModelInfo(code string, input, output float64) LiteLLMModelInfo {
	return LiteLLMModelInfo{
		ModelCode:      code,
		CapabilityType: "chat",
		TokenPriceTiers: []LiteLLMTokenPriceTierDTO{{
			InputPer1MUSD: input, OutputPer1MUSD: output,
			CacheWritePer1MUSD: input, CacheReadPer1MUSD: input,
		}},
	}
}

func TestTokenPriceTiersFromLiteLLMParsesMultipleAboveThresholds(t *testing.T) {
	var model LiteLLMModel
	if err := json.Unmarshal([]byte(`{
		"input_cost_per_token": 0.000001,
		"output_cost_per_token": 0.000002,
		"input_cost_per_token_above_200k_tokens": 0.000003,
		"output_cost_per_token_above_200k_tokens": 0.000004,
		"input_cost_per_token_above_272k_tokens": 0.000005,
		"output_cost_per_token_above_272k_tokens": 0.000006
	}`), &model); err != nil {
		t.Fatalf("unmarshal LiteLLM model: %v", err)
	}
	tiers := tokenPriceTiersFromLiteLLM(model)
	if len(tiers) != 3 || tiers[0].UpToInputTokens == nil || *tiers[0].UpToInputTokens != 200_000 ||
		tiers[1].UpToInputTokens == nil || *tiers[1].UpToInputTokens != 272_000 || tiers[2].UpToInputTokens != nil {
		t.Fatalf("tier bounds = %#v", tiers)
	}
	if tiers[1].InputPerToken != 0.000003 || tiers[2].OutputPerToken != 0.000006 {
		t.Fatalf("tier prices = %#v", tiers)
	}
	if tiers[1].CacheReadPerToken != tiers[1].InputPerToken || tiers[2].CacheWritePerToken != tiers[2].InputPerToken {
		t.Fatalf("missing cache prices must inherit their tier input price: %#v", tiers)
	}
}

func TestTokenPriceTiersFromLiteLLMPreservesExplicitFreeBaseCachePrices(t *testing.T) {
	var model LiteLLMModel
	if err := json.Unmarshal([]byte(`{
		"input_cost_per_token": 0.000001,
		"output_cost_per_token": 0.000002,
		"cache_creation_input_token_cost": 0,
		"cache_read_input_token_cost": 0
	}`), &model); err != nil {
		t.Fatalf("unmarshal LiteLLM model: %v", err)
	}
	tier := tokenPriceTiersFromLiteLLM(model)[0]
	if tier.CacheWritePerToken != 0 || tier.CacheReadPerToken != 0 {
		t.Fatalf("explicit free cache prices were not preserved: %#v", tier)
	}

	var missing LiteLLMModel
	if err := json.Unmarshal([]byte(`{
		"input_cost_per_token": 0.000001,
		"output_cost_per_token": 0.000002
	}`), &missing); err != nil {
		t.Fatalf("unmarshal LiteLLM model without cache prices: %v", err)
	}
	missingTier := tokenPriceTiersFromLiteLLM(missing)[0]
	if missingTier.CacheWritePerToken != missingTier.InputPerToken || missingTier.CacheReadPerToken != missingTier.InputPerToken {
		t.Fatalf("missing cache prices did not inherit input price: %#v", missingTier)
	}
}

func TestTokenPriceTiersFromLiteLLMFallsBackToLongContextMultipliers(t *testing.T) {
	model := LiteLLMModel{
		InputCostPerToken: 0.000001, OutputCostPerToken: 0.000002,
		LongContextInputTokenThreshold:    272_000,
		LongContextInputTenantMultiplier:  2,
		LongContextOutputTenantMultiplier: 1.5,
	}
	tiers := tokenPriceTiersFromLiteLLM(model)
	if len(tiers) != 2 || tiers[0].UpToInputTokens == nil || *tiers[0].UpToInputTokens != 272_000 {
		t.Fatalf("tiers = %#v", tiers)
	}
	if tiers[1].InputPerToken != 0.000002 || tiers[1].OutputPerToken != 0.000003 {
		t.Fatalf("long-context prices = %#v", tiers[1])
	}
}

func TestSyncCommonModelsIncludesBuiltInFallbackModels(t *testing.T) {
	repo := &repositoryStub{priceBookExists: true}
	svc := New(repo, fetcherStub{data: map[string]LiteLLMModel{}})

	res, err := svc.SyncCommonModels(context.Background(), "pb_1")
	if err != nil {
		t.Fatalf("SyncCommonModels error = %v", err)
	}
	if res.Synced != 4 {
		t.Fatalf("synced = %d, want 4", res.Synced)
	}
	if !slices.Contains(res.Missing, "gpt-5.5") {
		t.Fatalf("missing = %v, want gpt-5.5 to remain missing", res.Missing)
	}

	gotCodes := make([]string, 0, len(repo.importedEntries))
	for _, entry := range repo.importedEntries {
		gotCodes = append(gotCodes, entry.ModelCode)
	}
	slices.Sort(gotCodes)
	wantCodes := []string{"codex-auto-review", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"}
	if !slices.Equal(gotCodes, wantCodes) {
		t.Fatalf("imported model codes = %v, want %v", gotCodes, wantCodes)
	}
}

func TestSyncCommonModelsFallsBackToCodexAutoReviewPricing(t *testing.T) {
	repo := &repositoryStub{priceBookExists: true}
	svc := New(repo, fetcherStub{data: map[string]LiteLLMModel{}})

	if _, err := svc.SyncCommonModels(context.Background(), "pb_1"); err != nil {
		t.Fatalf("SyncCommonModels error = %v", err)
	}

	var entry domain.PriceBookEntry
	found := false
	for _, candidate := range repo.importedEntries {
		if candidate.ModelCode == "codex-auto-review" {
			entry = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("codex-auto-review was not imported from the built-in fallback")
	}
	tier := entry.TokenPriceTiers[0]
	if tier.UpToInputTokens != nil ||
		tier.InputPerToken != 5.0/litellmPerMillion ||
		tier.OutputPerToken != 30.0/litellmPerMillion ||
		tier.CacheWritePerToken != 5.0/litellmPerMillion ||
		tier.CacheReadPerToken != 0.5/litellmPerMillion {
		t.Fatalf("codex-auto-review fallback tier = %#v", tier)
	}
}
